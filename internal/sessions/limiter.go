package sessions

import (
	"sync"
	"time"
)

// The limiter slows guessing (0176). Five failures in a row for an account,
// or from an address, lock it: for a minute the first time, then twice as
// long each time again, to at most fifteen. A success clears it. It is kept
// in memory: a restart forgets it, and the lockouts are short anyway.
const (
	FailuresBeforeLock = 5
	firstLock          = time.Minute
	longestLock        = 15 * time.Minute
	forgetAfter        = time.Hour
)

// Limiter counts failures by key: "account:<id>" or "address:<ip>".
type Limiter struct {
	now func() time.Time
	mu  sync.Mutex
	by  map[string]*count
}

type count struct {
	fails int       // in a row, since the last lock or success
	locks int       // locks so far, which double the next
	until time.Time // locked until
	last  time.Time // the last failure
}

// NewLimiter returns a limiter.
func NewLimiter(now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{now: now, by: map[string]*count{}}
}

// Locked says how much longer the longest lock among keys lasts, or 0.
func (l *Limiter) Locked(keys ...string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	var most time.Duration
	for _, k := range keys {
		if c := l.by[k]; c != nil && now.Before(c.until) && c.until.Sub(now) > most {
			most = c.until.Sub(now)
		}
	}
	return most
}

// Fail counts a failure against each key, and locks one that reaches
// FailuresBeforeLock.
func (l *Limiter) Fail(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.forget(now)
	for _, k := range keys {
		c := l.by[k]
		if c == nil {
			c = &count{}
			l.by[k] = c
		}
		c.fails++
		c.last = now
		if c.fails >= FailuresBeforeLock {
			d := firstLock << c.locks
			if d > longestLock || d <= 0 {
				d = longestLock
			}
			c.until, c.fails = now.Add(d), 0
			if d < longestLock {
				c.locks++
			}
		}
	}
}

// Succeed clears each key.
func (l *Limiter) Succeed(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.by, k)
	}
}

// forget drops keys not failed for an hour and not locked, so the map
// doesn't grow without end.
func (l *Limiter) forget(now time.Time) {
	for k, c := range l.by {
		if now.Sub(c.last) > forgetAfter && !now.Before(c.until) {
			delete(l.by, k)
		}
	}
}
