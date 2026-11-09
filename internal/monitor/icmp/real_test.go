package icmp

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

func realPinger(t *testing.T) Pinger {
	t.Helper()
	if testing.Short() {
		t.Skip("real ICMP skipped in -short")
	}
	p, err := New()
	if err != nil {
		t.Skipf("no pinger: %v", err)
	}
	if p.Mode() == "unavailable" {
		t.Skip("ICMP unavailable (permissions)")
	}
	return p
}

func TestRealLoopback(t *testing.T) {
	p := realPinger(t)
	r := p.Ping(context.Background(), netip.MustParseAddr("127.0.0.1"), 2, time.Second)
	if r.Sent != 2 || r.Received < 1 || r.LatencyMs == nil || r.Code != OK {
		t.Fatalf("%+v (mode %s)", r, p.Mode())
	}
}

func TestRealLoopbackV6(t *testing.T) {
	p := realPinger(t)
	r := p.Ping(context.Background(), netip.MustParseAddr("::1"), 1, time.Second)
	if r.Received < 1 {
		t.Skipf("IPv6 loopback not answering here: %+v", r)
	}
}

func TestRealTestNet(t *testing.T) {
	p := realPinger(t)
	start := time.Now()
	r := p.Ping(context.Background(), netip.MustParseAddr("192.0.2.1"), 1, time.Second)
	if r.OK() || (r.Code != Timeout && r.Code != HostUnreachable && r.Code != NetUnreachable) {
		t.Fatalf("%+v", r)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

func TestFakeAndContextCancel(t *testing.T) {
	f := &Fake{Fn: func(netip.Addr) Result { return Result{Sent: 1, Received: 1} }}
	if !f.Ping(context.Background(), netip.MustParseAddr("10.0.0.1"), 1, time.Second).OK() || f.Mode() != "fake" {
		t.Fatal("fake")
	}
}
