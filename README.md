# Graceful 💎

[![GoDoc][godoc-image]][godoc-url]
[![Travis][travis-image]][travis-url]

> Go tools for graceful restarts.

# Overview

Package graceful provides tools for sharing file descriptors between the
processes.

The most common intent to use it is to get graceful restarts of an application.

# Usage

Graceful proposes a client-server mechanism of sharing file descriptors. 
That is, application that owns a descriptors is a **server** in `graceful`
terminology, and an application that wants to receive those descriptors is a
**client**.

The most common use of `graceful` looks like this:

```go
// Somewhere close to the application init.
err := graceful.Receive("/var/run/app.sock", func(fd int, meta io.Reader) {
	// Handle received descriptor depending on application logic.
	// 
	// meta is an additional information that corresponds to the descriptor and
	// represented by an io.Reader. User is free to select strategy of
	// marshaling/unmarshaling this information.
	//	
	// There is a helper type called graceful.Meta that could be used to send
	// key-value pairs of meta without additional lines of code.
	if meta == nil {
		ln = graceful.FdListener(fd)
		return
	}
	m, err := graceful.MetaFrom(meta)
	if err == nil {
		file = os.NewFile(fd, m["name"])
	}
})

// Somewhere in the application.
// This code will send `ln` and `file` descriptors to every accepted
// connection on unix domain socket "/var/run/app.sock".
go graceful.ListenAndServe("/var/run/app.sock", graceful.SequenceHandler(
	graceful.ListenerHandler(ln, nil),
	graceful.FileHandler(file, Meta{
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
	// handle error
}
app, err := ln.Accept()
if err != nil {
	// handle error
}
// Send some application specific data first.
if _, err := app.Write(data); err != nil {
	// handle error
}
// Then send some descriptors to the connection.
graceful.SendListenerTo(app, ln, nil)
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
