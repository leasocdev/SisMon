package auth

import (
	"net/netip"
	"sync"
	"time"
)

type attempts struct {
	fails []time.Time
	last  time.Time
}

// Limiter limits failed login attempts per client IP.
type Limiter struct {
	max      int
	window   time.Duration
	capacity int
	mu       sync.Mutex
	m        map[netip.Addr]*attempts
}

// NewLimiter allows max failures per window; at capacity the oldest entry is evicted.
func NewLimiter(max int, window time.Duration, capacity int) *Limiter {
	return &Limiter{max: max, window: window, capacity: capacity, m: map[netip.Addr]*attempts{}}
}

func (l *Limiter) prune(a *attempts, now time.Time) {
	cut := now.Add(-l.window)
	i := 0
	for i < len(a.fails) && !a.fails[i].After(cut) {
		i++
	}
	a.fails = a.fails[i:]
}

// Check reports whether ip may attempt a login, and otherwise when.
func (l *Limiter) Check(ip netip.Addr, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.m[ip]
	if !ok {
		return 0, true
	}
	l.prune(a, now)
	if len(a.fails) < l.max {
		return 0, true
	}
	return a.fails[0].Add(l.window).Sub(now), false
}

// Fail records a failure and returns the delay to apply to the response.
func (l *Limiter) Fail(ip netip.Addr, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.m[ip]
	if !ok {
		if len(l.m) >= l.capacity {
			var oldest netip.Addr
			var t time.Time
			for k, v := range l.m {
				if t.IsZero() || v.last.Before(t) {
					oldest, t = k, v.last
				}
			}
			delete(l.m, oldest)
		}
		a = &attempts{}
		l.m[ip] = a
	}
	l.prune(a, now)
	a.fails = append(a.fails, now)
	a.last = now
	d := 250 * time.Millisecond << (len(a.fails) - 1)
	if d > 4*time.Second || d <= 0 {
		d = 4 * time.Second
	}
	return d
}

// Reset clears the failures of ip.
func (l *Limiter) Reset(ip netip.Addr) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, ip)
}

// Len returns the number of tracked clients.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.m)
}

func (l *Limiter) has(ip netip.Addr) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.m[ip]
	return ok
}
