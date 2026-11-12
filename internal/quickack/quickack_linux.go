package quickack

import (
	"net"
	"syscall"
)

// Listener is l with every TCP connection it accepts acknowledging at
// once.
func Listener(l net.Listener) net.Listener { return listener{l} }

type listener struct{ net.Listener }

func (l listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tc, ok := c.(*net.TCPConn)
	if !ok {
		return c, nil
	}
	raw, err := tc.SyscallConn()
	if err != nil {
		return c, nil
	}
	q := &conn{TCPConn: tc, raw: raw}
	q.quick()
	return q, nil
}

// conn is a TCP connection that asks, after each read and each write, that
// what comes next be acknowledged at once.
type conn struct {
	*net.TCPConn
	raw syscall.RawConn
}

func (c *conn) quick() {
	// One that fails changes nothing but the 40 ms: it is not an error of
	// the connection's.
	_ = c.raw.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_QUICKACK, 1)
	})
}

func (c *conn) Read(b []byte) (int, error) {
	n, err := c.TCPConn.Read(b)
	c.quick()
	return n, err
}

func (c *conn) Write(b []byte) (int, error) {
	n, err := c.TCPConn.Write(b)
	c.quick()
	return n, err
}
