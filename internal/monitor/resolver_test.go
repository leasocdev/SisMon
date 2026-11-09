package monitor

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestResolveLiteral(t *testing.T) {
	r := NewResolver(func(context.Context, string) ([]netip.Addr, error) {
		t.Fatal("lookup called for literal")
		return nil, nil
	}, time.Second, func() bool { return true })
	a, warn, err := r.Resolve(context.Background(), "10.0.0.1")
	if err != nil || warn || a != netip.MustParseAddr("10.0.0.1") {
		t.Fatal(a, warn, err)
	}
}

func TestResolveCachesAndWarns(t *testing.T) {
	fail := false
	r := NewResolver(func(context.Context, string) ([]netip.Addr, error) {
		if fail {
			return nil, errors.New("dns down")
		}
		return []netip.Addr{netip.MustParseAddr("10.0.0.7")}, nil
	}, time.Second, func() bool { return true })
	if a, warn, err := r.Resolve(context.Background(), "r.local"); err != nil || warn || a.String() != "10.0.0.7" {
		t.Fatal(a, warn, err)
	}
	fail = true
	if a, warn, err := r.Resolve(context.Background(), "r.local"); err != nil || !warn || a.String() != "10.0.0.7" {
		t.Fatal(a, warn, err)
	}
}

func TestResolveNoCacheFails(t *testing.T) {
	r := NewResolver(func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("nx") }, time.Second, func() bool { return true })
	if _, _, err := r.Resolve(context.Background(), "x.local"); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveTimeout(t *testing.T) {
	r := NewResolver(func(ctx context.Context, _ string) ([]netip.Addr, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, 50*time.Millisecond, func() bool { return true })
	start := time.Now()
	if _, _, err := r.Resolve(context.Background(), "slow.local"); err == nil || time.Since(start) > time.Second {
		t.Fatal(err, time.Since(start))
	}
}

func TestResolvePrefersV4(t *testing.T) {
	addrs := []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}
	look := func(context.Context, string) ([]netip.Addr, error) { return addrs, nil }
	if a, _, _ := NewResolver(look, time.Second, func() bool { return true }).Resolve(context.Background(), "h"); a.String() != "127.0.0.1" {
		t.Fatal(a)
	}
	if a, _, _ := NewResolver(look, time.Second, func() bool { return false }).Resolve(context.Background(), "h"); a.String() != "::1" {
		t.Fatal(a)
	}
}
