package gtcp

import (
	"log"
)

type (
	Logger interface {
		Errorf(format string, args ...interface{})
	}

	BuiltinLogger struct{}
)

var (
	DefaultLogger Logger = BuiltinLogger{}
)

func (l BuiltinLogger) Errorf(format string, args ...interface{}) {
	log.Printf(format, args...)
}
