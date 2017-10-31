package graceful

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"syscall"
)

// Errors used by Receive() function.
var (
	ErrEmptyControlMessage  = fmt.Errorf("empty control message")
	ErrEmptyFileDescriptors = fmt.Errorf("empty file descriptors")
)

// ReceiveCallback describes a function that will be called on each received
// descriptor while parsing control messages.
type ReceiveCallback func(fd int, meta Meta)

// Receive dials to the "unix" network address addr and appends all received
// descriptors sent by the server to the given slice.
func Receive(addr string, cb ReceiveCallback) error {
	c := NewClient(msgDefaultBufferSize, oobDefaultBufferSize)
	return c.Receive(addr, cb)
}

// ReceiveMsg reads a single control message contents and appends parsed data
// to the given slice of Descriptors.
func ReceiveMsg(conn *net.UnixConn, cb ReceiveCallback) error {
	c := NewClient(msgDefaultBufferSize, oobDefaultBufferSize)
	return c.ReceiveMsg(conn, cb)
}

// ReceiveAll reads all control messages until EOF. If appends parsed data to
// the given slice of Descriptors.
func ReceiveAll(conn *net.UnixConn, cb ReceiveCallback) error {
	c := NewClient(msgDefaultBufferSize, oobDefaultBufferSize)
	return c.ReceiveAll(conn, cb)
}

// Client contains logic of parsing control messages.
type Client struct {
	msg []byte
	oob []byte
}

// NewClient creates new Client with given sizes for innter buffers. The "msgn"
// argument defines size of the buffer for serialized Meta fields, the "oobn"
// defines the buffer size for serialized descriptors.
//
// Note that client and server using this package MUST select the same buffer
// sizes. Another option is to use the global functions which use default
// sizes under the hood.
func NewClient(msgn, oobn int) *Client {
	return &Client{
		msg: make([]byte, msgn),
		oob: make([]byte, oobn),
	}
}

// Receive diales to the "unix" network address addr and appends all received
// descriptors sent by the server to the given slice.
func (c *Client) Receive(addr string, cb ReceiveCallback) error {
	conn, err := net.Dial("unix", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	return c.ReceiveAll(conn.(*net.UnixConn), cb)
}

// ReceiveMsg reads a single control message contents and appends parsed data
// to the given slice of Descriptors.
func (c *Client) ReceiveMsg(conn *net.UnixConn, cb ReceiveCallback) error {
	return receive(conn, c.msg, c.oob, cb)
}

// ReceiveAll reads all control messages until EOF. If appends parsed data to
// the given slice of Descriptors.
func (c *Client) ReceiveAll(conn *net.UnixConn, cb ReceiveCallback) error {
	return receiveAll(conn, c.msg, c.oob, cb)
}

func receive(conn *net.UnixConn, msg, oob []byte, cb func(int, Meta)) error {
	msgn, oobn, _, _, err := conn.ReadMsgUnix(msg, oob)
	if err != nil {
		if isEOF(err) {
			// Set err to io.EOF cause ReadMsgUnix returns net.OpError for
			// EOF case.
			err = io.EOF
		}
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

	return nil
}

func receiveAll(conn *net.UnixConn, msg, oob []byte, cb func(fd int, meta Meta)) error {
	for {
		err := receive(conn, msg, oob, cb)
		if isEOF(err) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func isEOF(err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		err = opErr.Err
	}
	return err == io.EOF
}
