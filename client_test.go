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

	var i int
	err = Receive(ln.Addr().String(), func(fd int, meta Meta) {
		same, err := sameFile(fd, expFd[i])
		if err != nil {
			t.Errorf("fstat error: %v", err)
		} else if !same {
			t.Errorf("file descriptors of #%d file are not the same", i)
		}
		if act, exp := meta, expMeta[i]; !reflect.DeepEqual(act, exp) {
			t.Errorf(
				"unexpected meta of #%d file:\nact:\t%#v\nexp:\t%#v\n",
				i, act, exp,
			)
		}
		i++
	})
	if err != nil {
		t.Fatal(err)
	}
	if act, exp := i, 4; act != exp {
		t.Errorf("unexpected number of received descriptors: %d; want %d", act, exp)
	}
}
