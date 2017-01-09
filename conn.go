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
	// Conn is the interface that wraps connetcion specific operations
	// in addition to net.Conn.
	Conn interface {
		net.Conn
		// Flush writes any buffered data to the underlying net.Conn.
		Flush() error

		// SetCancelFunc sets context.CancelFunc that called automatically
		// when Read/Write failed.
		SetCancelFunc(context.CancelFunc)

		// Stats returns in/out bytes gone through this Conn.
		Stats() (int64, int64)

		// SetIdle sets whether this Conn is idle or not.
		// It's used to realize gtcp.Server.Shutdown.
		SetIdle(bool)

		// IsIdle returns whether this Conn is idle or not.
		// It's used to realize gtcp.Server.Shutdown.
		IsIdle() bool

		// Peek returns the next n bytes without advancing the reader.
		Peek(int) ([]byte, error)
	}

	// NewConn is the function that takes a Conn and returns another Conn
	// that enables to generate multi layered Conn.
	// ex)
	//		func(conn Conn) Conn {
	//			return gtcp.NewBufferedConn(gtcp.NewStatsConn(conn))
	//		}
	NewConn func(Conn) Conn

	baseConn struct {
		net.Conn
		CancelFunc context.CancelFunc
		idle       atomicBool
		hasByte    bool
		byteBuf    [1]byte
		mu         sync.Mutex // guard Close invoked from multiple goroutines
	}

	// BufferedConn implements Conn that wraps Conn in bufio.Reader|Writer.
	BufferedConn struct {
		Conn
		bufr *bufio.Reader
		bufw *bufio.Writer
		once sync.Once
	}

	// StatsConn implements Conn that holds in/out bytes.
	StatsConn struct {
		Conn
		// InBytes stores incomming bytes.
		InBytes int64
		// OutBytes stores outgoing bytes.
		OutBytes int64
	}

	// DebugConn implements Conn that logs every Read/Write/Close operations using standard log.
	DebugConn struct {
		Conn
	}

	atomicBool int32
)

var (
	// ErrBufferFull returned when Peek takes a larger value than its buffer.
	ErrBufferFull = errors.New("gtcp: buffer full")
)

var (
	readerPool sync.Pool
	writerPool sync.Pool
	basePool   sync.Pool
)

func (b *atomicBool) isSet() bool { return atomic.LoadInt32((*int32)(b)) != 0 }
func (b *atomicBool) setTrue()    { atomic.StoreInt32((*int32)(b), 1) }
func (b *atomicBool) setFalse()   { atomic.StoreInt32((*int32)(b), 0) }

// NewBaseConn takes net.Conn and returns Conn that wraps net.Conn.
// It's exported only for test purpose.
func NewBaseConn(conn net.Conn) Conn {
	if v := basePool.Get(); v != nil {
		bc := v.(*baseConn)
		bc.Conn = conn
		return bc
	} else {
		return &baseConn{
			Conn: conn,
		}
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

func (bc *baseConn) Close() (err error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if bc.Conn == nil {
		return
	}
	err = bc.Conn.Close()
	bc.Conn = nil
	basePool.Put(bc)
	return
}

// NewBufferedConn returns Conn wraps a given Conn in bufio.Reader|Writer.
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

// Read reads data into buf using internal bufio.Reader.
// It returns the number of bytes read into buf.
func (b *BufferedConn) Read(buf []byte) (n int, err error) {
	n, err = b.bufr.Read(buf)
	return
}

// Write writes the contents of buf into the internal bufio.Writer.
// It returns the number of bytes written.
func (b *BufferedConn) Write(buf []byte) (n int, err error) {
	n, err = b.bufw.Write(buf)
	return
}

// Close closes the internal bufio.Reader|Writer and also Conn.
// It's protected by sync.Once as multiple goroutines can call
// especially in case using gtcp.Server.Close|Shutdown.
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

// Flush writes any buffered data to the underlying Conn.
func (b *BufferedConn) Flush() (err error) {
	return b.bufw.Flush()
}

// Peek returns the next n bytes without advancing the reader.
func (b *BufferedConn) Peek(n int) ([]byte, error) {
	return b.bufr.Peek(n)
}

// NewStatsConn returns Conn that holds in/out bytes.
func NewStatsConn(conn Conn) Conn {
	return &StatsConn{Conn: conn}
}

// Read reads data into buf and returns the number of bytes read into buf.
// It also adds InBytes and bytes read up.
func (s *StatsConn) Read(buf []byte) (n int, err error) {
	n, err = s.Conn.Read(buf)
	s.InBytes += int64(n)
	return
}

// Write writes the contents of buf and returns the number of bytes written.
// It also adds OutBytes and bytes written up.
func (s *StatsConn) Write(buf []byte) (n int, err error) {
	n, err = s.Conn.Write(buf)
	s.OutBytes += int64(n)
	return
}

// Stats returns in/out bytes gone through this Conn.
func (s *StatsConn) Stats() (int64, int64) {
	return s.InBytes, s.OutBytes
}

// NewDebugConn returns Conn that logs debug information using standard log.
func NewDebugConn(conn Conn) Conn {
	return &DebugConn{Conn: conn}
}

// Read reads data into buf and returns the number of bytes read into buf.
// It also outputs debug information before/after calling internal Conn.Read.
func (d *DebugConn) Read(buf []byte) (n int, err error) {
	log.Printf("Read(%d) = ....", len(buf))
	n, err = d.Conn.Read(buf)
	log.Printf("Read(%d) = %d, %v", len(buf), n, err)
	return
}

// Write writes the contents of buf and returns the number of bytes written.
// It also outputs debug information before/after calling internal Conn.Write.
func (d *DebugConn) Write(buf []byte) (n int, err error) {
	log.Printf("Write(%d) = ....", len(buf))
	n, err = d.Conn.Write(buf)
	log.Printf("Write(%d) = %d, %v", len(buf), n, err)
	return
}

// Closes closes the internal Conn.
// It also outputs debug information before/after calling internal Conn.Close().
func (d *DebugConn) Close() (err error) {
	log.Printf("Close() = ...")
	err = d.Conn.Close()
	log.Printf("Close() = %v", err)
	return
}
