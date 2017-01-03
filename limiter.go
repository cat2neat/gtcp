package gtcp

import "sync/atomic"

type (
	Limiter interface {
		OnConnected(Conn) bool
		OnClosed(Conn)
	}

	MaxConnLimiter struct {
		Max     uint32
		current uint32 // accessed atomically
	}
)

func (mc *MaxConnLimiter) OnConnected(conn Conn) bool {
	new := atomic.AddUint32(&mc.current, 1)
	if new > mc.Max {
		atomic.AddUint32(&mc.current, ^uint32(0))
		return false
	}
	return true
}

func (mc *MaxConnLimiter) OnClosed(conn Conn) {
	atomic.AddUint32(&mc.current, ^uint32(0))
}
