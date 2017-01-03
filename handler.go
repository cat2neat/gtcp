package gtcp

import (
	"context"
	"io"
	"runtime"
	"time"
)

type (
	WriteFlusher interface {
		io.Writer
		Flush() error
	}
	ConnHandler    func(context.Context, Conn)
	ReqHandler     func(Conn) error
	PipelineReader func(io.Reader) ([]byte, error)
	PipelineWriter func([]byte, WriteFlusher) error
)

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
