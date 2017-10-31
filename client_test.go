package graceful

import (
	"io/ioutil"
	"net"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestReceive(t *testing.T) {
	// Prepare files to be sent.
	var (
		expMeta = make([]Meta, 4)
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
		expMeta[i] = Meta{Name: f.Name()}
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
			if err := Send(conn, expFd[i], expMeta[i]); err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
		}
		conn.Close()
	}()

	var ds []descriptor
	err = Receive(ln.Addr().String(), func(fd int, meta Meta) {
		ds = append(ds, descriptor{fd, meta})
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
		if act, exp := d.meta, expMeta[i]; !reflect.DeepEqual(act, exp) {
			t.Errorf(
				"unexpected meta of #%d file:\nact:\t%#v\nexp:\t%#v\n",
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
	meta Meta
}
