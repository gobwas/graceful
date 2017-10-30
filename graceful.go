/*
Package graceful provides tools for passing file descriptors between the
applications.

It is intended to make graceful restarts of Go applications easier.

The very common example:

	ln, err := net.Listen("tcp", "localhost:")
	if err != nil {
		// handle error
	}

	... // Work until must restart.

	graceful.ListenAndServe(
		"/var/my_app/graceful.sock",
		graceful.ListenerHandler(ln),
	)
*/
package graceful
