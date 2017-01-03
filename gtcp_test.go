package gtcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cat2neat/gtcp"
)

type (
	debugNetConn struct {
		ReadFunc  func([]byte) (int, error)
		WriteFunc func([]byte) (int, error)
		Local     string
		Remote    string
	}
	nullLogger struct{}
)

const (
	smashingInt = 1979
)

var (
	errTest     = errors.New("errTest")
	smashingStr = strconv.FormatInt(smashingInt, 10)
	nl          = nullLogger{}
)

func (l nullLogger) Errorf(fmt string, args ...interface{}) {
	// nop
}

func (dc *debugNetConn) Read(b []byte) (int, error) {
	return dc.ReadFunc(b)
}

func (dc *debugNetConn) Write(b []byte) (int, error) {
	return dc.WriteFunc(b)
}

func (dc *debugNetConn) Close() error {
	return nil
}

func (dc *debugNetConn) LocalAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP(dc.Local),
		Port: smashingInt,
	}
}

func (dc *debugNetConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP(dc.Remote),
		Port: smashingInt,
	}
}

func (dc *debugNetConn) SetDeadline(t time.Time) error {
	return nil
}

func (dc *debugNetConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (dc *debugNetConn) SetWriteDeadline(t time.Time) error {
	return nil
}

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
	// panic
	c := make(chan struct{})
	go func() {
		defer func() {
			recover()
			c <- struct{}{}
		}()
		bc.Peek(1)
	}()
	<-c
	bc.Flush()
	err = bc.Close()
	if err != nil {
		t.Errorf("gtcp_test: BaseConn.Close expected: nil actual: %+v\n", err)
	}
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

func TestExponentialRetry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		expected time.Duration
	}{
		{expected: time.Millisecond},
		{expected: 2 * time.Millisecond},
		{expected: 4 * time.Millisecond},
		{expected: 8 * time.Millisecond},
		{expected: 16 * time.Millisecond},
		{expected: 20 * time.Millisecond},
		{expected: 20 * time.Millisecond},
	}
	var retry gtcp.Retry = gtcp.ExponentialRetry{
		InitialDelay: time.Millisecond,
		MaxDelay:     20 * time.Millisecond,
	}
	for i, test := range tests {
		ret := retry.Backoff(uint64(i))
		if test.expected != ret {
			t.Errorf("gtcp_test: ExponentialRetry.Backoff expected: %v actual: %v\n",
				test.expected, ret)
		}
	}
}

func TestMaxConnLimiter(t *testing.T) {
	t.Parallel()
	const max = 64
	ml := gtcp.MaxConnLimiter{Max: max}
	bc := gtcp.NewBaseConn(&debugNetConn{})

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
	// reset
	ml = gtcp.MaxConnLimiter{Max: max}
	wg := sync.WaitGroup{}
	var cnt uint32
	for i := 0; i < max/2; i++ {
		wg.Add(1)
		go func() {
			ml.OnConnected(bc)
			atomic.AddUint32(&cnt, 1)
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
				atomic.AddUint32(&cnt, 1)
			} else {
				ml.OnClosed(bc)
				atomic.AddUint32(&cnt, ^uint32(0))
			}
			wg.Done()
		}()
	}
	wg.Wait()
	if cnt != max/2 {
		t.Errorf("gtcp_test: MaxConnLimiter.OnConnected in pararrel expected: %d actual: %d\n",
			max/2,
			cnt)
	}
}

func TestTrafficStatistics(t *testing.T) {
	t.Parallel()
	dc := &debugNetConn{}
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	dc.WriteFunc = func(buf []byte) (int, error) {
		return len(buf), nil
	}
	c := gtcp.NewStatsConn(gtcp.NewBaseConn(dc))
	buf := make([]byte, 4)
	c.Read(buf)
	c.Write([]byte(smashingStr))

	const max = 64
	ts := gtcp.TrafficStatistics{}
	wg := sync.WaitGroup{}
	for i := 0; i < max; i++ {
		wg.Add(1)
		go func() {
			ts.AddConnStats(c)
			wg.Done()
		}()
	}
	wg.Wait()
	decoded := struct {
		InBytes  int `json:"in_bytes"`
		OutBytes int `json:"out_bytes"`
	}{}
	err := json.Unmarshal([]byte(ts.String()), &decoded)
	if err != nil {
		t.Errorf("gtcp_test: TrafficStatistics.String err: %+v\n", err)
	}
	if decoded.InBytes != 4*64 || decoded.OutBytes != 4*64 {
		t.Errorf("gtcp_test: TrafficStatistics.String raw: %s expected: 256, 256 actual: %d, %d\n",
			ts.String(),
			decoded.InBytes,
			decoded.OutBytes)
	}
	ts.Reset()
	err = json.Unmarshal([]byte(ts.String()), &decoded)
	if err != nil {
		t.Errorf("gtcp_test: TrafficStatistics.String err: %+v\n", err)
	}
	if decoded.InBytes != 0 || decoded.OutBytes != 0 {
		t.Errorf("gtcp_test: TrafficStatistics.String raw: %s expected: 0, 0 actual: %d, %d\n",
			ts.String(),
			decoded.InBytes,
			decoded.OutBytes)
	}
}

