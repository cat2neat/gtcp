package gtcp

import (
	"log"
)

type (
	// Logger is the interface that wraps logging operations.
	Logger interface {
		// Errorf logs error information.
		// Arguments are handled in the manner of fmt.Printf.
		Errorf(format string, args ...interface{})
	}
	// BuiltinLogger implements Logger based on the standard log package.
	BuiltinLogger struct{}
)

var (
	// DefaultLogger is the default Logger.
	DefaultLogger Logger = BuiltinLogger{}
)

// Errorf logs error information using the standard log package.
func (l BuiltinLogger) Errorf(format string, args ...interface{}) {
	log.Printf(format, args...)
}
