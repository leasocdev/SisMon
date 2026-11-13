package api

import (
	"net/netip"
	"sync"
)

// SSE connection caps: the dispatcher screens and operators need a handful
// each; an anonymous client must not be able to hold thousands of streams.
const (
	SSEMaxTotal = 500
	SSEMaxPerIP = 50
)

type sseCap struct {
	mu       sync.Mutex
	total    int
	perIP    map[netip.Addr]int
	maxTotal int
	maxPerIP int
}

func newSSECap(total, perIP int) *sseCap {
	return &sseCap{perIP: map[netip.Addr]int{}, maxTotal: total, maxPerIP: perIP}
}

// acquire reserves a stream for ip; false when a cap is reached.
func (c *sseCap) acquire(ip netip.Addr) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total >= c.maxTotal || c.perIP[ip] >= c.maxPerIP {
		return false
	}
	c.total++
	c.perIP[ip]++
	return true
}

func (c *sseCap) release(ip netip.Addr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total--
	if c.perIP[ip]--; c.perIP[ip] <= 0 {
		delete(c.perIP, ip)
	}
}
