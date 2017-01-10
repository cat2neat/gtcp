package gtcp_test

import (
	"context"
	"testing"

	"github.com/cat2neat/gtcp"
)

func TestBaseConn(t *testing.T) {
	t.Parallel()
	dc := &debugNetConn{}
	bc := gtcp.NewBaseConn(dc)
	// read related
	buf := make([]byte, 4)
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	n, err := bc.Read(buf)
	if n != 4 || string(buf) != smashingStr || err != nil {
		t.Errorf("gtcp_test: BaseConn.Read expected: 4, nil actual: %d, %+v\n", n, err)
	}
	dc.ReadFunc = func(buf []byte) (int, error) {
		return 0, errTest
	}
	n, err = bc.Read(buf)
	if n != 0 || err != errTest {
		t.Errorf("gtcp_test: BaseConn.Read expected: 0, nil actual: %d, %+v\n", n, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	bc.SetCancelFunc(cancel)
	n, err = bc.Read(buf)
	if n != 0 || err != errTest {
		t.Errorf("gtcp_test: BaseConn.Read expected: 0, nil actual: %d, %+v\n", n, err)
	}
	<-ctx.Done() // CancelFunc should be called when error happened
	// idle related
	idle := bc.IsIdle()
	if idle {
		t.Errorf("gtcp_test: BaseConn.IsIdle expected: false actual: %v\n", idle)
	}
	bc.SetIdle(true)
	idle = bc.IsIdle()
	if !idle {
		t.Errorf("gtcp_test: BaseConn.IsIdle expected: true actual: %v\n", idle)
	}
	bc.SetIdle(false)
	idle = bc.IsIdle()
	if idle {
		t.Errorf("gtcp_test: BaseConn.IsIdle expected: false actual: %v\n", idle)
	}
	// write related
	dc.WriteFunc = func(buf []byte) (int, error) {
		return 4, nil
	}
	n, err = bc.Write(buf)
	if n != 4 || err != nil {
		t.Errorf("gtcp_test: BaseConn.Write expected: 4, nil actual: %d, %+v\n", n, err)
	}
	bc.SetCancelFunc(nil)
	dc.WriteFunc = func(buf []byte) (int, error) {
		return 0, errTest
	}
	n, err = bc.Write(buf)
	if n != 0 || err != errTest {
		t.Errorf("gtcp_test: BaseConn.Write expected: 0, errTest actual: %d, %+v\n", n, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	bc.SetCancelFunc(cancel)
	n, err = bc.Write(buf)
	if n != 0 || err != errTest {
		t.Errorf("gtcp_test: BaseConn.Write expected: 0, errTest actual: %d, %+v\n", n, err)
	}
	<-ctx.Done() // CancelFunc should be called when error happened
	in, out := bc.Stats()
	if in != 0 || out != 0 {
		t.Errorf("gtcp_test: BaseConn.Stats expected: 0, 0 actual: %d, %d\n", in, out)
	}
	// peek related
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr[:len(buf)])
		return len(buf), nil
	}
	pbuf, err := bc.Peek(1)
	if len(pbuf) != 1 || err != nil {
		t.Errorf("gtcp_test: BaseConn.Peek expected: 1, nil actual: %d, %+v\n", len(pbuf), err)
	}
	pbuf, err = bc.Peek(2)
	if len(pbuf) != 1 || err != gtcp.ErrBufferFull {
		t.Errorf("gtcp_test: BaseConn.Peek expected: 1, %+v actual: %d, %+v\n", gtcp.ErrBufferFull, len(pbuf), err)
	}
	n, err = bc.Read(buf)
	if n != 1 || err != nil {
		t.Errorf("gtcp_test: BaseConn.Read expected: 1,nil actual: %d, %+v\n", n, err)
	}
	n, err = bc.Read(buf)
	if n != 4 || err != nil {
		t.Errorf("gtcp_test: BaseConn.Read expected: 4,nil actual: %d, %+v\n", n, err)
	}
	dc.ReadFunc = func(buf []byte) (int, error) {
		return 0, errTest
	}
	pbuf, err = bc.Peek(4)
	if pbuf != nil || err != errTest {
		t.Errorf("gtcp_test: BaseConn.Peek expected: nil, errTest actual: %+v, %+v\n", pbuf, err)
	}
	// others
	bc.Flush()
	err = bc.Close()
	if err != nil {
		t.Errorf("gtcp_test: BaseConn.Close expected: nil actual: %+v\n", err)
	}
	bc.Close()
	bc = gtcp.NewBaseConn(dc)
	bc.Close()
}

func TestBufferedConn(t *testing.T) {
	t.Parallel()
	dc := &debugNetConn{}
	bc := gtcp.NewBaseConn(dc)
	var rr struct {
		buf []byte
		n   int
		err error
	}
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	dc.WriteFunc = func(buf []byte) (int, error) {
		rr.buf, rr.n, rr.err = buf, len(buf), nil
		return rr.n, rr.err
	}
	bufc := gtcp.NewBufferedConn(bc)
	buf := make([]byte, 4)
	n, err := bufc.Read(buf)
	if n != 4 || string(buf) != smashingStr || err != nil {
		t.Errorf("gtcp_test: BufferedConn.Read expected: 4, nil actual: %d, %+v\n", n, err)
	}
	n, err = bufc.Write([]byte(smashingStr))
	if n != 4 || err != nil {
		t.Errorf("gtcp_test: BufferedConn.Write expected: 4, nil actual: %d, %+v\n", n, err)
	}
	if rr.buf != nil {
		t.Errorf("gtcp_test: BufferedConn.Write should not call the wrapped BaseConn.Write\n")
	}
	err = bufc.Flush() // Flush trigger the wrapped BaseConn.Write
	if err != nil || rr.n != len(smashingStr) || rr.err != nil || string(rr.buf) != smashingStr {
		t.Errorf("gtcp_test: BufferedConn.Flush expected: nil actual: %+v\n", err)
	}
	err = bufc.Close()
	bc = gtcp.NewBaseConn(dc)
	bufc = gtcp.NewBufferedConn(bc) // whether bufio reused can be confirmed by coverage
	buf, err = bufc.Peek(2)
	if len(buf) != 2 || string(buf) != smashingStr[:2] || err != nil {
		t.Errorf("gtcp_test: BufferedConn.Peek expected: 2, nil actual: %d, %+v\n", n, err)
	}
	rr.buf, rr.n, rr.err = nil, 0, nil
	_, _ = bufc.Write([]byte(smashingStr))
	bufc.Close() // Close trigger the wrapped BaseConn.Write
	if rr.n != len(smashingStr) || rr.err != nil || string(rr.buf) != smashingStr {
		t.Errorf("gtcp_test: BufferedConn.Close should trigger the wrapped BaseConn.Write\n")
	}
	dc.WriteFunc = func(buf []byte) (int, error) {
		return 0, errTest
	}
	bc = gtcp.NewBaseConn(dc)
	bufc = gtcp.NewBufferedConn(bc)
	n, err = bufc.Write([]byte(smashingStr))
	if err != nil {
		t.Errorf("gtcp_test: BufferedConn.Write expected: 4, nil actual: %d, %+v\n", n, err)
	}
	err = bufc.Close()
	if err != errTest {
		t.Errorf("gtcp_test: BufferedConn.Close expected: errTest actual: %+v\n", err)
	}
}

func TestStatsConn(t *testing.T) {
	t.Parallel()
	dc := &debugNetConn{}
	bc := gtcp.NewBaseConn(dc)
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	dc.WriteFunc = func(buf []byte) (int, error) {
		return len(buf), nil
	}
	sc := gtcp.NewStatsConn(bc)
	buf := make([]byte, 4)
	n, err := sc.Read(buf)
	if n != 4 || string(buf) != smashingStr || err != nil {
		t.Errorf("gtcp_test: StatsConn.Read expected: 4, nil actual: %d, %+v\n", n, err)
	}
	_, _ = sc.Read(buf)
	_, _ = sc.Read(buf)
	n, err = sc.Write([]byte(smashingStr))
	if n != 4 || err != nil {
		t.Errorf("gtcp_test: StatsConn.Write expected: 4, nil actual: %d, %+v\n", n, err)
	}
	in, out := sc.Stats()
	if in != 12 || out != 4 {
		t.Errorf("gtcp_test: StatsConn.Stats expected: 12, 4 actual: %d, %d\n", in, out)
	}
}

func TestDebugConn(t *testing.T) {
	dc := &debugNetConn{}
	bc := gtcp.NewBaseConn(dc)
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	dc.WriteFunc = func(buf []byte) (int, error) {
		return len(buf), nil
	}
	c := gtcp.NewDebugConn(bc)
	buf := make([]byte, 4)
	n, err := c.Read(buf)
	if n != 4 || string(buf) != smashingStr || err != nil {
		t.Errorf("gtcp_test: DebugConn.Read expected: 4, nil actual: %d, %+v\n", n, err)
	}
	n, err = c.Write([]byte(smashingStr))
	if n != 4 || err != nil {
		t.Errorf("gtcp_test: DebugConn.Write expected: 4, nil actual: %d, %+v\n", n, err)
	}
	err = c.Close()
	if err != nil {
		t.Errorf("gtcp_test: DebugConn.Close expected: nil actual: %+v\n", err)
	}
}

func TestLayeredConn(t *testing.T) {
	dc := &debugNetConn{}
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	dc.WriteFunc = func(buf []byte) (int, error) {
		return len(buf), nil
	}
	c := gtcp.NewStatsConn(gtcp.NewDebugConn(gtcp.NewBaseConn(dc)))
	buf := make([]byte, 4)
	n, err := c.Read(buf)
	if n != 4 || string(buf) != smashingStr || err != nil {
		t.Errorf("gtcp_test: Read expected: 4, nil actual: %d, %+v\n", n, err)
	}
	n, err = c.Write([]byte(smashingStr))
	if n != 4 || err != nil {
		t.Errorf("gtcp_test: Write expected: 4, nil actual: %d, %+v\n", n, err)
	}
	err = c.Close()
	if err != nil {
		t.Errorf("gtcp_test: Close expected: nil actual: %+v\n", err)
	}
	in, out := c.Stats()
	if in != 4 || out != 4 {
		t.Errorf("gtcp_test: Stats expected: 4, 4 actual: %d, %d\n", in, out)
	}
	c = gtcp.NewBufferedConn(gtcp.NewStatsConn(gtcp.NewDebugConn(gtcp.NewBaseConn(dc))))
	n, err = c.Read(buf)
	if n != 4 || string(buf) != smashingStr || err != nil {
		t.Errorf("gtcp_test: Read expected: 4, nil actual: %d, %+v\n", n, err)
	}
	n, err = c.Write([]byte(smashingStr))
	if n != 4 || err != nil {
		t.Errorf("gtcp_test: Write expected: 4, nil actual: %d, %+v\n", n, err)
	}
	c.Write([]byte(smashingStr))
	c.Write([]byte(smashingStr))
	c.Write([]byte(smashingStr))
	in, out = c.Stats()
	if in != 4 || out != 0 {
		// Write still buffered
		t.Errorf("gtcp_test: Stats expected: 4, 0 actual: %d, %d\n", in, out)
	}
	err = c.Close()
	if err != nil {
		t.Errorf("gtcp_test: Close expected: nil actual: %+v\n", err)
	}
	in, out = c.Stats()
	if in != 4 || out != 16 {
		t.Errorf("gtcp_test: Stats expected: 4, 16 actual: %d, %d\n", in, out)
	}
}
