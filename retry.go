package gtcp

import (
	"time"
)

type (
	Retry interface {
		Backoff(uint64) time.Duration
	}

	ExponentialRetry struct {
		InitialDelay time.Duration
		MaxDelay     time.Duration
	}
)

var (
	// DefaultRetry implements the same behaviour with net/http/Server
	DefaultRetry Retry = ExponentialRetry{
		InitialDelay: 5 * time.Millisecond,
		MaxDelay:     1 * time.Second,
	}
)

func (er ExponentialRetry) Backoff(retry uint64) time.Duration {
	d := er.InitialDelay * (1 << retry)
	if d > er.MaxDelay {
		d = er.MaxDelay
	}
	return d
}
