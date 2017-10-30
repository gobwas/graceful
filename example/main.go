package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gobwas/graceful"
)

var (
	sock  = flag.String("graceful", "/tmp/graceful.sock", "path to graceful unix socket")
	addr  = flag.String("listen", "localhost:3333", "addr to bind to")
	sleep = flag.Duration("sleep", 5*time.Second, "time to sleep for each http request")
)

func main() {
	flag.Parse()

	var (
		// ln is a listener that we will use below for our web server.
		ln  net.Listener
		err error
	)
	// First assume that some application instance is already running.
	// Then we could try to request an active listener's descriptor from it.
	err = graceful.Receive(*sock, func(fd int, meta graceful.Meta) {
		ln, err = graceful.FdListener(fd, meta)
	})
	if err != nil {
		// Error normally means that no app is running there.
		// Thus current instance become listener initializer.
		ln, err = net.Listen("tcp", *addr)
	}
	if err != nil {
		log.Fatalf("can not listen on %q: %v", *addr, err)
	}

	// Wrap listener such that all accepted connections are registered inside
	// innter sync.WaitGroup. We will Wait() for them after new application
	// instance appear.
	//
	// Note that in some cases such simple solution can not fit performance needs.
	// Wrapped connections does not work well with net.Buffers implementation.
	// See https://github.com/golang/go/issues/21756.
	lw := watchListener(ln)

	// Generate random 8 byte name to send within response.
	name := randomName(8)
	log.Printf(
		"starting server %q on %q (%q %s)",
		name, *addr, ln.Addr().Network(), ln.Addr(),
	)
	defer log.Printf("stopped server %q", name)

	// Start server.
	go http.Serve(lw, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(rw, "Hello, I am a graceful server %q!", name)
		time.Sleep(5 * time.Second)
	}))

	// Create graceful server socket to pass current listener to the new
	// application instance in the future.
	l, err := net.Listen("unix", *sock)
	if err != nil {
		log.Fatalf("can not listen on %q: %v", *sock, err)
	}
	gln := l.(*net.UnixListener)

	// Create exit channel which closure means that application can exit.
	exit := make(chan struct{})
	go graceful.Serve(gln, graceful.SequenceHandler(
		// This "handler" will close graceful socket. This will help us to
		// avoid races on next socket creation.
		graceful.CallbackHandler(func() {
			gln.Close()
		}),

		// This handler will send our web server's listener descriptor to the
		// client.
		graceful.ListenerHandler(ln, graceful.Meta{
			Name: "http",
		}),

		// This "handler" closes exit channel, signaling that we can exit.
		graceful.CallbackHandler(func() {
			close(exit)
		}),
	))

	// Catch SIGINT signal to cleanup gln listener.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)

	// Lock on exit until the new app comes.
	select {
	case <-exit:
		// Wait all accepted connection to be processed before exit.
		log.Printf("stopping server %q", name)
		lw.Wait()
	case <-sig:
		gln.Close()
	}
}

func randomName(n int) string {
	p := make([]byte, n)
	rand.Seed(int64(time.Now().UnixNano()))
	for i := range p {
		p[i] = byte('A' + rand.Intn('Z'-'A'+1))
	}
	return string(p)
}

func watchListener(ln net.Listener) *listener {
	return &listener{Listener: ln}
}

type listener struct {
	net.Listener
	wg sync.WaitGroup
}

type conn struct {
	net.Conn
	onClose func()
}

func (c conn) Close() error {
	err := c.Conn.Close()
	c.onClose()
	return err
}

func (l *listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.wg.Add(1)
	return conn{c, l.wg.Done}, nil
}

func (l *listener) Wait() {
	l.wg.Wait()
}
