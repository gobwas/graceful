package graceful

import (
	"fmt"
	"net"
	"os"
)

// Errors used by the server utils.
var (
	ErrNotFiler = fmt.Errorf("given object does not implements a Filer interface")
)

// Filer describes an object that can return os.File instance.
// Note that `net.Conn` interface implements it.
type Filer interface {
	File() (*os.File, error)
}

// SendListener sends a listener ln with given meta to the ResponseWriter.
func SendListener(resp ResponseWriter, ln net.Listener, meta Meta) error {
	lnf, ok := ln.(Filer)
	if !ok {
		return ErrNotFiler
	}
	return SendFiler(resp, lnf, meta)
}

// SendConn sends a connection conn with given meta to the ResponseWriter.
func SendConn(resp ResponseWriter, conn net.Conn, meta Meta) error {
	connf, ok := conn.(Filer)
	if !ok {
		return ErrNotFiler
	}
	return SendFiler(resp, connf, meta)
}

// SendFiler sends a Filer implementation f with given meta to the
// ResponseWriter.
func SendFiler(resp ResponseWriter, f Filer, meta Meta) error {
	file, err := f.File()
	if err != nil {
		return err
	}
	return SendFile(resp, file, meta)
}

// SendFile sends a file f with given meta to the ResponseWriter.
func SendFile(resp ResponseWriter, file *os.File, meta Meta) error {
	return resp.Write(int(file.Fd()), meta)
}

// ListenerHandler returns a Handler that sends listener ln with given meta to
// the received connection. If some error occures, it logs it by calling
// resp.Errorf().
func ListenerHandler(ln net.Listener, meta Meta) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := SendListener(resp, ln, meta); err != nil {
			resp.Errorf("send listener error: %v", err)
		}
	})
}

// ConnHandler returns a Handler that sends conn with given meta to the
// received connection. If some error occures, it logs it by calling
// resp.Errorf().
func ConnHandler(conn net.Conn, meta Meta) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := SendConn(resp, conn, meta); err != nil {
			resp.Errorf("send conn error: %v", err)
		}
	})
}

// FilerHandler returns a Handler that sends file that returns given Filer
// implementation with given meta to the received connection. If some error
// occures, it logs it by calling resp.Errorf().
func FilerHandler(filer Filer, meta Meta) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := SendFiler(resp, filer, meta); err != nil {
			resp.Errorf("send filer error: %v", err)
		}
	})
}

// FileHandler returns a Handler that sends file with given meta to the
// received connection. If some error occures, it logs it by calling
// resp.Errorf().
func FileHandler(file *os.File, meta Meta) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := SendFile(resp, file, meta); err != nil {
			resp.Errorf("send file error: %v", err)
		}
	})
}

// SequenceHandler returns a Handler that calls Handle() method on each passed
// handlers in sequence.
func SequenceHandler(hs ...Handler) Handler {
	return HandlerFunc(func(conn net.Conn, resp ResponseWriter) {
		for _, h := range hs {
			h.Handle(conn, resp)
		}
	})
}

func nameListener(ln net.Listener) string {
	return ln.Addr().Network() + ":" + ln.Addr().String()
}

func nameConn(conn net.Conn) string {
	return conn.LocalAddr().Network() + ":" + conn.LocalAddr().String() +
		" > " + conn.RemoteAddr().Network() + ":" + conn.RemoteAddr().String()
}
