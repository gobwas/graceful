package graceful

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestReceive(t *testing.T) {
	// Prepare files to be sent.
	var (
		expMeta = make([][]byte, 4)
		expFd   = make([]int, 4)
	)
	for i := 0; i < 4; i++ {
		f, err := ioutil.TempFile(tempFileDir, tempFilePrefix)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(f.Name())
		defer f.Close()

		expFd[i] = int(f.Fd())
		expMeta[i] = []byte(strconv.Itoa(i))
	}

	ln, err := net.Listen("unix", "")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		conn := c.(*net.UnixConn)

		for i := range expFd {
			if err := SendTo(conn, expFd[i], bytes.NewReader(expMeta[i])); err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
		}
		conn.Close()
	}()

	var ds []descriptor
	err = Receive(ln.Addr().String(), func(fd int, meta io.Reader) {
		b, err := ioutil.ReadAll(meta)
		if err != nil {
			t.Fatal(err)
		}
		ds = append(ds, descriptor{fd, b})
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range ds {
		same, err := sameFile(d.fd, expFd[i])
		if err != nil {
			t.Errorf("fstat error: %v", err)
		} else if !same {
			t.Errorf("file descriptors of #%d file are not the same", i)
		}
		if act, exp := d.meta, expMeta[i]; !bytes.Equal(act, exp) {
			t.Errorf(
				"unexpected meta of #%d file:\nact:\t%s\nexp:\t%s\n",
				i, act, exp,
			)
		}
	}
	if act, exp := len(ds), 4; act != exp {
		t.Errorf("unexpected number of received descriptors: %d; want %d", act, exp)
	}
}

type descriptor struct {
	fd   int
	meta []byte
}
