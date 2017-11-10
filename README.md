# Graceful 💎

[![GoDoc][godoc-image]][godoc-url]
[![Travis][travis-image]][travis-url]

> Go tools for graceful restarts.

# Overview

Package graceful provides tools for sharing file descriptors between processes.

The most common reason to use it is the ability of graceful restart of an application.

# Usage

Graceful proposes a client-server mechanism of sharing file descriptors. 
That is, application that owns a descriptors is a **server** in `graceful`
terminology, and an application that wants to receive those descriptors is a
**client**.

The most common use of `graceful` looks like this:

```go
// Somewhere close to the application initialization.
//
// By some logic we've decided to receive descriptors from currently running
// instance of the application.
var (
	ln   net.Listener
	file *os.File
)
err := graceful.Receive("/var/run/app.sock", func(fd int, meta io.Reader) {
	// Handle received descriptor with concrete application logic.
	// 
	// meta is an additional information that corresponds to the descriptor and
	// represented by an io.Reader. User is free to select strategy of
	// marshaling/unmarshaling this information.
	if meta == nil {
		// In our example listener is passed with empty meta.
		ln = graceful.FdListener(fd)
		return
	}
	// There is a helper type called graceful.Meta that could be used to send
	// key-value pairs of meta without additional lines of code. Lets use it.
	m := new(graceful.Meta)
	if _, err := m.ReadFrom(meta); err != nil {
		// Handle error.
	}
	file = os.NewFile(fd, m["name"])
})
if err != nil {
	// Handle error.
}

...

// Somewhere close to the application termination.
//
// By some logic we've decided to send our descriptors to new application
// instance that probably just started.
//
// This code will send `ln` and `file` descriptors to every accepted
// connection on unix domain socket "/var/run/app.sock".
go graceful.ListenAndServe("/var/run/app.sock", graceful.SequenceHandler(
	graceful.ListenerHandler(ln, nil),
	graceful.FileHandler(file, graceful.Meta{
		"name": file.Name(),
	}),
))
```

That is, `graceful` does not force users to stick to some logic of processing
restarts. It just provides mechanism for sharing descriptors in a client-server
way.

If you have a more difficult logic of restarts, you could use less general API
of `graceful`:

```go
// Listen for an upcoming instance at the socket.
ln, err := net.Listen("unix", "/var/run/app.sock")
if err != nil {
	// Handle error.
}

// Accept client connection from another application instance.
conn, err := ln.Accept()
if err != nil {
	// Handle error.
}
defer conn.Close()

// Send some application specific data first. This is useful when data is much
// larger than graceful i/o buffers and won't be putted in the descriptor meta.
if _, err := conn.Write(data); err != nil {
	// Handle error.
}

// Then send some descriptors to the connection.
graceful.SendListenerTo(conn, ln, nil)
graceful.SendListenerTo(conn, file, graceful.Meta{
	"name": file.Name(),
})
```

There is an [example web application](example) that handles restarts
gracefully. Note that it does not handle `SIGTERM` signal just to show up that
`graceful` if flexible and could be used in a simple way. 


# Status

This library is not tagged as stable version (and not tagged at all yet). 
This means that backward compatibility can be broken due some changes or
refactoring.


[sigterm]:      https://www.gnu.org/software/libc/manual/html_node/Termination-Signals.html
[example]:      https://github.com/gobwas/graceful/tree/master/example
[godoc-image]:  https://godoc.org/github.com/gobwas/graceful?status.svg
[godoc-url]:    https://godoc.org/github.com/gobwas/graceful
[travis-image]: https://travis-ci.org/gobwas/graceful.svg?branch=master
[travis-url]:   https://travis-ci.org/gobwas/graceful
