package gtcp_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cat2neat/gtcp"
)

const bufSize = 1024

func echoServer() *gtcp.Server {
	srv := &gtcp.Server{
		Addr: ":0",
		ConnHandler: func(ctx context.Context, conn gtcp.Conn) {
			buf := make([]byte, bufSize)
			done := false
			for !done {
				n, err := conn.Read(buf)
				if err != nil {
					done = true
					if err != io.EOF {
						return
					}
					// when EOF reached, there may have data read actually
					// go ahead to write back the last piece
				}

				conn.Write(buf[:n])
				err = conn.Flush()
				if err != nil {
					return
				}
			}
		},
	}
	return srv
}

func echoServerKeepAlive() *gtcp.Server {
	srv := &gtcp.Server{
		Addr: ":0",
	}
	srv.SetKeepAliveHandler(5*time.Millisecond,
		func(conn gtcp.Conn) error {
			buf := make([]byte, bufSize)
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					srv.Logger.Errorf("gtcp_test: err: %+v\n", err)
				}
				return err
			}
			conn.Write(buf[:n])
			err = conn.Flush()
			if err != nil {
				return err
			}
			return nil
		})
	return srv
}

func echoServerPipeline() *gtcp.Server {
	srv := &gtcp.Server{Addr: ":0"}
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

func doClientConn(conn net.Conn, src []string, closeWrite func() error, t testing.TB) {
	defer conn.Close()
	for _, s := range src {
		n, err := conn.Write([]byte(s))
		if n != len(s) || err != nil {
			t.Errorf("gtcp_test: err: %+v\n", err)
			return
		}
	}

	err := closeWrite()
	if err != nil {
		t.Errorf("gtcp_test: err: %+v\n", err)
		return
	}
	buf := make([]byte, bufSize)
	var total int
	for {
		n, err := conn.Read(buf[total:])
		if err != nil {
			if err == io.EOF {
				// when EOF reached, there may have data read actually
				total += n
				break
			} else {
				t.Errorf("gtcp_test: err: %+v\n", err)
				return
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

func doEchoClient(addr string, src []string, t testing.TB) {
	raw, err := net.Dial("tcp", addr)
	if err != nil {
		t.Errorf("gtcp_test: err: %+v\n", err)
		return
	}

	closeWrite := func() error {
		conn := raw.(*net.TCPConn)
		return conn.CloseWrite()
	}
	doClientConn(raw, src, closeWrite, t)
}

func TestServer(t *testing.T) {
	// echo:server
	srv := echoServer()
	var err error
	go func() {
		err = srv.ListenAndServe()
	}()

	time.Sleep(5 * time.Millisecond)
	if err != nil {
		t.Fatal("failed to do ListenAndServe", err)
	}

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
			doEchoClient(srv.ListenerAddr().String(), data, t)
			wg.Done()
		}()
	}
	wg.Wait()
	// should fail due to port collision
	srv.Addr = srv.ListenerAddr().String()
	err = srv.ListenAndServe()
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
	gtcp.ListenAndServe(":0", nil)
}

func doEchoClientTLS(addr string, src []string, t testing.TB) {
	config := tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", addr, &config)
	if err != nil {
		t.Errorf("gtcp_test: err: %+v\n", err)
		return
	}

	closeWrite := func() error {
		return conn.CloseWrite()
	}
	doClientConn(conn, src, closeWrite, t)
}

func TestTLSServer(t *testing.T) {
	// echo:server
	srv := echoServer()
	var err error
	go func() {
		err = srv.ListenAndServeTLS("certs/server_cert.pem", "certs/server_key.pem")
	}()

	time.Sleep(5 * time.Millisecond)
	if err != nil {
		t.Fatal("failed to do ListenAndServeTLS", err)
	}

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
			doEchoClientTLS(srv.ListenerAddr().String(), data, t)
			wg.Done()
		}()
	}
	wg.Wait()
	srv.Shutdown(context.Background())
	// safe to double close
	srv.Close()
}

func connectTCPClient(addr string, t *testing.T) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("gtcp_test: err: %+v\n", err)
	}
	// block until socket closed by server
	var buf [4]byte
	conn.Read(buf[:])
}

