package graceful

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
)

const (
	tempFileDir    = ""
	tempFilePrefix = "graceful"
)

func defaultResponseWriter(conn *net.UnixConn) *response {
	return newResponse(
		conn, msgDefaultBufferSize, oobDefaultBufferSize,
		StandardLogger{Prefix: "test"},
	)
}

func TestResponseWriter(t *testing.T) {
	client, server, err := unixSocketpair()
	if err != nil {
		t.Fatal(err)
	}

	resp := defaultResponseWriter(server)

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

		meta := strings.NewReader(f.Name())
		fd := int(f.Fd())
		if err := resp.Write(fd, meta); err != nil {
			t.Fatal(err)
		}

		expMeta[i] = []byte(f.Name())
		expFd[i] = fd
	}
	if err := resp.Flush(); err != nil {
		t.Fatal(err)
	}
	server.Close()

	var ds []descriptor
	err = ReceiveAllFrom(client, func(fd int, meta io.Reader) error {
		b, err := ioutil.ReadAll(meta)
		if err != nil {
			return err
		}
		ds = append(ds, descriptor{fd, b})
		return nil
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
				"unexpected meta of #%d file:\nact:\t%q\nexp:\t%q\n",
				i, act, exp,
			)
		}
	}
	if act, exp := len(ds), 4; act != exp {
		t.Errorf(
			"unexpected number of received descriptors: %d; want %d",
			act, exp,
		)
	}
}

func TestResponseWriterFormat(t *testing.T) {
	for _, test := range []struct {
		name string
		msgn int
		meta []string
		err  []error
	}{
		{
			msgn: 18,
			meta: []string{
				"", "22", "4444", "666666",
			},
		},
		{
			msgn: 10,
			meta: []string{
				"7777777",
			},
			err: []error{
				ErrLongWrite,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server, err := unixSocketpair()
			if err != nil {
				t.Fatal(err)
			}
			rw := newResponse(
				server, test.msgn, 4096,
				StandardLogger{Prefix: "test"},
			)

			f, err := ioutil.TempFile(tempFileDir, tempFilePrefix)
			if err != nil {
				t.Fatal(err)
			}

			for i, meta := range test.meta {
				var exp error
				if test.err != nil {
					exp = test.err[i]
				}
				err := rw.Write(int(f.Fd()), strings.NewReader(meta))
				if err != exp {
					t.Errorf(
						"unexpected error writing %q: %v; want %v",
						meta, err, exp,
					)
				}
			}
			if err := rw.Flush(); err != nil {
				t.Fatal(err)
			}
			server.Close()

			bts, err := ioutil.ReadAll(client)
			if err != nil {
				t.Fatal(err)
			}

			var (
				r = bytes.NewReader(bts)
				h = make([]byte, 4)
			)
			for i, meta := range test.meta {
				if test.err != nil && test.err[i] != nil {
					continue
				}
				if _, err := r.Read(h); err != nil {
					t.Fatalf("error reading #%d item header: %v", i, err)
				}
				n := int(binary.LittleEndian.Uint32(h))
				p := make([]byte, n)
				if m, err := r.Read(p); m != n || err != nil {
					t.Fatalf(
						"error reading #%d item: read %d bytes (%v); want read %d bytes",
						i, m, err, n,
					)
				}
				if act, exp := string(p), meta; act != exp {
					t.Errorf("meta #%d is %q; want %q", i, act, exp)
				}
			}
		})
	}
}

func TestResponseWriterBuffering(t *testing.T) {
	for _, test := range []struct {
		name string
		msgn int
		oobn int
		fdn  int
		meta string
		err  error
		exp  int
	}{
		{
			msgn: 10,
			oobn: 64,
			fdn:  1,
			meta: "7777777",
			err:  ErrLongWrite,
		},
		{
			msgn: 128,
			oobn: 128,
			fdn:  1,
			exp:  1,
		},
		{
			msgn: 7,
			oobn: 128,
			fdn:  2,
			exp:  2,
		},
		{
			msgn: 4096,
			oobn: 128,
			fdn:  10,
			exp:  2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server, err := unixSocketpair()
			if err != nil {
				t.Fatal(err)
			}

			f, err := ioutil.TempFile(tempFileDir, tempFilePrefix)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(f.Name())
			defer f.Close()

			rw := newResponse(
				server, test.msgn, test.oobn,
				StandardLogger{Prefix: "test"},
			)
			for i := 0; i < test.fdn; i++ {
				err = rw.Write(
					int(f.Fd()),
					strings.NewReader(test.meta),
				)
				if err != nil {
					break
				}
			}
			if err == nil {
				err = rw.Flush()
			}
			if err != test.err {
				t.Errorf("unexpected error: %v; want %v", err, test.err)
			}
			server.Close()

			var (
				act = 0
				msg = make([]byte, 4096)
				oob = make([]byte, 4096)
			)
			for {
				_, _, _, _, err := client.ReadMsgUnix(msg, oob)
				if isEOF(err) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				act++
			}
			if exp := test.exp; act != exp {
				t.Errorf("unexpected number of messages: %d; want %d", act, exp)
			}
		})
	}
}

//func TestListenerServer(t *testing.T) {
//	var err error
//	lns := make([]net.Listener, 4)
//	for i := 0; i < len(lns); i++ {
//		lns[i], err = net.Listen("tcp", "localhost:")
//		if err != nil {
//			t.Fatal(err)
//		}
//	}
//
//	h := ListenerServer(lns...)
//
//	var sent []Descriptor
//	h.Handle(nil, &stubResponseWriter{
//		send: func(d []Descriptor) error {
//			sent = append(sent, d...)
//			return nil
//		},
//	})
//
//	if n, m := len(sent), len(lns); n != m {
//		t.Errorf("unexpected sent listeners: %d; want %d", n, m)
//	}
//	for i, s := range sent {
//		if !sameFile(s.Fd, listenerFile(lns[i])) {
//			t.Errorf("sent #%d descriptor is not the same file as #%d listener", i, i)
//		}
//		if act, exp := s.Extra.Name, nameListener(lns[i]); act != exp {
//			t.Errorf("sent #%d descriptor name is %q; want %q", i, act, exp)
//		}
//	}
//
//}

func listenerFile(ln net.Listener) int {
	f, err := ln.(filer).File()
	if err != nil {
		panic(err)
	}
	return int(f.Fd())
}

func sameFile(a, b int) (bool, error) {
	var sa, sb syscall.Stat_t
	if err := syscall.Fstat(a, &sa); err != nil {
		return false, err
	}
	if err := syscall.Fstat(b, &sb); err != nil {
		return false, err
	}
	return sa.Dev == sb.Dev && sa.Ino == sb.Ino, nil
}

func unixSocketpair() (client, server *net.UnixConn, err error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, err
	}

	cf := os.NewFile(uintptr(fds[0]), "client")
	defer cf.Close()
	c, err := net.FileConn(cf)
	if err != nil {
		return nil, nil, err
	}
	sf := os.NewFile(uintptr(fds[1]), "server")
	defer sf.Close()
	s, err := net.FileConn(sf)
	if err != nil {
		return nil, nil, err
	}

	client = c.(*net.UnixConn)
	server = s.(*net.UnixConn)

	return client, server, nil
}
