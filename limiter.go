package gtcp

import "sync/atomic"

type (
	// Limiter is the interface that limits connections accepted.
	Limiter interface {
		// OnConnected is called when Conn accepted on a listener.
		// Returns false if limits it otherwise true.
		OnConnected(Conn) bool
		// OnClosed is called when Conn closed.
		OnClosed(Conn)
	}

	// MaxConnLimiter implements Limiter based on the maximum connections.
	MaxConnLimiter struct {
		// Max defines the maximum connections.
		Max     uint32
		current uint32 // accessed atomically
	}
)

// OnConnected returns false if the number of current active connections exceeds Max,
// otherwise true.
func (mc *MaxConnLimiter) OnConnected(conn Conn) bool {
	new := atomic.AddUint32(&mc.current, 1)
	if new > mc.Max {
		atomic.AddUint32(&mc.current, ^uint32(0))
		return false
	}
	return true
}

// OnClosed decreases the number of current active connections.
func (mc *MaxConnLimiter) OnClosed(conn Conn) {
	atomic.AddUint32(&mc.current, ^uint32(0))
}
