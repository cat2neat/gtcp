package gtcp

import (
	"context"
	"io"
	"runtime"
	"time"
)

type (
	// WriteFlusher is the interface that wraps write operations on Conn.
	WriteFlusher interface {
		io.Writer
		// Flush writes any buffered data to the underlying Conn.
		Flush() error
	}

	// ConnHandler is the callback function called when Conn gets ready to communicate with peer.
	// You can use ConnHandler to gain full control on socket.
	ConnHandler func(context.Context, Conn)

	// ReqHandler is the callback function used with SetKeepAliveHandler.
	// It's called when Conn gets ready to communicate specifically
	// - accepted by listener
	// - receiving one more byte while being in keepalive
	// You can use ReqHandler with SetKeepAliveHandler to implement keepalive easily.
	ReqHandler func(Conn) error

	// PipelineReader is the callback function used with SetPipelineHandler.
	// SetPipelineHandler enables to implement protocol pipelining easily.
	// It's used for reading part of pipelining and dispatch meaningful []byte to
	// PipelineWriter via return value.
	PipelineReader func(io.Reader) ([]byte, error)

	// PipelineWriter is the callback function used with SetPipelineHandler.
	// SetPipelineHandler enables to implement protocol pipelining easily.
	// It's used for writing part of pipelining and
	// called when receiving a meaningful []byte from PipelineReader.
	PipelineWriter func([]byte, WriteFlusher) error
)

// SetKeepAliveHandler enables to implement keepalive easily.
// It call h and call again repeatedly if receiving one more byte while waiting idle time
// or stop communicating.
// It also stop when detecting listener got closed.
func (s *Server) SetKeepAliveHandler(idle time.Duration, h ReqHandler) {
	s.ConnHandler = func(ctx context.Context, conn Conn) {
		for {
			err := h(conn)
			if err != nil {
				// don't reuse if some error happened
				return
			}
			select {
			case <-ctx.Done():
				// canceled by parent
				return
			default:
			}
			conn.SetIdle(true)
			conn.SetReadDeadline(time.Now().Add(idle))
			if _, err = conn.Peek(1); err != nil {
				return
			}
			conn.SetIdle(false)
			conn.SetReadDeadline(time.Time{})
		}
	}
}

// SetPipelineHandler enables to implement protocol pipelining easily.
// It combines pr and pw with a buffered channel that has numBuf.
// pr need to implement reading part of pipelining and dispatch meaningful []byte to pw.
// pw need to implement writing part of pipelining.
// It stops if pr returns nil buf or any error.
// It also stop when detecting listener got closed.
func (s *Server) SetPipelineHandler(
	numBuf int,
	pr PipelineReader,
	pw PipelineWriter) {
	s.ConnHandler = func(ctx context.Context, conn Conn) {
		packet := make(chan []byte, numBuf)
		go func() {
			defer func() {
				if err := recover(); err != nil && err != ErrAbortHandler {
					const size = 64 << 10
					buf := make([]byte, size)
					buf = buf[:runtime.Stack(buf, false)]
					s.Logger.Errorf("gtcp: panic serving %v: %v\n%s", conn.RemoteAddr(), err, buf)
				}
				close(packet)
			}()
			for {
				// reader
				buf, err := pr(conn)
				if buf == nil || err != nil {
					return
				}
				select {
				case packet <- buf:
				case <-ctx.Done():
					// canceled by parent
					return
				}
			}

		}()
		// writer
		for {
			select {
			case buf := <-packet:
				if buf == nil {
					// context canceled or tcp session closed or error happened at reader
					return
				}
				err := pw(buf, conn)
				if err != nil {
					// continue until reader failed
					continue
				}
			}
		}
	}
}
