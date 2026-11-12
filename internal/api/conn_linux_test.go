//go:build linux

package api

import (
	"context"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/WiFiGUY512/aeolus/internal/quickack"
)

// TestDeadAfter: a held line's connection, accepted as the manager accepts
// them (quick-ACK), has the kernel's give-up time set (0165). A first try
// looked for a bare TCP connection, which the quick-ACK wrapper is not, and
// set nothing.
func TestDeadAfter(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	l := quickack.Listener(ln)
	defer l.Close()
	dial, err := net.Dial("tcp", ln.Addr().String())
	must(t, err)
	defer dial.Close()
	c, err := l.Accept()
	must(t, err)
	defer c.Close()
	deadAfter(WithConn(context.Background(), c), 15*time.Second)
	raw, err := c.(syscall.Conn).SyscallConn()
	must(t, err)
	got := -1
	must(t, raw.Control(func(fd uintptr) {
		got, err = syscall.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout)
	}))
	must(t, err)
	if got != 15000 {
		t.Fatalf("TCP_USER_TIMEOUT = %d ms, want 15000", got)
	}
}
