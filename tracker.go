package gtcp

import (
	"context"
	"sync"
	"time"
)

type (
	ConnTracker interface {
		AddConn(Conn)
		DelConn(Conn)
		Close() error
		Shutdown(context.Context) error
	}

	WGConnTracker struct {
		wg sync.WaitGroup
	}

	MapConnTracker struct {
		mu         sync.Mutex
		activeConn map[Conn]struct{}
	}
)

var (
	shutdownPollInterval = 500 * time.Millisecond
)

func (ct *WGConnTracker) AddConn(Conn) {
	ct.wg.Add(1)
}

func (ct *WGConnTracker) DelConn(Conn) {
	ct.wg.Done()
}

func (ct *WGConnTracker) Close() error {
	ct.wg.Wait()
	return nil
}

func (ct *WGConnTracker) Shutdown(ctx context.Context) error {
	ct.wg.Wait()
	return nil
}

func NewMapConnTracker() ConnTracker {
	return &MapConnTracker{
		activeConn: make(map[Conn]struct{}),
	}
}

func (ct *MapConnTracker) AddConn(conn Conn) {
	ct.mu.Lock()
	ct.activeConn[conn] = struct{}{}
	ct.mu.Unlock()
}

func (ct *MapConnTracker) DelConn(conn Conn) {
	ct.mu.Lock()
	delete(ct.activeConn, conn)
	ct.mu.Unlock()
}

func (ct *MapConnTracker) Close() error {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	for c := range ct.activeConn {
		c.Close()
		delete(ct.activeConn, c)
	}
	return nil
}

func (ct *MapConnTracker) Shutdown(ctx context.Context) error {
	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	for {
		if ct.closeIdleConns() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// closeIdleConns closes all idle connections and reports whether the
// server is quiescent.
func (ct *MapConnTracker) closeIdleConns() bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	quiescent := true
	for c := range ct.activeConn {
		if !c.IsIdle() {
			quiescent = false
			continue
		}
		c.Close()
		delete(ct.activeConn, c)
	}
	return quiescent
}
