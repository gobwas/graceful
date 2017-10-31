package graceful

import (
	"encoding/gob"
	"errors"
	"io"
	"net"
	"runtime"
	"syscall"
	"time"
)

const (
	oobBufferSize = 4096
	msgBufferSize = 4096
)

// Meta contains name of a corresponding file descriptor and a custom meta
// fields.
type Meta struct {
	Name   string
	Custom map[string]interface{}
}

// ResponseWriter describes an object that can receive a descriptor.
type ResponseWriter interface {
	Logger
	Write(fd int, meta Meta) error
}

// Handler describes an object that can send descriptors to the connection with
// given ResponseWriter.
type Handler interface {
	Handle(net.Conn, ResponseWriter)
}

// HandlerFunc is an adapter to allow the use of ordinary functions as
// Handlers.
type HandlerFunc func(net.Conn, ResponseWriter)

// Handle calls h(conn, resp).
func (h HandlerFunc) Handle(conn net.Conn, resp ResponseWriter) {
	h(conn, resp)
}

// HandlerFunc is an adapter to allow the use of ordinary functions with empty
// arguments as Handlers.
type CallbackHandler func()

// Handle calls cb().
func (cb CallbackHandler) Handle(net.Conn, ResponseWriter) {
	cb()
}

// ListenAndServe creates Server instance with given handler and then calls
// server.ListenAndServe(addr) to handle incoming connections.
func ListenAndServe(addr string, handler Handler) error {
	server := &Server{Handler: handler}
	return server.ListenAndServe(addr)
}

// Serve creates Server instance with given handler and then calls
// server.Serve(ln) to handle incoming connections.
func Serve(ln *net.UnixListener, handler Handler) error {
	server := &Server{Handler: handler}
	return server.Serve(ln)
}

// Server sends descriptors by calling Handler's Hanlde() method for every
// accepted connection.
//
// Note that client and server using this package MUST select the same buffer
// sizes for the meta fields and descriptors. Another option is to use the
// global functions which use default sizes under the hood.
type Server struct {
	// MsgBufferSize defines size of the buffer for serialized Meta fields
	MsgBufferSize int

	// OOBBufferSize defines size of the buffer for serialized descriptors.
	OOBBufferSize int

	// Handler is a neccessary field that contains logic of sending descriptors
	// to the every arrived connection.
	Handler Handler

	// Logger contains optional implementation of any *Logger interfaces
	// provided by this package.
	// If Logger is nil, then no logging is made.
	Logger interface{}
}

// ListenAndServe listens on the "unix" network address addr and then calls
// Serve to handle incoming connections.
func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("unix", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	return s.Serve(ln.(*net.UnixListener))
}

// Serve accepts incoming connections on the listener ln creating a new
// goroutine for each. That goroutine calls s.Handler.Handle(conn, rw) and
// exits.
func (s *Server) Serve(ln *net.UnixListener) error {
	for {
		conn, err := ln.AcceptUnix()
		if terr, ok := err.(net.Error); ok && terr.Temporary() {
			s.debugf("accept error: %v; delaying", terr)
			time.Sleep(time.Millisecond * 5)
			continue
		}
		if err != nil {
			return err
		}

		name := nameConn(conn)
		s.debugf("accepted connection: %q", name)

		go func() {
			defer func() {
				if err := recover(); err != nil {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]
					s.errorf("panic serving connection %q: %v\n%s", name, err, buf)
				}
			}()

			resp := newResponseWriter(
				conn, s.MsgBufferSize, s.OOBBufferSize,
				serverLogger{s},
			)
			s.Handler.Handle(conn, resp)

			if err := resp.Flush(); err != nil {
				s.errorf("flush descriptors to %q error: %v", name, err)
			} else {
				s.infof("sent descriptors to %q", name)
			}

			s.debugf("closing connection %q", name)
			conn.Close()
		}()
	}
}

func (s *Server) debugf(f string, args ...interface{}) {
	if l, ok := s.Logger.(DebugLogger); ok {
		l.Debugf(f, args...)
	}
}

func (s *Server) infof(f string, args ...interface{}) {
	if l, ok := s.Logger.(InfoLogger); ok {
		l.Infof(f, args...)
	}
}

func (s *Server) errorf(f string, args ...interface{}) {
	if l, ok := s.Logger.(ErrorLogger); ok {
		l.Errorf(f, args...)
	}
}

// responseWriter is an unexported ResponseWriter implementation.
type responseWriter struct {
	Logger
	conn *net.UnixConn

	fds []int
	buf *buffer
	enc *gob.Encoder

	err error
}

// newResponseWriter returns ResponseWriter instance that writes descriptors to
// the given conn.
func newResponseWriter(conn *net.UnixConn, msgn, oobn int, log Logger) *responseWriter {
	buf := newBuffer(msgn)
	enc := gob.NewEncoder(buf)
	r := &responseWriter{
		Logger: log,
		conn:   conn,

		fds: make([]int, 0, sizeFromCmsgSpace(oobn)),
		buf: buf,
		enc: enc,
	}
	return r
}

var ErrLongWrite = errors.New("long write")

func (rw *responseWriter) Write(fd int, meta Meta) (ret error) {
	if rw.err != nil {
		return rw.err
	}
	for {
		var err error
		if len(rw.fds) == cap(rw.fds) {
			goto flush
		}
		err = rw.enc.Encode(meta)
		if err == ErrLongWrite {
			goto flush
		}
		if err != nil {
			rw.err = err
			return err
		}
		rw.fds = append(rw.fds, fd)
		return nil

	flush:
		if ret != nil {
			return ret
		}
		ret = ErrLongWrite
		if err := rw.Flush(); err != nil {
			return err
		}
	}
}

func (rw *responseWriter) Flush() error {
	if rw.err != nil {
		return rw.err
	}
	if len(rw.fds) == 0 {
		return nil
	}
	var (
		msgBytes = rw.buf.Bytes()
		oobBytes = syscall.UnixRights(rw.fds...)
	)
	msgn, oobn, err := rw.conn.WriteMsgUnix(msgBytes, oobBytes, nil)
	if err == nil && (msgn < len(msgBytes) || oobn < len(oobBytes)) {
		err = io.ErrShortWrite
	}
	rw.err = err
	rw.buf.Reset()
	rw.fds = rw.fds[:0]
	return err
}

func sizeFromCmsgSpace(n int) int {
	s := syscall.CmsgSpace(4)
	return n / s
}

type buffer struct {
	p []byte
	n int
}

func newBuffer(n int) *buffer {
	return &buffer{
		p: make([]byte, n),
	}
}

func (b *buffer) Write(p []byte) (n int, err error) {
	n = copy(b.p[b.n:], p)
	if len(p) > n {
		return 0, ErrLongWrite
	}
	b.n += n
	return n, nil
}

func (b *buffer) Reset() {
	b.n = 0
}

func (b *buffer) Bytes() []byte {
	return b.p[:b.n]
}

// Send is a standalone file descriptor to conn sender.
func Send(conn *net.UnixConn, fd int, meta Meta) error {
	rw := newResponseWriter(conn, msgBufferSize, oobBufferSize, nil)
	rw.Write(fd, meta)
	return rw.Flush()
}
