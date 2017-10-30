package graceful

import (
	"fmt"
	"log"
)

// DebugLogger interface is used by a Sever or a Client
// to log some debug information.
type DebugLogger interface {
	Debugf(string, ...interface{})
}

// InfoLogger interface is used by a Sever or a Client
// to log some info information.
type InfoLogger interface {
	Infof(string, ...interface{})
}

// ErrorLogger interface is used by a Sever or a Client
// to log some error information.
type ErrorLogger interface {
	Errorf(string, ...interface{})
}

// Logger interface is used by a ResponseWriter to provide log methods for
// Handlers.
type Logger interface {
	DebugLogger
	InfoLogger
	ErrorLogger
}

// DefaultLogger implements all *Logger interfaces. It formats and prints given
// message via standard `log` pacakge.
type DefaultLogger struct {
	Prefix      string
	IgnoreDebug bool
	IgnoreInfo  bool
	IgnoreError bool
}

// Debugf formats and prints debug message via standard `log` package.
// If d.IgnoreDebug is true then it does nothing.
func (d DefaultLogger) Debugf(f string, args ...interface{}) {
	if !d.IgnoreDebug {
		log.Print(d.Prefix, "[DEBUG] ", fmt.Sprintf(f, args...))
	}
}

// Infof formats and prints info message via standard `log` package.
// If d.IgnoreInfo is true then it does nothing.
func (d DefaultLogger) Infof(f string, args ...interface{}) {
	if !d.IgnoreInfo {
		log.Print(d.Prefix, "[INFO] ", fmt.Sprintf(f, args...))
	}
}

// Errorf formats and prints error message via standard `log` package.
// If d.IgnoreError is true then it does nothing.
func (d DefaultLogger) Errorf(f string, args ...interface{}) {
	if d.IgnoreError {
		log.Print(d.Prefix, "[ERROR] ", fmt.Sprintf(f, args...))
	}
}

type serverLogger struct {
	*Server
}

func (s serverLogger) Debugf(f string, args ...interface{}) { s.debugf(f, args...) }
func (s serverLogger) Infof(f string, args ...interface{})  { s.infof(f, args...) }
func (s serverLogger) Errorf(f string, args ...interface{}) { s.errorf(f, args...) }