func TestLogger(t *testing.T) {
	gtcp.DefaultLogger.Errorf("gtcp_test: t=%+v\n", t)
}

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

func TestSetKeepAliveHandler(t *testing.T) {
	t.Parallel()
	dc := &debugNetConn{}
	dc.ReadFunc = func(buf []byte) (int, error) {
		copy(buf, smashingStr)
		return len(smashingStr), nil
	}
	c := gtcp.NewBufferedConn(gtcp.NewBaseConn(dc))
	srv := gtcp.Server{Logger: gtcp.DefaultLogger}
	first := true
	srv.SetKeepAliveHandler(time.Millisecond, func(conn gtcp.Conn) error {
		if first {
			first = false
			return nil
		} else {
			// Get the buffer in BufferedConn empty
			buf := make([]byte, len(smashingStr))
			conn.Read(buf)
			// Get the second Peek failed
			dc.ReadFunc = func(buf []byte) (int, error) {
				return 0, errTest
			}
			return nil
		}
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
		} else {
			return nil, errTest
		}
	}, func(buf []byte, wf gtcp.WriteFlusher) error {
		new := atomic.AddUint32(&cnt, 1)
		if new%2 == 0 {
			return nil
		} else {
			return errTest
		}
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
		return nil, errTest
	}, func(buf []byte, wf gtcp.WriteFlusher) error {
		return nil
	})
	srv.ConnHandler(context.Background(), c)
}

const bufSize = 1024

func echoServer() *gtcp.Server {
	srv := &gtcp.Server{NewConn: gtcp.NewBufferedConn}
	srv.SetPipelineHandler(32,
		func(r io.Reader) ([]byte, error) {
			buf := make([]byte, bufSize)
			n, err := r.Read(buf)
			return buf[:n], err
		}, func(buf []byte, wf gtcp.WriteFlusher) error {
			wf.Write(buf)
			return wf.Flush()
		})
	return srv
}

func doEchoClient(src []string, t *testing.T) {
	addr := &net.TCPAddr{
		IP:   net.ParseIP("localhost"),
		Port: smashingInt,
	}
	time.Sleep(time.Millisecond)
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		t.Fatalf("gtcp_test: err: %+v\n", err)
	}
	defer conn.Close()
	for _, s := range src {
		n, err := conn.Write([]byte(s))
		if n != len(s) || err != nil {
			t.Fatalf("gtcp_test: err: %+v\n", err)
		}
	}
	err = conn.CloseWrite()
	if err != nil {
		t.Fatalf("gtcp_test: err: %+v\n", err)
	}
	buf := make([]byte, bufSize)
	var total int
	for {
		n, err := conn.Read(buf[total:])
		if err != nil {
			if err == io.EOF {
				break
			} else {
				t.Fatalf("gtcp_test: err: %+v\n", err)
			}
		}
		total += n
	}
	expected := strings.Join(src, "")
	actual := string(buf[:total])
	if actual != expected {
		t.Errorf("gtcp_test: expected: %s, actual: %s\n", expected, actual)
	}
}

func TestServer(t *testing.T) {
	// echo:server
	srv := echoServer()
	go srv.ListenAndServe()
	// echo:client
	data := []string{
		"foo",
		"bar",
		"buzz",
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			doEchoClient(data, t)
			wg.Done()
		}()
	}
	wg.Wait()
	// should fail due to port collision
	err := srv.ListenAndServe()
	if err == nil {
		t.Errorf("gtcp_test: ListenAndServe should fail due to port collision\n")
	}
	srv.Shutdown(context.Background())
	// safe to double close
	srv.Close()
}

func TestServerNilHandler(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Errorf("gtcp_test: Nil handler should cause panic\n")
		}
	}()
	gtcp.ListenAndServe(":1980", nil)
}

func doTcpClient(port int, t *testing.T) {
	addr := &net.TCPAddr{
		IP:   net.ParseIP("localhost"),
		Port: port,
	}
	time.Sleep(time.Millisecond)
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		t.Fatalf("gtcp_test: err: %+v\n", err)
	}
	conn.Close()
}

func TestServerPanicHandler(t *testing.T) {
	go gtcp.ListenAndServe(":1981", func(ctx context.Context, conn gtcp.Conn) {
		panic("gtcp: intentional panic")
	})
	doTcpClient(1981, t)
}

func TestServerWithLimitter(t *testing.T) {
	srv := gtcp.Server{
		Addr: ":1982",
		ConnHandler: func(ctx context.Context, conn gtcp.Conn) {
			time.Sleep(2 * time.Millisecond)
			conn.Close()
		},
		ConnTracker: &gtcp.WGConnTracker{},
		Limiters:    append([]gtcp.Limiter(nil), &gtcp.MaxConnLimiter{Max: 2}),
	}
	go srv.ListenAndServe()
	defer srv.Shutdown(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			doTcpClient(1982, t)
			wg.Done()
		}()
	}
	wg.Wait()
}
