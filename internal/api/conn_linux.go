//go:build linux

package api

import (
	"syscall"
	"time"
)

// tcpUserTimeout is TCP_USER_TIMEOUT as linux/tcp.h has it, the same on
// every architecture; Go's syscall names it for some of them only.
const tcpUserTimeout = 18

// userTimeout sets TCP_USER_TIMEOUT on c: how long what was written may go
// unacknowledged before the kernel gives the connection up.
func userTimeout(c syscall.Conn, d time.Duration) {
	raw, err := c.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout, int(d/time.Millisecond))
	})
}
