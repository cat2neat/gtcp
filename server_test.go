package gtcp_test

import (
	"context"
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"os"
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

func doServer(listen func(*gtcp.Server) error, echoClientFunc func(string, []string, testing.TB), t *testing.T) {
	// echo:server
	srv := echoServer()
	var err error
	go func() {
		err = listen(srv)
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
			echoClientFunc(srv.ListenerAddr().String(), data, t)
			wg.Done()
		}()
	}
	wg.Wait()
	// should fail due to port collision
	srv.Addr = srv.ListenerAddr().String()
	err = listen(srv)
	if err == nil {
		t.Errorf("gtcp_test: ListenAndServe should fail due to port collision\n")
	}
	srv.Shutdown(context.Background())
	// safe to double close
	srv.Close()
}

func TestServer(t *testing.T) {
	listen := func(svc *gtcp.Server) error {
		return svc.ListenAndServe()
	}
	doServer(listen, doEchoClient, t)
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
	certFile := "server_cert.pem"
	keyFile := "server_key.pem"
	if err := createCerts(certFile, keyFile); err != nil {
		t.Errorf("failed to create cert files", err)
	}

	defer deleteCerts(certFile, keyFile)

	listen := func(svc *gtcp.Server) error {
		return svc.ListenAndServeTLS(certFile, keyFile)
	}

	doServer(listen, doEchoClientTLS, t)
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

const (
	certData = `-----BEGIN CERTIFICATE-----
MIIEVDCCAzygAwIBAgIJAJc1TOqaTTUpMA0GCSqGSIb3DQEBBQUAMHkxCzAJBgNV
BAYTAkNIMQswCQYDVQQIEwJTSDELMAkGA1UEBxMCU0gxDDAKBgNVBAoTA1NQTDEN
MAsGA1UECxMEU09MTjESMBAGA1UEAxMJbG9jYWxob3N0MR8wHQYJKoZIhvcNAQkB
FhBrY2hlbkBzcGx1bmsuY29tMB4XDTE2MDUxMTIxNTIwNFoXDTE3MDUxMTIxNTIw
NFoweTELMAkGA1UEBhMCQ0gxCzAJBgNVBAgTAlNIMQswCQYDVQQHEwJTSDEMMAoG
A1UEChMDU1BMMQ0wCwYDVQQLEwRTT0xOMRIwEAYDVQQDEwlsb2NhbGhvc3QxHzAd
BgkqhkiG9w0BCQEWEGtjaGVuQHNwbHVuay5jb20wggEiMA0GCSqGSIb3DQEBAQUA
A4IBDwAwggEKAoIBAQC2beOk6AGWGzU6iyIlKWEzf/4sUFFc/Ad29RbwhlkSoW1j
+1npnZspM/3VAi2Ll/k3YbNuMaQ1vDChDt9BQO1BD+IFKqO/tLd5+Rrj003r1uKi
uPYviRMkIGnrr361S4ASZieqedkpi585ijxVx9uY2PjGXFdFU6AxCyLDzQ36A8N+
Mn1T2zMO9Koem+/0xkFcBhKyvGFmWM/oYfCtupiWK3tmIwnQ+ldh41srsPb+DAQ3
Pqk7ssEumrgnsG/tkNosgkEbuODtcaTy2NTpuo4h1Lgzbs3yYvqwDhHe8kdijRyz
NczfucK63Uy0mDqy+t2QF8nZ5AMNWzHyuIegQmanAgMBAAGjgd4wgdswHQYDVR0O
BBYEFIEMpJ6XYlx5p0LOM6uvlZrcyPT8MIGrBgNVHSMEgaMwgaCAFIEMpJ6XYlx5
p0LOM6uvlZrcyPT8oX2kezB5MQswCQYDVQQGEwJDSDELMAkGA1UECBMCU0gxCzAJ
BgNVBAcTAlNIMQwwCgYDVQQKEwNTUEwxDTALBgNVBAsTBFNPTE4xEjAQBgNVBAMT
CWxvY2FsaG9zdDEfMB0GCSqGSIb3DQEJARYQa2NoZW5Ac3BsdW5rLmNvbYIJAJc1
TOqaTTUpMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAKs8n/CIihGw
nDo8kXOFfzC0+mSWgD2Xz2fCltGXjHn4h5wjD+I2tIos/teUSw6CLZ6kWp3aK3b7
8tHgTI6Y52Ztt3dvF1PeVPl/3ob7DoVQb4Gmqgv1/0iLxgFekNVrya9HJoW47cpN
rdLLqXAFc+Ev3rSt9ZsztzNc8e9F+dOsjVmtYsKGLKyfhhY6XMLvwpzptnhzjKLD
SRMp9uK80LG0nrg5EG7uXQ/KcerrZp4d8TbshOHY67R+9QY1r+q2cZ4PmdPDoTgp
bnB6lZAP2wDvlmXZp5fwDajsQghhtO4KyzuefJofyaMo6lyufpuIc8jas9pNvuyG
7w6DJcWUASM=
-----END CERTIFICATE-----`
	keyData = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAtm3jpOgBlhs1OosiJSlhM3/+LFBRXPwHdvUW8IZZEqFtY/tZ
6Z2bKTP91QIti5f5N2GzbjGkNbwwoQ7fQUDtQQ/iBSqjv7S3efka49NN69biorj2
L4kTJCBp669+tUuAEmYnqnnZKYufOYo8VcfbmNj4xlxXRVOgMQsiw80N+gPDfjJ9
U9szDvSqHpvv9MZBXAYSsrxhZljP6GHwrbqYlit7ZiMJ0PpXYeNbK7D2/gwENz6p
O7LBLpq4J7Bv7ZDaLIJBG7jg7XGk8tjU6bqOIdS4M27N8mL6sA4R3vJHYo0cszXM
37nCut1MtJg6svrdkBfJ2eQDDVsx8riHoEJmpwIDAQABAoIBAGYkRO9SD4FSHo12
1VllP80r/s4k8klTu4I5W+yz7C9oPu1aEE+jNPru51Jac9HS93CwvVwXY0/K3Jdw
0kOg7LYfBHfMFf8CWjBq70lcSCaiHCbr1LtszlDN7UBO9GzhpwWmONNUgeinCjGX
WozU5/k+kpvNm/dvCSQsjfx/VTID8pLhaI6IbCS4m4HM0K0QwYTgjl3uENmnfnsr
8F6rP4qHS11Rqo0gOVlkHTTkvrM76d7POSmqd8bjPSncdp6dxN/X5k0+sxKG5loO
Lx5b6P/7JKrG42IGxugCGc2/VCF5DghWqqKokZw/wCxhxub25VErF3xgMfAg6Bgl
Euqd+PECgYEA61DSCuU5IGCqX9p6xR4v734cRzR0NIc5urX4yiiw1mXNvOv1Cx8R
1BhDT1fAKDma29DAIW3R0MZuba6VFB+uYcZ2cp/r4I35IcRWXld6ya92eoO6IEN8
R4+rlP8+AHZiR6p22A87L7ddpnE7z5n54O6N6THicsKdC1Sd+zC2WHsCgYEAxnb+
f8y9p19Z/GPprAg1d2AuHX45HUKFmKVMetIld9ySORDsWIjIXHyAno2WunrALmUs
znwvX8WuNYkZkbnYWAQPpIg9dlpZoLbJgT3hcq56SE4pbI6cvbYm1sof5jFv1Zi1
5NQn3fNmq5QRHiPx0VVru+g8LdJcwXaYDO7n8MUCgYEAkqOiwLdnig2zHlh/+SZ+
qLfl11mQsMsz5m5Pw2roCDMYqopAAdYyvgEAsQj17hs3rZPApxRQk9GULzWEIS48
9SE/3t5Zl23hunEngVLyaYy2QFKmQkTLxax6ODd248LiK9bGiI21TF7wNTCLHSvO
06TVOmSjwPAV/WGVsVsBxtECgYBjbwz1dNv0doZ8OIbDpV08URjpt+rfqQuMPg1C
X/Vbx0wPgVYYyXcxN0OtrJy/E28kD5bSYU/O+RjeQ7Fm3Kjy+B3qPkQk/wF2zv3I
XfuNXLNxdI+2jwEi35c3+A7hYxV3+8nuOwk6X4+qGUY2RqYKTnTqsWEtR/8nAscN
e8kDTQKBgC6p5UfzUK07geZDBodKZSrb9q0CjcCAPNzA40WRSrS5Y2eY4YfnZTdr
JXxUpG3royiAYXFAmC6Y/GVNCXHTZeLXnKtojygElv66wo6pfMA5xX8q6q/bvsZb
KZN1JLuNetwk/caU/ZUGVTOTT/hZ9SOgGa2fj5eTJD8anpiRvktt
-----END RSA PRIVATE KEY-----`
)

func createCerts(certFile, keyFile string) error {
	deleteCerts(certFile, keyFile)

	err := ioutil.WriteFile(certFile, []byte(certData), 0644)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(keyFile, []byte(keyData), 0644)
	if err != nil {
		return err
	}

	return nil
}

func deleteCerts(certFile, keyFile string) {
	os.Remove(certFile)
	os.Remove(keyFile)
}
