// Package monitor schedules checks, resolves names and writes results.
package monitor

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"
)

// LookupFunc resolves a host name to addresses.
type LookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// Resolver resolves names with a timeout and remembers the last good address.
type Resolver struct {
	lookup   LookupFunc
	timeout  time.Duration
	preferV4 func() bool
	mu       sync.Mutex
	last     map[string]netip.Addr
}

// NewResolver builds a resolver.
func NewResolver(lookup LookupFunc, timeout time.Duration, preferV4 func() bool) *Resolver {
	return &Resolver{lookup: lookup, timeout: timeout, preferV4: preferV4, last: map[string]netip.Addr{}}
}

// Resolve returns an address for host. dnsWarning is true when the lookup
// failed and the last known address is used instead.
func (r *Resolver) Resolve(ctx context.Context, host string) (netip.Addr, bool, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return a, false, nil
	}
	lctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	type res struct {
		addrs []netip.Addr
		err   error
	}
	ch := make(chan res, 1)
	go func() {
		a, err := r.lookup(lctx, host)
		ch <- res{a, err}
	}()
	var out res
	select {
	case out = <-ch:
	case <-lctx.Done():
		out.err = lctx.Err()
	}
	if out.err == nil && len(out.addrs) == 0 {
		out.err = errors.New("nicio adresă")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if out.err != nil {
		if a, ok := r.last[host]; ok {
			return a, true, nil
		}
		return netip.Addr{}, false, out.err
	}
	pick := out.addrs[0]
	if r.preferV4() {
		for _, a := range out.addrs {
			if a.Unmap().Is4() {
				pick = a.Unmap()
				break
			}
		}
	}
	r.last[host] = pick
	return pick, false, nil
}
