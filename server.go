package gtcp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"runtime"
	"sync"
	"time"
)

type (
	// Server defines parameters for running a gtcp server.
	// The zero value for Server is a valid configuration.
	Server struct {
		// Addr to listen on, ":1979" if empty.
		Addr string

		// ConnHandler handles a tcp connection accepted.
		// It can be used not only directly setting ConnHandler but
		// - Server.SetKeepAliveHandler
		// - Server.SetPipelineHandler
		// Panic occurred if empty.
		ConnHandler ConnHandler

		// Configurable components

		// NewConn that applied to each tcp connection accepted by listener.
		// It can be
		// - gtcp.NewBufferedConn
		// - gtcp.NewStatsConn
		// - gtcp.NewDebugConn
		// also can be layered like the below.
		//		func(conn Conn) Conn {
		//			return gtcp.NewBufferedConn(gtcp.NewStatsConn(conn))
		//		}
		// None NewConn is applied if empty.
		NewConn NewConn

		// ConnTracker that handles active connections.
		// It can be
		// - gtcp.MapConnTracker
		// - gtcp.WGConnTracker
		// gtcp.MapConnTracker is set if empty.
		ConnTracker ConnTracker

		// Logger that logs if an error occurred in gtcp.
		// It can be
		// - gtcp.BuiltinLogger
		// gtcp.DefaultLogger is set if empty.
		Logger Logger

		// Retry that handles the retry interval when Accept on listner failed.
		// It can be
		// - gtcp.ExponentialRetry
		// gtcp.DefaultRetry is set if empty.(it behaves in the same manner with net/http/Server.
		Retry Retry

		// Limiters that limits connections.
		// It can be
		// - gtcp.MaxConnLimiter
		// also multiple limiters can be set.
		// None Limiter is set if empty.
		Limiters []Limiter

		// Statistics that measures some statistics.
		// It can be
		// - gtcp.TrafficStatistics
		// gtcp.TrafficStatistics is set if empty.
		Statistics Statistics

		listener net.Listener

		mu       sync.Mutex
		doneChan chan struct{}
	}
)

var (
	// ErrServerClosed returned when listener got closed through Close/Shutdown.
	ErrServerClosed = errors.New("gtcp: Server closed")
	// ErrAbortHandler is a sentinel panic value to abort a handler.
	// panicking with ErrAbortHandler also suppresses logging of a stack
	// trace to the server's error log.
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

// Close immediately closes the listner and any
// connections tracked by ConnTracker.
// Close returns any error returned from closing the listner.
func (s *Server) Close() (err error) {
	s.mu.Lock()
	s.closeDoneChanLocked()
	s.mu.Unlock()
	err = s.listener.Close()
	s.ConnTracker.Close()
	return
}

// Shutdown gracefully shuts down the server without interrupting any
// active connections.
// If the provided context expires before the shutdown is complete,
// then the context's error is returned.
func (s *Server) Shutdown(ctx context.Context) (err error) {
	s.mu.Lock()
	s.closeDoneChanLocked()
	s.mu.Unlock()
	err = s.listener.Close()
	s.ConnTracker.Shutdown(ctx)
	return
}

// ListenerAddr returns the listner.Addr() or nil if listner is empty.
func (s *Server) ListenerAddr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{
		Certificates:             []tls.Certificate{cert},
		PreferServerCipherSuites: true,
	}

	ln, err := s.listen()
	if err != nil {
		return err
	}

	ln = tls.NewListener(ln, tlsConfig)
	return s.Serve(ln)
}

// ListenAndServe listens on the TCP network address Addr and then
// calls Serve to handle requests on incoming connections.
// If Addr is blank, ":1979" is used.
// ListenAndServe always returns a non-nil error.
func (s *Server) ListenAndServe() error {
	ln, err := s.listen()
	if err != nil {
		return err
	}

	return s.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
}

func (s *Server) listen() (net.Listener, error) {
	addr := s.Addr
	if addr == "" {
		addr = ":1979"
	}
	return net.Listen("tcp", addr)
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

// ListenAndServe listens on the TCP network address addr and then
// calls Serve to handle requests on incoming connections.
// ListenAndServe always returns a non-nil error.
func ListenAndServe(addr string, handler ConnHandler) error {
	server := &Server{Addr: addr, ConnHandler: handler}
	return server.ListenAndServe()
}

// Serve accepts incoming connections on the Listener l, creating a
// new service goroutine for each.
// The service goroutines call ConnHandler to reply to them.
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

	// context per listener
	// notify goroutines executing ConnHandler that your listner got closed
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		rw, e := l.Accept()
		if e != nil {
			select {
			case <-s.getDoneChan():
				return ErrServerClosed
			default:
			}
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				delay := s.Retry.Backoff(retry)
				s.Logger.Errorf("gtcp: Accept error: %v; retrying in %v", e, delay)
				time.Sleep(delay)
				retry++
				continue
			}
			return e
		}
		retry = 0
		conn := NewBaseConn(rw)
		if s.NewConn != nil {
			conn = s.NewConn(conn)
		}
		s.serve(ctx, conn)
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
