package graceful

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

type funcLogger struct {
	debugf func(string, ...interface{})
	infof  func(string, ...interface{})
	errorf func(string, ...interface{})
}

// LoggerFunc creates Logger from given functions.
func LoggerFunc(debugf, infof, errorf func(string, ...interface{})) Logger {
	return funcLogger{debugf, infof, errorf}
}

func (f funcLogger) Debugf(s string, args ...interface{}) {
	if f.debugf != nil {
		f.debugf(s, args...)
	}
}
func (f funcLogger) Infof(s string, args ...interface{}) {
	if f.infof != nil {
		f.infof(s, args...)
	}
}
func (f funcLogger) Errorf(s string, args ...interface{}) {
	if f.errorf != nil {
		f.errorf(s, args...)
	}
}

type serverLogger struct {
	*Server
}

func (s serverLogger) Debugf(f string, args ...interface{}) { s.debugf(f, args...) }
func (s serverLogger) Infof(f string, args ...interface{})  { s.infof(f, args...) }
func (s serverLogger) Errorf(f string, args ...interface{}) { s.errorf(f, args...) }
