package graceful

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"syscall"
	"time"
)

const (
	oobDefaultBufferSize = 4096
	msgDefaultBufferSize = 4096
)

// Errors used by the Server and server helpers.
var (
	// ErrNotUnixListener is returned by a Server when not a *net.UnixListener
	// is passed to its Serve() method.
	ErrNotUnixListener = errors.New("not a unix listener")

	// ErrLongWrite is returned by the ResponseWriter or Send* functions when
	// data that want be written is too large to be buffered.
	//
	// In this case user should send data separately to the client.
	//
	// Note that it is not possible to send messages larger than selected
	// buffer size because client still will not receive it due to that client
	// and server must use the same buffer size for reading and writing.
	ErrLongWrite = errors.New("long write")
)

// ResponseWriter describes an object that can receive a descriptor.
type ResponseWriter interface {
	Logger
	// Write prepares file descriptor fd to be sent as well as an optional meta
	// information represented by an io.WriterTo.
	Write(fd int, meta io.WriterTo) error
}

// ListenAndServe creates Server instance with given handler and then calls
// server.ListenAndServe(addr) to handle incoming connections.
func ListenAndServe(addr string, handler Handler) error {
	server := &Server{Handler: handler}
	return server.ListenAndServe(addr)
}

// Serve creates Server instance with given handler and then calls
// server.Serve(ln) to handle incoming connections.
func Serve(ln net.Listener, handler Handler) error {
	server := &Server{Handler: handler}
	return server.Serve(ln)
}

// DefaultServer is an empty Server that is used by a Send* global functions as
// a method receiver.
var DefaultServer Server

// Send dials to the "unix" network address addr and sends file descriptor to
// the peer.
func Send(addr string, fd int, meta io.WriterTo) error {
	conn, err := net.Dial("unix", addr)
	if err != nil {
		return err
	}
	return SendTo(conn, fd, meta)
}

// SendTo sends file descriptor fd and its meta to the given conn.
func SendTo(conn net.Conn, fd int, meta io.WriterTo) error {
	return DefaultServer.SendTo(conn, fd, meta)
}

// SendListenerTo sends listener ln and its meta to the given conn.
func SendListenerTo(conn net.Conn, ln net.Listener, meta io.WriterTo) error {
	return DefaultServer.SendListenerTo(conn, ln, meta)
}

// SendConnTo sends connection conn and its meta to the given connection dst.
func SendConnTo(dst, conn net.Conn, meta io.WriterTo) error {
	return DefaultServer.SendConnTo(dst, conn, meta)
}

// SendFileTo sends file and its meta to the given conn.
func SendFileTo(conn net.Conn, file *os.File, meta io.WriterTo) error {
	return DefaultServer.SendFileTo(conn, file, meta)
}

// Server sends descriptors by calling Handler's Hanlde() method for every
// accepted connection.
//
// Note that client and server using this package MUST select the same buffer
// sizes for the meta fields and descriptors. Another option is to use the
// global functions which use default sizes under the hood.
type Server struct {
	// MsgBufferSize defines size of the buffer for meta fields.
	// If MsgBufferSize is zero, then the default size is used.
	MsgBufferSize int

	// OOBBufferSize defines size of the buffer for serialized descriptors.
	// If OOBBufferSize is zero, then the default size is used.
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
	return s.Serve(ln)
}

// Serve accepts incoming connections on the listener l creating a new
// goroutine for each. That goroutine calls s.Handler.Handle(conn, rw) and
// exits.
func (s *Server) Serve(l net.Listener) error {
	ln, ok := l.(*net.UnixListener)
	if !ok {
		return ErrNotUnixListener
	}
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
				s.debugf("closing connection %q", name)
				conn.Close()
			}()

			// We do not handle err here cause it only be when conn is not a
			// *net.UnixConn. Here it is always false.
			resp, _ := s.newResponseWriter(conn)
			s.Handler.Handle(conn, resp)

			if err := resp.Flush(); err != nil {
				s.errorf("flush descriptors to %q error: %v", name, err)
			} else {
				s.infof("sent descriptors to %q", name)
			}
		}()
	}
}

// SendTo sends file descriptor fd and its meta to the given conn.
func (s *Server) SendTo(conn net.Conn, fd int, meta io.WriterTo) error {
	rw, err := s.newResponseWriter(conn)
	if err != nil {
		return err
	}
	rw.Write(fd, meta)
	return rw.Flush()
}

// SendListenerTo sends listener ln and its meta to the given conn.
func (s *Server) SendListenerTo(conn net.Conn, ln net.Listener, meta io.WriterTo) error {
	rw, err := s.newResponseWriter(conn)
	if err != nil {
		return err
	}
	return SendListener(rw, ln, meta)
}

