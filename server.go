// Goals:
// - can be used in the same manner with http.Server
//   - Make API as much compatible as possible
//   - Make the zero value useful
// - inherit as much battle tested code from net/http
// - much flexiblity with built-in components
//   - ConnHandler
//     - Raw
//     - KeepAliveHandler
//     - PipelineHandler
//   - ConnTracker
//     - MapConnTracker(same with net/http
//     - WGConnTracker(fast, less memory than Map*
//   - NewConn (BufferedConn, StatsConn, DebugConn
//     - can be layered
//   - Logger
//   - Retry
//   - Statistics (# of connections, in bytes, out bytes
//   - Limiter (Connection, IP
// - Get GC pressure as little as possible with sync.Pool
// - Zero 3rd party depentencies
// TODO:
// - tls
// - multiple listeners
package gtcp

import (
	"context"
	"errors"
	"net"
	"runtime"
	"sync"
	"time"
)

type (
	Server struct {
		Addr string

		ConnHandler ConnHandler

		// Configurable components
		NewConn     NewConn
		ConnTracker ConnTracker
		Logger      Logger
		Retry       Retry
		Limiters    []Limiter
		Statistics  Statistics

		listener net.Listener

		mu       sync.Mutex
		doneChan chan struct{}
	}
)

var (
	ErrServerClosed = errors.New("gtcp: Server closed")
	ErrAbortHandler = errors.New("gtcp: abort Handler")
)

func (s *Server) getDoneChan() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getDoneChanLocked()
}

func (s *Server) getDoneChanLocked() chan struct{} {
	if s.doneChan == nil {
		s.doneChan = make(chan struct{})
	}
	return s.doneChan
}

func (s *Server) closeDoneChanLocked() {
	ch := s.getDoneChanLocked()
	select {
	case <-ch:
		// Already closed. Don't close again.
	default:
		// Safe to close here. We're the only closer, guarded
		// by s.mu.
		close(ch)
	}
}

func (s *Server) Close() (err error) {
	s.mu.Lock()
	s.closeDoneChanLocked()
	s.mu.Unlock()
	err = s.listener.Close()
	s.ConnTracker.Close()
	return
}

func (s *Server) Shutdown(ctx context.Context) (err error) {
	s.mu.Lock()
	s.closeDoneChanLocked()
	s.mu.Unlock()
	err = s.listener.Close()
	s.ConnTracker.Shutdown(ctx)
	return
}

func (s *Server) ListenAndServe() error {
	addr := s.Addr
	if addr == "" {
		addr = ":1979"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
}

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func ListenAndServe(addr string, handler ConnHandler) error {
	server := &Server{Addr: addr, ConnHandler: handler}
	return server.ListenAndServe()
}

func (s *Server) Serve(l net.Listener) error {
	var retry uint64
	defer l.Close()

	if s.ConnHandler == nil {
		panic("gtcp: nil handler")
	}

	// set reasonable default to each component
	if s.ConnTracker == nil {
		s.ConnTracker = NewMapConnTracker()
	}
	if s.Logger == nil {
		s.Logger = DefaultLogger
	}
	if s.Retry == nil {
		s.Retry = DefaultRetry
	}
	if s.Statistics == nil {
		s.Statistics = &TrafficStatistics{}
	}

	s.listener = l

	base := context.Background()
	for {
		rw, e := l.Accept()
		if e != nil {
			select {
			case <-s.getDoneChan():
				// @todo notify children contexts
				return ErrServerClosed
			default:
			}
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				delay := s.Retry.Backoff(retry)
				s.Logger.Errorf("gtcp: Accept error: %v; retrying in %v", e, delay)
				time.Sleep(delay)
				retry += 1
				continue
			}
			return e
		}
		retry = 0
		var conn Conn = NewBaseConn(rw)
		if s.NewConn != nil {
			conn = s.NewConn(conn)
		}
		s.serve(base, conn)
	}
}

func (s *Server) serve(ctx context.Context, conn Conn) {
	for _, limiter := range s.Limiters {
		if limiter.OnConnected(conn) == false {
			s.Logger.Errorf("gtcp: connection refused %v: by limiter %+v", conn.RemoteAddr(), limiter)
			conn.Close()
			return
		}
	}
	s.ConnTracker.AddConn(conn)
	go func() {
		defer func() {
			for _, limiter := range s.Limiters {
				limiter.OnClosed(conn)
			}
			s.ConnTracker.DelConn(conn)
			s.Statistics.AddConnStats(conn)
			if err := recover(); err != nil && err != ErrAbortHandler {
				const size = 64 << 10
				buf := make([]byte, size)
				buf = buf[:runtime.Stack(buf, false)]
				s.Logger.Errorf("gtcp: panic serving %v: %v\n%s", conn.RemoteAddr(), err, buf)
			}
			conn.Close()
		}()
		// context per connection
		ctx, cancel := context.WithCancel(ctx)
		conn.SetCancelFunc(cancel)
		defer cancel()
		s.ConnHandler(ctx, conn)
	}()
}
