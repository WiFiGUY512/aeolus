package api

import (
	"context"
	"crypto/tls"
	"net"
	"syscall"
	"time"
)

// connKey is where the server puts a request's connection in its context.
type connKey struct{}

// WithConn is the manager's http.Server's ConnContext: it puts each
// connection in the context of its requests, where a held answer finds it
// (0165).
func WithConn(ctx context.Context, c net.Conn) context.Context {
	return context.WithValue(ctx, connKey{}, c)
}

// deadAfter has the kernel give a request's connection up once what was
// written on it goes unacknowledged for d (0165), where the connection
// lets its socket be reached, under TLS and the manager's quick-ACK
// wrapper alike, and the system can: TCP_USER_TIMEOUT, on Linux. A peer
// that is gone without closing is otherwise taken to be there until TCP's
// own retries run out, a quarter of an hour.
func deadAfter(ctx context.Context, d time.Duration) {
	c, _ := ctx.Value(connKey{}).(net.Conn)
	if tc, ok := c.(*tls.Conn); ok {
		c = tc.NetConn()
	}
	if sc, ok := c.(syscall.Conn); ok {
		userTimeout(sc, d)
	}
}
