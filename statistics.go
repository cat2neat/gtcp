package gtcp

import (
	"fmt"
	"sync"
)

type (
	// Statistics is the interface that wraps operations for accumulating Conn statistics.
	Statistics interface {
		// AddConnStats adds a Conn statistics.
		AddConnStats(Conn)
		// Reset clears statistics holden now.
		Reset()
		// String returns a string that represents the current statistics.
		String() string
	}

	// TrafficStatistics implements Statistics to hold the in/out traffic on a gtcp server.
	TrafficStatistics struct {
		mu       sync.RWMutex
		inBytes  int64
		outBytes int64
	}
)

// AddConnStats ingests inBytes and outBytes from conn.
// You need to use StatsConn in gtcp.NewConn if you want in/out traffic.
func (ts *TrafficStatistics) AddConnStats(conn Conn) {
	in, out := conn.Stats()
	ts.mu.Lock()
	ts.inBytes += in
	ts.outBytes += out
	ts.mu.Unlock()
}

// Reset clears statistics holden now.
func (ts *TrafficStatistics) Reset() {
	ts.mu.Lock()
	ts.inBytes, ts.outBytes = 0, 0
	ts.mu.Unlock()
}

// String returns the in/out traffic on a gtcp server as a json string.
func (ts *TrafficStatistics) String() (str string) {
	ts.mu.RLock()
	str = fmt.Sprintf(`{"in_bytes": %d, "out_bytes": %d}`, ts.inBytes, ts.outBytes)
	ts.mu.RUnlock()
	return
}
