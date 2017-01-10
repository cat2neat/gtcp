package gtcp_test

import (
	"errors"
	"net"
	"strconv"
	"time"
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
