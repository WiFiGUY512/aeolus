//go:build !linux

package api

import (
	"syscall"
	"time"
)

// userTimeout does nothing where the system has no TCP_USER_TIMEOUT: a
// peer gone without closing is found when TCP's own retries run out.
func userTimeout(syscall.Conn, time.Duration) {}
