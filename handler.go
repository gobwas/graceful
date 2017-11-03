package graceful

import (
	"fmt"
	"io"
	"net"
	"os"
)

// ErrNotFiler returned when object given to Send* functios does provides
// ability for getting its underlying *os.File.
var ErrNotFiler = fmt.Errorf("given value does not provide *os.File getter")

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

// filer describes an object that can return os.File instance.
// Note that `net.Conn` interface implements it.
type filer interface {
	File() (*os.File, error)
}

// SendListener sends a listener ln with given meta to the ResponseWriter.
func SendListener(resp ResponseWriter, ln net.Listener, meta io.WriterTo) error {
	f, err := fileFrom(ln)
	if err != nil {
		return err
	}
	return SendFile(resp, f, meta)
}

// SendConn sends a connection conn with given meta to the ResponseWriter.
func SendConn(resp ResponseWriter, conn net.Conn, meta io.WriterTo) error {
	f, err := fileFrom(conn)
	if err != nil {
		return err
	}
	return SendFile(resp, f, meta)
}

// SendPacketConn sends a connection conn with given meta to the ResponseWriter.
func SendPacketConn(resp ResponseWriter, conn net.PacketConn, meta io.WriterTo) error {
	f, err := fileFrom(conn)
	if err != nil {
		return err
	}
	return SendFile(resp, f, meta)
}

// SendFile sends a file f with given meta to the ResponseWriter.
func SendFile(resp ResponseWriter, file *os.File, meta io.WriterTo) error {
	return resp.Write(int(file.Fd()), meta)
}

// ListenerHandler returns a Handler that sends listener ln with given meta to
// the received connection. If some error occures, it logs it by calling
// resp.Errorf().
func ListenerHandler(ln net.Listener, meta io.WriterTo) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := SendListener(resp, ln, meta); err != nil {
			resp.Errorf("send listener error: %v", err)
		}
	})
}

// ConnHandler returns a Handler that sends conn with given meta to the
// received connection. If some error occures, it logs it by calling
// resp.Errorf().
func ConnHandler(conn net.Conn, meta io.WriterTo) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := SendConn(resp, conn, meta); err != nil {
			resp.Errorf("send conn error: %v", err)
		}
	})
}

// PacketConnHandler returns a Handler that sends conn with given meta to the
// received connection. If some error occures, it logs it by calling
// resp.Errorf().
func PacketConnHandler(conn net.PacketConn, meta io.WriterTo) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := SendPacketConn(resp, conn, meta); err != nil {
			resp.Errorf("send conn error: %v", err)
		}
	})
}

// FileHandler returns a Handler that sends file with given meta to the
// received connection. If some error occures, it logs it by calling
// resp.Errorf().
func FileHandler(file *os.File, meta io.WriterTo) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := SendFile(resp, file, meta); err != nil {
			resp.Errorf("send file error: %v", err)
		}
	})
}

// FdHandler returns a Handler that sends file descriptor with given meta to
// the received connection. If some error occures, it logs it by calling
// resp.Errorf().
func FdHandler(fd int, meta io.WriterTo) Handler {
	return HandlerFunc(func(_ net.Conn, resp ResponseWriter) {
		if err := resp.Write(fd, meta); err != nil {
			resp.Errorf("send fd error: %v", err)
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

func fileFrom(v interface{}) (*os.File, error) {
	f, ok := v.(filer)
	if !ok {
		return nil, ErrNotFiler
	}
	return f.File()
}
