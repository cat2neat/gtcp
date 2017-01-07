package gtcp

import (
	"bufio"
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
)

type (
	Conn interface {
		net.Conn
		Flush() error
		SetCancelFunc(context.CancelFunc)
		Stats() (int64, int64)
		SetIdle(bool)
		IsIdle() bool
		Peek(int) ([]byte, error)
	}

	NewConn func(Conn) Conn

	baseConn struct {
		net.Conn
		CancelFunc context.CancelFunc
		idle       atomicBool
		hasByte    bool
		byteBuf    [1]byte
	}

	BufferedConn struct {
		Conn
		bufr *bufio.Reader
		bufw *bufio.Writer
		once sync.Once
	}

	StatsConn struct {
		Conn
		InBytes  int64
		OutBytes int64
	}

	DebugConn struct {
		Conn
	}

	atomicBool int32
)

var (
	ErrBufferFull = errors.New("gtcp: buffer full")
)

var (
	readerPool sync.Pool
	writerPool sync.Pool
)

func (b *atomicBool) isSet() bool { return atomic.LoadInt32((*int32)(b)) != 0 }
func (b *atomicBool) setTrue()    { atomic.StoreInt32((*int32)(b), 1) }
func (b *atomicBool) setFalse()   { atomic.StoreInt32((*int32)(b), 0) }

func NewBaseConn(conn net.Conn) Conn {
	return &baseConn{
		Conn: conn,
	}
}

func (bc *baseConn) Read(buf []byte) (n int, err error) {
	if bc.hasByte {
		buf[0] = bc.byteBuf[0]
		bc.hasByte = false
		return 1, nil
	}
	n, err = bc.Conn.Read(buf)
	if err != nil && bc.CancelFunc != nil {
		bc.CancelFunc()
	}
	return
}

func (bc *baseConn) Write(buf []byte) (n int, err error) {
	n, err = bc.Conn.Write(buf)
	if err != nil && bc.CancelFunc != nil {
		bc.CancelFunc()
	}
	return
}

func (bc *baseConn) Flush() error {
	return nil
}

func (bc *baseConn) SetCancelFunc(cancel context.CancelFunc) {
	bc.CancelFunc = cancel
}

func (bc *baseConn) Stats() (int64, int64) {
	return 0, 0
}

func (bc *baseConn) SetIdle(idle bool) {
	if idle {
		bc.idle.setTrue()
	} else {
		bc.idle.setFalse()
	}
}

func (bc *baseConn) IsIdle() bool {
	return bc.idle.isSet()
}

func (bc *baseConn) Peek(n int) (buf []byte, err error) {
	if n > 1 {
		err = ErrBufferFull
	}
	if bc.hasByte {
		return bc.byteBuf[:], err
	} else {
		rn, rerr := bc.Conn.Read(bc.byteBuf[:])
		if rn == 1 {
			bc.hasByte = true
			buf = bc.byteBuf[:]
		}
		if rerr != nil {
			err = rerr // override
		}
		return
	}
}

func NewBufferedConn(conn Conn) Conn {
	var br *bufio.Reader
	var bw *bufio.Writer
	if v := readerPool.Get(); v != nil {
		br = v.(*bufio.Reader)
		br.Reset(conn)
	} else {
		br = bufio.NewReader(conn)
	}
	if v := writerPool.Get(); v != nil {
		bw = v.(*bufio.Writer)
		bw.Reset(conn)
	} else {
		bw = bufio.NewWriter(conn)
	}
	return &BufferedConn{
		Conn: conn,
		bufr: br,
		bufw: bw,
	}
}

func (b *BufferedConn) Read(buf []byte) (n int, err error) {
	n, err = b.bufr.Read(buf)
	return
}

func (b *BufferedConn) Write(buf []byte) (n int, err error) {
	n, err = b.bufw.Write(buf)
	return
}

func (b *BufferedConn) Close() (err error) {
	b.once.Do(func() {
		b.bufr.Reset(nil)
		readerPool.Put(b.bufr)
		b.bufr = nil
		err = b.bufw.Flush()
		b.bufw.Reset(nil)
		writerPool.Put(b.bufw)
		b.bufw = nil
		e := b.Conn.Close()
		if err == nil {
			err = e
		}
	})
	return
}

func (b *BufferedConn) Flush() (err error) {
	return b.bufw.Flush()
}

func (b *BufferedConn) Peek(n int) ([]byte, error) {
	return b.bufr.Peek(n)
}

func NewStatsConn(conn Conn) Conn {
	return &StatsConn{Conn: conn}
}

func (s *StatsConn) Read(buf []byte) (n int, err error) {
	n, err = s.Conn.Read(buf)
	s.InBytes += int64(n)
	return
}

func (s *StatsConn) Write(buf []byte) (n int, err error) {
	n, err = s.Conn.Write(buf)
	s.OutBytes += int64(n)
	return
}

func (s *StatsConn) Stats() (int64, int64) {
	return s.InBytes, s.OutBytes
}

func NewDebugConn(conn Conn) Conn {
	return &DebugConn{Conn: conn}
}

func (d *DebugConn) Read(buf []byte) (n int, err error) {
	log.Printf("Read(%d) = ....", len(buf))
	n, err = d.Conn.Read(buf)
	log.Printf("Read(%d) = %d, %v", len(buf), n, err)
	return
}

func (d *DebugConn) Write(buf []byte) (n int, err error) {
	log.Printf("Write(%d) = ....", len(buf))
	n, err = d.Conn.Write(buf)
	log.Printf("Write(%d) = %d, %v", len(buf), n, err)
	return
}

func (d *DebugConn) Close() (err error) {
	log.Printf("Close() = ...")
	err = d.Conn.Close()
	log.Printf("Close() = %v", err)
	return
}
