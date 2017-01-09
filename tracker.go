package gtcp

import (
	"context"
	"sync"
	"time"
)

type (
	// ConnTracker is the interface that wraps operations to track active connections.
	ConnTracker interface {
		// AddConn adds Conn to active connections.
		AddConn(Conn)
		// DelConn deletes Conn from active connections.
		DelConn(Conn)
		// Close closes active connections with the same manner on net/http/Server.Close.
		// In short, force close.
		Close() error
		// Shutdown closes active connections with the same manner on net/http/Server.Shutdown.
		// In short, graceful shutdown.
		Shutdown(context.Context) error
	}

	// WGConnTracker implements ConnTracker with sync.WaitGroup.
	// Its Close implemntation is semantically different from what ConnTracker.Close should be.
	// Use MapConnTracker if you want force close active connections.
	WGConnTracker struct {
		wg sync.WaitGroup
	}

	// MapConnTracker implements ConnTracker with the same manner on net/http/Server.
	MapConnTracker struct {
		mu         sync.Mutex
		activeConn map[Conn]struct{}
	}
)

var (
	shutdownPollInterval = 500 * time.Millisecond
)

// AddConn adds Conn to active connections using sync.WaitGroup.Add.
func (ct *WGConnTracker) AddConn(Conn) {
	ct.wg.Add(1)
}

// DelConn deletes Conn from active connections using sync.WaitGroup.Done.
func (ct *WGConnTracker) DelConn(Conn) {
	ct.wg.Done()
}

// Close closes(actually waits for get things done) active connections using sync.WaitGroup.Wait.
func (ct *WGConnTracker) Close() error {
	ct.wg.Wait()
	return nil
}

// Shutdown waits for get active connections closed using sync.WaitGroup.Wait.
func (ct *WGConnTracker) Shutdown(ctx context.Context) error {
	ct.wg.Wait()
	return nil
}

// NewMapConnTracker returns a new MapConnTracker as a ConnTracker.
func NewMapConnTracker() ConnTracker {
	return &MapConnTracker{
		activeConn: make(map[Conn]struct{}),
	}
}

// AddConn adds Conn to active connections using map.
func (ct *MapConnTracker) AddConn(conn Conn) {
	ct.mu.Lock()
	ct.activeConn[conn] = struct{}{}
	ct.mu.Unlock()
}

// DelConn deletes Conn from active connections using map.
func (ct *MapConnTracker) DelConn(conn Conn) {
	ct.mu.Lock()
	delete(ct.activeConn, conn)
	ct.mu.Unlock()
}

// Close closes active connections forcefully.
func (ct *MapConnTracker) Close() error {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	for c := range ct.activeConn {
		c.Close()
		delete(ct.activeConn, c)
	}
	return nil
}

// Shutdown closes active connections with the same manner on net/http/Server.Shutdown.
// It's useful when you use gtcp.Server.SetKeepAliveHandler
// or use ConnHandler directly with gtcp.Conn.(SetIdle|IsIdle)
// as Shutdown only try to close idle connections.
// If the provided context expires before the shutdown is complete,
// then the context's error is returned.
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
