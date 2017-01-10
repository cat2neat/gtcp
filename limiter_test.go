package gtcp_test

import (
	"sync"
	"testing"

	"github.com/cat2neat/gtcp"
)

func TestMaxConnLimiter(t *testing.T) {
	t.Parallel()
	const max = 64
	ml := gtcp.MaxConnLimiter{Max: max}
	bc := gtcp.NewBaseConn(&debugNetConn{})

	// single
	for i := 0; i < max; i++ {
		if !ml.OnConnected(bc) {
			t.Errorf("gtcp_test: MaxConnLimiter.OnConnected expected: true actual: false\n")
		}
	}
	if ml.OnConnected(bc) {
		t.Errorf("gtcp_test: MaxConnLimiter.OnConnected expected: false actual: true\n")
	}
	ml.OnClosed(bc)
	if !ml.OnConnected(bc) {
		t.Errorf("gtcp_test: MaxConnLimiter.OnConnected expected: true actual: false\n")
	}
	// parallel
	ml = gtcp.MaxConnLimiter{Max: max}
	wg := sync.WaitGroup{}
	for i := 0; i < max/2; i++ {
		wg.Add(1)
		go func() {
			ml.OnConnected(bc)
			wg.Done()
		}()
	}
	wg.Wait()
	for i := 0; i < max/2; i++ {
		li := i // capture
		wg.Add(1)
		go func() {
			if li%2 == 0 {
				ml.OnConnected(bc)
			} else {
				ml.OnClosed(bc)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	for i := 0; i < max/2; i++ {
		ml.OnConnected(bc)
	}
	if ml.OnConnected(bc) {
		t.Errorf("gtcp_test: MaxConnLimiter.OnConnected expected: false actual: true\n")
	}
	ml.OnClosed(bc)
	if !ml.OnConnected(bc) {
		t.Errorf("gtcp_test: MaxConnLimiter.OnConnected expected: true actual: false\n")
	}
}