func TestServerPanicHandler(t *testing.T) {
	srv := gtcp.Server{
		Addr: ":0",
		ConnHandler: func(ctx context.Context, conn gtcp.Conn) {
			panic(gtcp.ErrAbortHandler)
		},
	}
	go srv.ListenAndServe()
	defer srv.Shutdown(context.Background())
	time.Sleep(5 * time.Millisecond)
	connectTCPClient(srv.ListenerAddr().String(), t)
}

func TestServerWithLimitter(t *testing.T) {
	srv := gtcp.Server{
		Addr: ":0",
		ConnHandler: func(ctx context.Context, conn gtcp.Conn) {
			time.Sleep(10 * time.Millisecond)
		},
		ConnTracker: &gtcp.WGConnTracker{},
		Limiters:    append([]gtcp.Limiter(nil), &gtcp.MaxConnLimiter{Max: 2}),
	}
	go srv.ListenAndServe()
	defer srv.Shutdown(context.Background())
	time.Sleep(5 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			connectTCPClient(srv.ListenerAddr().String(), t)
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestServerForceClose(t *testing.T) {
	srv := gtcp.Server{
		Addr: ":0",
		ConnHandler: func(ctx context.Context, conn gtcp.Conn) {
			select {
			case <-time.After(time.Second):
				t.Errorf("gtcp_test: unexpected timeout happen")
			case <-ctx.Done():
			}
		},
		ConnTracker: &gtcp.WGConnTracker{},
		NewConn:     gtcp.NewBufferedConn,
	}
	addr := srv.ListenerAddr()
	if addr != nil {
		t.Errorf("gtcp_test: expected: nil actual:%+v\n", addr)
	}
	var err error
	go func() {
		err = srv.ListenAndServe()
	}()
	time.Sleep(5 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			connectTCPClient(srv.ListenerAddr().String(), t)
			wg.Done()
		}()
	}
	time.Sleep(5 * time.Millisecond)
	srv.Close()
	wg.Wait()
	if err != gtcp.ErrServerClosed {
		t.Errorf("gtcp_test: err: %+v\n", err)
	}
}

func BenchmarkRawEchoServer(b *testing.B) {
	srv := &rawEchoServer{}
	benchEchoServer(srv, b)
}

func BenchmarkEchoServer(b *testing.B) {
	srv := echoServer()
	benchEchoServer(srv, b)
}

func BenchmarkEchoServerPipeline(b *testing.B) {
	srv := echoServerPipeline()
	benchEchoServer(srv, b)
}

func BenchmarkEchoServerKeepAlive(b *testing.B) {
	srv := echoServerKeepAlive()
	benchEchoServer(srv, b)
}

func benchEchoServer(srv echoer, b *testing.B) {
	errChan := make(chan error)
	go func() {
		errChan <- srv.ListenAndServe()
	}()
	data := []string{
		"foo",
		"bar",
		"buzz",
	}
	time.Sleep(10 * time.Millisecond)
	select {
	case err := <-errChan:
		b.Errorf("gtcp_test: err: %+v\n", err)
		return
	default:
	}
	defer srv.Close()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				doEchoClient(srv.ListenerAddr().String(), data, b)
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

type echoer interface {
	Close() error
	ListenerAddr() net.Addr
	ListenAndServe() error
}

type rawEchoServer struct {
	*net.TCPListener
}

func (s *rawEchoServer) Close() error {
	return s.TCPListener.Close()
}

func (s *rawEchoServer) ListenerAddr() net.Addr {
	if s.TCPListener != nil {
		return s.TCPListener.Addr()
	}
	return nil
}

func (s *rawEchoServer) ListenAndServe() error {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	s.TCPListener = ln.(*net.TCPListener)
	for {
		conn, err := s.TCPListener.AcceptTCP()
		if err != nil {
			return err
		}
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(3 * time.Minute)
		go func() {
			defer conn.Close()
			buf := make([]byte, bufSize)
			for {
				n, err := conn.Read(buf)
				if err != nil {
					return
				}
				_, err = conn.Write(buf[:n])
				if err != nil {
					return
				}
			}
		}()
	}
}
