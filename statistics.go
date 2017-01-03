package gtcp

import (
	"fmt"
	"sync"
)

type (
	Statistics interface {
		AddConnStats(Conn)
		Reset()
		String() string
	}

	TrafficStatistics struct {
		mu       sync.RWMutex
		inBytes  int64
		outBytes int64
	}
)

func (ts *TrafficStatistics) AddConnStats(conn Conn) {
	in, out := conn.Stats()
	ts.mu.Lock()
	ts.inBytes += in
	ts.outBytes += out
	ts.mu.Unlock()
}

func (ts *TrafficStatistics) Reset() {
	ts.mu.Lock()
	ts.inBytes, ts.outBytes = 0, 0
	ts.mu.Unlock()
}

func (ts *TrafficStatistics) String() (str string) {
	ts.mu.RLock()
	str = fmt.Sprintf(`{"in_bytes": %d, "out_bytes": %d}`, ts.inBytes, ts.outBytes)
	ts.mu.RUnlock()
	return
}
