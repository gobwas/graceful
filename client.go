package graceful

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"syscall"
)

// Errors used by Receive() function.
var (
	ErrEmptyControlMessage  = fmt.Errorf("empty control message")
	ErrEmptyFileDescriptors = fmt.Errorf("empty file descriptors")
)

// ErrNotUnixConn is returned by a Client when not a *net.UnixConn is
// passed to its Receive* methods.
var ErrNotUnixConn = errors.New("not a unix connection")

// ReceiveCallback describes a function that will be called on each received
// descriptor while parsing control messages.
// Its first argument is a received file descriptor. Its second argument is an
// optional meta information represented by an io.Reader.
//
// If the callback returns non-nil error, then the function to which this
// callback was given exits immediately with that error.
//
// Note that meta reader is only valid until callback returns.
// If server does not provide additional information for descriptor, meta
// argument will be nil.
type ReceiveCallback func(fd int, meta io.Reader) error

// Receive dials to the "unix" network address addr and calls cb for each
// received descriptor from it until EOF.
func Receive(addr string, cb ReceiveCallback) error {
	c := NewClient(msgDefaultBufferSize, oobDefaultBufferSize)
	return c.Receive(addr, cb)
}

// ReceiveFrom reads a single control message from the given connection conn
// and calls cb for each descriptor inside that message.
func ReceiveFrom(conn net.Conn, cb ReceiveCallback) error {
	c := NewClient(msgDefaultBufferSize, oobDefaultBufferSize)
	return c.ReceiveFrom(conn, cb)
}

// ReceiveAllFrom reads all control messages from the given connection conn and
// calls cb for each descriptor inside those messages.
func ReceiveAllFrom(conn net.Conn, cb ReceiveCallback) error {
	c := NewClient(msgDefaultBufferSize, oobDefaultBufferSize)
	return c.ReceiveAllFrom(conn, cb)
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

// Receive dials to the "unix" network address addr and calls cb for each
// received descriptor.
func (c *Client) Receive(addr string, cb ReceiveCallback) error {
	conn, err := net.Dial("unix", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	return c.ReceiveAllFrom(conn, cb)
}

// ReceiveFrom reads a single control message from the given connection conn
// and calls cb for each descriptor inside that message.
func (c *Client) ReceiveFrom(conn net.Conn, cb ReceiveCallback) error {
	return receive(conn, c.msg, c.oob, cb)
}

// ReceiveAllFrom reads all control messages from the given connection conn and
// calls cb for each descriptor inside those messages.
func (c *Client) ReceiveAllFrom(conn net.Conn, cb ReceiveCallback) error {
	for {
		err := receive(conn, c.msg, c.oob, cb)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
	}
	return nil
}

func receive(c net.Conn, msg, oob []byte, cb ReceiveCallback) error {
	conn, ok := c.(*net.UnixConn)
	if !ok {
		return ErrNotUnixConn
	}
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

	var (
		r = bytes.NewReader(msg[:msgn])
		p = make([]byte, 4)
	)
	for _, fd := range fds {
		// Read meta header.
		if _, err := r.Read(p); err != nil {
			return err
		}
		n := int64(binary.LittleEndian.Uint32(p))

		var meta io.Reader
		if n > 0 {
			meta = io.LimitReader(r, n)
		}
		if err := cb(fd, meta); err != nil {
			return err
		}
		if meta != nil {
			// Ensure that all meta bytes was read.
			_, err := io.Copy(ioutil.Discard, meta)
			if err != nil {
				return err
			}
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
