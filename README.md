gtcp
====

[![Go Report Card](https://goreportcard.com/badge/cat2neat/gtcp)](https://goreportcard.com/report/cat2neat/gtcp) [![Build Status](https://travis-ci.org/cat2neat/gtcp.svg?branch=master)](https://travis-ci.org/cat2neat/gtcp) [![Coverage Status](https://coveralls.io/repos/github/cat2neat/gtcp/badge.svg?branch=master)](https://coveralls.io/github/cat2neat/gtcp?branch=master) [![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/cat2neat/gtcp)

Package gtcp is a TCP server framework that inherits battle-tested code from net/http
and can be extended through built-in interfaces.

### Features

- Can be used in the same manner with http.Server(>= 1.8).
  - Make API as much compatible as possible.
  - Make the zero value useful.

- Inherits as much battle tested code from net/http.

- Provides much flexiblity through built-in interfaces.
  - ConnHandler
    - ConnHandler
    - KeepAliveHandler that makes it easy to implement keepalive.
    - PipelineHandler that makes it easy to implement pipelining.
  - ConnTracker
    - MapConnTracker that handles force closing active connections also graceful shutdown.
    - WGConnTracker that handles only graceful shutdown using a naive way with sync.WaitGroup.
  - Conn
    - BufferedConn that wraps Conn in bufio.Reader/Writer.
    - StatsConn that wraps Conn to measure incomming/outgoing bytes.
    - DebugConn that wraps Conn to output debug information.
  - Logger
    - BuiltinLogger that logs using standard log package.
  - Retry
    - ExponentialRetry that implements exponential backoff algorithm without jitter.
  - Statistics
    - TrafficStatistics that measures incomming/outgoing traffic across a server.
  - Limiter
    - MaxConnLimiter that limits connections based on the maximum number.

- Gets GC pressure as little as possible with sync.Pool.

- Zero 3rd party depentencies.

### TODO

- Support TLS

- Support multiple listeners

Example
-------

```go
import "github.com/cat2neat/gtcp"

// echo server:
// https://tools.ietf.org/html/rfc862
func echoServer() error {
	srv := &gtcp.Server{
		Addr:    ":1979",
		NewConn: gtcp.NewStatsConn,
		ConnHandler: func(ctx context.Context, conn gtcp.Conn) {
			buf := make([]byte, 1024)
			for {
				n, err := conn.Read(buf)
				if err != nil {
					return
				}
				conn.Write(buf[:n])
				err = conn.Flush()
				if err != nil {
					return
				}
				select {
				case <-ctx.Done():
					// canceled by parent
					return
				default:
				}
			}
		},
	}
	return srv.ListenAndServe()
}
```

Install
-------

```shell
go get -u github.com/cat2neat/gtcp
```
