package gtcp_test

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cat2neat/gtcp"
)

func TestSetKeepAliveHandler(t *testing.T) {
	t.Parallel()
	dc := &debugNetConn{}
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr[:len(buf)])
		return len(buf), nil
	}
	c := gtcp.NewBaseConn(dc)
	srv := gtcp.Server{Logger: gtcp.DefaultLogger}
	first := true
	srv.SetKeepAliveHandler(time.Millisecond, func(conn gtcp.Conn) error {
		if first {
			first = false
			return nil
		}
		// consume buffer in BaseConn that filled by keepalive handler
		var bb [1]byte
		c.Read(bb[:])
		dc.ReadFunc = func(buf []byte) (int, error) {
			return 0, errTest
		}
		return nil
	})
	srv.ConnHandler(context.Background(), c)
	srv.SetKeepAliveHandler(time.Millisecond, func(conn gtcp.Conn) error {
		return errTest
	})
	srv.ConnHandler(context.Background(), c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.SetKeepAliveHandler(time.Millisecond, func(conn gtcp.Conn) error {
		return nil
	})
	srv.ConnHandler(ctx, c)
}

func TestSetPipelineHandler(t *testing.T) {
	dc := &debugNetConn{}
	dc.Remote = "127.0.0.1"
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	c := gtcp.NewBufferedConn(gtcp.NewBaseConn(dc))
	srv := gtcp.Server{Logger: nl}
	var cnt uint32
	srv.SetPipelineHandler(16, func(r io.Reader) ([]byte, error) {
		if atomic.LoadUint32(&cnt) < 16 {
			buf := make([]byte, len(smashingStr))
			r.Read(buf)
			return buf, nil
		}
		return nil, errTest
	}, func(buf []byte, wf gtcp.WriteFlusher) error {
		new := atomic.AddUint32(&cnt, 1)
		if new%2 == 0 {
			return nil
		}
		return errTest
	})
	srv.ConnHandler(context.Background(), c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.SetPipelineHandler(1, func(r io.Reader) ([]byte, error) {
		buf := make([]byte, len(smashingStr))
		r.Read(buf)
		return buf, nil
	}, func(buf []byte, wf gtcp.WriteFlusher) error {
		return nil
	})
	srv.ConnHandler(ctx, c)
	srv.SetPipelineHandler(1, func(r io.Reader) ([]byte, error) {
		panic("gtcp_test: panic for PipelineReader test")
	}, func(buf []byte, wf gtcp.WriteFlusher) error {
		return nil
	})
	srv.ConnHandler(context.Background(), c)
}
