package graceful

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

// Errors used by Receive() function.
var (
	ErrEmptyControlMessage  = fmt.Errorf("empty control message")
	ErrEmptyFileDescriptors = fmt.Errorf("empty file descriptors")
)

// Receive dials to the "unix" network address addr and receives all
// descriptors sent by the server. It calls cb for each received descriptor and
// its meta.
func Receive(addr string, cb func(fd int, meta Meta)) error {
	conn, err := net.Dial("unix", addr)
	if err != nil {
		return err
	}
	err = receive(conn.(*net.UnixConn), cb)
	conn.Close()
	return err
}

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

func receive(conn *net.UnixConn, cb func(int, Meta)) error {
	msg := make([]byte, 4096)
	oob := make([]byte, 4096)

	for {
		msgn, oobn, _, _, err := conn.ReadMsgUnix(msg, oob)
		if isEOF(err) {
			return nil
		}
		if err != nil {
			return err
		}

		cmsg, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return err
		}
		if len(cmsg) == 0 {
			return ErrEmptyControlMessage
		}
		fds, err := syscall.ParseUnixRights(&cmsg[0])
		if err != nil {
			return err
		}
		if len(fds) == 0 {
			return ErrEmptyFileDescriptors
		}

		dec := gob.NewDecoder(bytes.NewReader(msg[:msgn]))
		for _, fd := range fds {
			var meta Meta
			if err = dec.Decode(&meta); err != nil {
				return err
			}
			cb(fd, meta)
		}
	}

	return nil
}

func isEOF(err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		err = opErr.Err
	}
	return err == io.EOF
}