// SendConnTo sends connection conn and its meta to the given connection dst.
func (s *Server) SendConnTo(dst, conn net.Conn, meta io.WriterTo) error {
	rw, err := s.newResponseWriter(conn)
	if err != nil {
		return err
	}
	return SendConn(rw, conn, meta)
}

// SendFileTo sends file and its meta to the given conn.
func (s *Server) SendFileTo(conn net.Conn, file *os.File, meta io.WriterTo) error {
	rw, err := s.newResponseWriter(conn)
	if err != nil {
		return err
	}
	return SendFile(rw, file, meta)
}

func (s *Server) newResponseWriter(conn net.Conn) (*response, error) {
	c, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, ErrNotUnixConn
	}
	var (
		msgn = nonZero(s.MsgBufferSize, msgDefaultBufferSize)
		oobn = nonZero(s.OOBBufferSize, oobDefaultBufferSize)
	)
	return newResponse(
		c, msgn, oobn,
		serverLogger{s},
	), nil
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

// response is an unexported ResponseWriter implementation.
type response struct {
	Logger
	conn *net.UnixConn

	fds []int
	buf []byte
	n   int

	err error
}

// newResponse returns ResponseWriter instance that writes descriptors to the
// given conn.
func newResponse(conn *net.UnixConn, msgn, oobn int, log Logger) *response {
	r := &response{
		Logger: log,
		conn:   conn,

		fds: make([]int, 0, sizeFromCmsgSpace(oobn)),
		buf: make([]byte, msgn),
	}
	return r
}

const msgHeaderSize = 4

var zeroHeader = []byte{0, 0, 0, 0}

func (r *response) Write(fd int, meta io.WriterTo) (ret error) {
	if r.err != nil {
		return r.err
	}
	var (
		metaBytes []byte
		mustCopy  bool
	)
	for {
		if len(r.fds) == cap(r.fds) {
			// No space for a new descriptor.
			goto flush
		}
		if len(r.buf)-r.n < msgHeaderSize {
			// No space even for an empty meta.
			goto flush
		}
		if meta == nil {
			r.n += copy(r.buf[r.n:], zeroHeader)
		} else {
			if metaBytes == nil {
				// Skip msgHeaderSize bytes and get the slice.
				p := r.buf[r.n+msgHeaderSize:]
				// Create bytes.Buffer with slice backed by rw.buf
				// hoping that no reallocation will be made.
				buf := bytes.NewBuffer(p[:0])
				limbuf := &limitedWriter{
					W: buf,
					// Anyway, we can handle only len(rw.buf) bytes even after
					// flushing.
					N: len(r.buf) - msgHeaderSize,
				}
				n, err := meta.WriteTo(limbuf)
				if limbuf.E {
					return ErrLongWrite
				}
				if err != nil {
					return err
				}
				if n == 0 {
					meta = nil
					continue
				}

				metaBytes = buf.Bytes()
				if &metaBytes[0] != &p[0] {
					// Reallocation was made. Must flush before. It is
					// guaranteed that rw.buf fits metaBytes due to
					// limitedWriter above.
					mustCopy = true
					goto flush
				}
			}
			// Buffer header bytes.
			binary.LittleEndian.PutUint32(r.buf[r.n:], uint32(len(metaBytes)))
			r.n += msgHeaderSize
			// Buffer meta bytes.
			if mustCopy {
				r.n += copy(r.buf[r.n:], metaBytes)
			} else {
				r.n += len(metaBytes)
			}
		}
		r.fds = append(r.fds, fd)
		return nil

	flush:
		if ret != nil {
			// Too many flushes – no free space to handle this write.
			return ret
		}
		ret = ErrLongWrite
		if err := r.Flush(); err != nil {
			return err
		}
	}
}

func (r *response) Flush() error {
	if r.err != nil {
		return r.err
	}
	if len(r.fds) == 0 {
		return nil
	}
	var (
		msgBytes = r.buf[:r.n]
		oobBytes = syscall.UnixRights(r.fds...)
	)
	msgn, oobn, err := r.conn.WriteMsgUnix(msgBytes, oobBytes, nil)
	if err == nil && (msgn < len(msgBytes) || oobn < len(oobBytes)) {
		err = io.ErrShortWrite
	}
	r.err = err
	r.n = 0
	r.fds = r.fds[:0]
	return err
}

func sizeFromCmsgSpace(n int) int {
	s := syscall.CmsgSpace(4)
	return n / s
}

type limitedWriter struct {
	W io.Writer
	N int
	E bool
}

func (w limitedWriter) Write(p []byte) (int, error) {
	if w.N <= 0 || len(p) > w.N {
		w.E = true
		return 0, ErrLongWrite
	}
	n, err := w.W.Write(p)
	w.N -= n
	return n, err
}
