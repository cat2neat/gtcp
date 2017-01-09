/*
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
*/
package gtcp
