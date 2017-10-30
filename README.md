# Graceful 💎.

[![GoDoc][godoc-image]][godoc-url]
[![Travis][travis-image]][travis-url]

> Go tools for graceful restarts.

# Overview

Package graceful provides tools for passing file descriptors between the
applications.

The most common intent to use it is to get graceful restart of an application.

# Usage

Graceful proposes a client-server mechanism of sharing descriptors. That is,
application that owns a descriptors is a **server** in `graceful` terminology,
and an application that wants to receive those descriptors is a **client**.

Here is a sketch of [example web application](example) that handles restart
gracefully. This example app does not use `SIGTERM` termination just to show up
how `graceful` could be used in a most simple way. It interprets request for a
listener descriptor as a termination signal.


```go
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gobwas/graceful"
)

var (
	sock = flag.String("graceful", "/tmp/graceful.sock", "path to graceful unix socket")
	addr = flag.String("listen", "localhost:3333", "addr to bind to")
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
	graceful.Receive(*sock, func(fd int, meta graceful.Meta) {
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
	go http.Serve(lw, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		// handle request somehow.
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
		lw.Wait()
	case <-sig:
		gln.Close()
	}
}
```

For full source code listing please see the [`example`](example) directory.

# Status

This library is not tagged s stable version (neither tagged at all). 
This means that backward compatibility can be broken due some changes or
refactoring.


[sigterm]:      https://www.gnu.org/software/libc/manual/html_node/Termination-Signals.html
[example]:      https://github.com/gobwas/graceful
[godoc-image]:  https://godoc.org/github.com/gobwas/graceful?status.svg
[godoc-url]:    https://godoc.org/github.com/gobwas/graceful
[travis-image]: https://travis-ci.org/gobwas/graceful.svg?branch=master
[travis-url]:   https://travis-ci.org/gobwas/graceful
