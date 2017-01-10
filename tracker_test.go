package gtcp_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cat2neat/gtcp"
)

func testConnTracker(ct gtcp.ConnTracker,
	connHandler func(gtcp.ConnTracker, gtcp.Conn),
	finishHandler func(gtcp.ConnTracker),
	t *testing.T) {
	const max = 64
	bcs := make([]gtcp.Conn, max)
	for i := 0; i < max/2; i++ {
		bcs[i] = gtcp.NewBaseConn(&debugNetConn{})
		ct.AddConn(bcs[i])
	}
	for i := 0; i < max/2; i++ {
		li := i
		go ct.DelConn(bcs[li])
	}
	var wg sync.WaitGroup
	for i := max / 2; i < max; i++ {
		bcs[i] = gtcp.NewBaseConn(&debugNetConn{})
		li := i
		wg.Add(1)
		go func() {
			ct.AddConn(bcs[li])
			wg.Done()
		}()
	}
	wg.Wait()
	for i := max / 2; i < max; i++ {
		li := i
		go connHandler(ct, bcs[li])
	}
	finishHandler(ct)
}

func TestWGConnTracker(t *testing.T) {
	t.Parallel()
	ct := &gtcp.WGConnTracker{}
	testConnTracker(ct,
		func(ct gtcp.ConnTracker, conn gtcp.Conn) {
			ct.DelConn(conn)
		},
		func(ct gtcp.ConnTracker) {
			ct.Close()
		},
		t)
	testConnTracker(ct,
		func(ct gtcp.ConnTracker, conn gtcp.Conn) {
			ct.DelConn(conn)
		},
		func(ct gtcp.ConnTracker) {
			ct.Shutdown(context.Background())
		},
		t)
}

func TestMapConnTracker(t *testing.T) {
	t.Parallel()
	ct := gtcp.NewMapConnTracker()
	testConnTracker(ct,
		func(ct gtcp.ConnTracker, conn gtcp.Conn) {
			ct.DelConn(conn)
		},
		func(ct gtcp.ConnTracker) {
			ct.Close()
		},
		t)
	testConnTracker(ct,
		func(ct gtcp.ConnTracker, conn gtcp.Conn) {
			conn.SetIdle(true)
		},
		func(ct gtcp.ConnTracker) {
			ct.Shutdown(context.Background())
		},
		t)
	testConnTracker(ct,
		func(ct gtcp.ConnTracker, conn gtcp.Conn) {
			conn.SetIdle(false)
		},
		func(ct gtcp.ConnTracker) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			err := ct.Shutdown(ctx)
			if err != context.DeadlineExceeded {
				t.Errorf("gtcp_test: ConnTracker.Shutdown expected: %+v\n", context.DeadlineExceeded)
			}
			cancel()
		},
		t)
}
