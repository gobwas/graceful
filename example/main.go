package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
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
	err = graceful.Receive(*sock, func(fd int, meta io.Reader) error {
		ln, err = graceful.FdListener(fd)
		return err
	})
	if err != nil {
		// Error normally means that no app is running there.
		// Thus current instance become listener initializer.
		ln, err = net.Listen("tcp", *addr)
	}
	if err != nil {
		log.Fatalf("can not listen on %q: %v", *addr, err)
	}

	// Generate random 8 byte name to send within response.
	name := randomName(8)
	log.Printf(
		"starting server %q on %q %s",
		name, ln.Addr().Network(), ln.Addr(),
	)
	defer log.Printf("stopped server %q", name)

	server := http.Server{Handler: http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(rw, "Hello, I am a graceful server %q!\n", name)
		time.Sleep(*sleep)
	})}
	go server.Serve(ln)

	// Create graceful server socket to pass current listener to the new
	// application instance in the future.
	gln, err := net.Listen("unix", *sock)
	if err != nil {
		log.Fatalf("can not listen on %q: %v", *sock, err)
	}

	// restart is a channel which closure means that new instance of
	// application has been started.
	restart := make(chan struct{})

	go graceful.Serve(gln, graceful.SequenceHandler(
		// This "handler" will close graceful socket. This will help us to
		// avoid races on next socket creation.
		graceful.CallbackHandler(func() {
			gln.Close()
		}),

		// This handler will send our web server's listener descriptor to the
		// client.
		graceful.ListenerHandler(ln, nil),

		// This "handler" closes restart channel, signaling that we can exit.
		graceful.CallbackHandler(func() {
			close(restart)
		}),
	))

	// Catch SIGINT signal to cleanup gln listener.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)

	// Lock on restart until the new app comes.
	select {
	case <-restart:
		// Wait all accepted connection to be processed before exit.
		log.Printf("stopping server %q", name)
		server.Shutdown(context.Background())
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
