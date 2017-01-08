package gtcp

import (
	"time"
)

type (
	// Retry is the interface that provides a retry strategy
	// based on a given retry counter.
	Retry interface {
		// Backoff returns a retry interval.
		Backoff(uint64) time.Duration
	}

	// ExponentialRetry implements exponential backoff algorithm without jitter.
	ExponentialRetry struct {
		// InitialDelay defines the retry interval at the first retry.
		InitialDelay time.Duration
		// MaxDelay defines the maximum retry interval.
		MaxDelay time.Duration
	}
)

var (
	// DefaultRetry implements the same behaviour with net/http/Server
	DefaultRetry Retry = ExponentialRetry{
		InitialDelay: 5 * time.Millisecond,
		MaxDelay:     1 * time.Second,
	}
)

// Backoff returns a retry interval based on retry.
func (er ExponentialRetry) Backoff(retry uint64) time.Duration {
	d := er.InitialDelay * (1 << retry)
	if d > er.MaxDelay {
		d = er.MaxDelay
	}
	return d
}
