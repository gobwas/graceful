package graceful

import (
	"net"
	"os"
)

// FdListener is a helper function that converts given descriptor to the
// net.Listener interface.
func FdListener(fd int, meta Meta) (net.Listener, error) {
	f := os.NewFile(uintptr(fd), meta.Name)
	ln, err := net.FileListener(f)
	f.Close() // FileListener made dup() inside.
	return ln, err
}

// FdConn is a helper function that converts given descriptor to the
// net.Conn interface.
func FdConn(fd int, meta Meta) (net.Conn, error) {
	f := os.NewFile(uintptr(fd), meta.Name)
	conn, err := net.FileConn(f)
	f.Close() // FileConn made dup() inside.
	return conn, err
}

func nameListener(ln net.Listener) string {
	return ln.Addr().Network() + ":" + ln.Addr().String()
}

func nameConn(conn net.Conn) string {
	return conn.LocalAddr().Network() + ":" + conn.LocalAddr().String() +
		" > " + conn.RemoteAddr().Network() + ":" + conn.RemoteAddr().String()
}

func nonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}
