package monitor

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/live"
	"sismon/internal/monitor/icmp"
	"sismon/internal/settings"
	"sismon/internal/state"
)

type slowPinger struct {
	d        time.Duration
	cur, max atomic.Int64
	mu       sync.Mutex
	perAddr  map[netip.Addr]int
	maxAddr  int
	starts   []time.Time
}

func (p *slowPinger) Mode() string { return "fake" }
func (p *slowPinger) Ping(ctx context.Context, a netip.Addr, n int, _ time.Duration) icmp.Result {
	c := p.cur.Add(1)
	for {
		m := p.max.Load()
		if c <= m || p.max.CompareAndSwap(m, c) {
			break
		}
	}
	p.mu.Lock()
	if p.perAddr == nil {
		p.perAddr = map[netip.Addr]int{}
	}
	p.perAddr[a]++
	if p.perAddr[a] > p.maxAddr {
		p.maxAddr = p.perAddr[a]
	}
	p.starts = append(p.starts, time.Now())
	p.mu.Unlock()
	time.Sleep(p.d)
	p.mu.Lock()
	p.perAddr[a]--
	p.mu.Unlock()
	p.cur.Add(-1)
	return icmp.Result{Sent: n, Received: n}
}

func noLookup(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("no dns") }

func targets(n int, interval time.Duration) []Target {
	var ts []Target
	for i := 0; i < n; i++ {
		ts = append(ts, Target{DeviceID: int64(i + 1), Rev: 1, Host: netip.AddrFrom4([4]byte{10, 1, byte(i / 250), byte(i%250 + 1)}).String(), Interval: interval, Timeout: time.Second, Packets: 1})
	}
	return ts
}

func runSched(t *testing.T, p icmp.Pinger, workers int, ts []Target, d time.Duration) (*Scheduler, *atomic.Int64) {
	var got atomic.Int64
	s := NewScheduler(p, NewResolver(noLookup, time.Second, func() bool { return true }), func(Result) { got.Add(1) }, workers)
	s.SetTargets(ts)
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	s.Run(ctx)
	return s, &got
}

func TestSchedulerNoOverlap(t *testing.T) {
	p := &slowPinger{d: 120 * time.Millisecond}
	s, got := runSched(t, p, 4, targets(1, 50*time.Millisecond), 700*time.Millisecond)
	if p.maxAddr != 1 {
		t.Fatalf("concurrent checks for one device: %d", p.maxAddr)
	}
	if got.Load() < 3 {
		t.Fatalf("only %d results", got.Load())
	}
	_ = s
}

func TestSchedulerLateChecksCounted(t *testing.T) {
	p := &slowPinger{d: 400 * time.Millisecond}
	s, _ := runSched(t, p, 1, targets(5, 100*time.Millisecond), 4*time.Second)
	if s.Stats().LateChecks == 0 {
		t.Fatal("expected late checks with 1 worker and 5 slow targets")
	}
}

func TestSchedulerStaggers(t *testing.T) {
	p := &slowPinger{d: time.Millisecond}
	runSched(t, p, 64, targets(10, time.Second), 1100*time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.starts) < 10 {
		t.Fatalf("starts %d", len(p.starts))
	}
	first := p.starts[:10]
	if spread := first[9].Sub(first[0]); spread < 500*time.Millisecond {
		t.Fatalf("spread %v", spread)
	}
}

func TestSchedulerConcurrencyCap(t *testing.T) {
	p := &slowPinger{d: 100 * time.Millisecond}
	runSched(t, p, 64, targets(100, 10*time.Millisecond), 600*time.Millisecond)
	if m := p.max.Load(); m > 64 || m < 10 {
		t.Fatalf("max concurrency %d", m)
	}
}

func TestSchedulerSetTargetsRemoves(t *testing.T) {
	p := &slowPinger{d: time.Millisecond}
	var got atomic.Int64
	s := NewScheduler(p, NewResolver(noLookup, time.Second, func() bool { return true }), func(Result) { got.Add(1) }, 4)
	s.SetTargets(targets(1, 20*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	s.SetTargets(nil)
	time.Sleep(50 * time.Millisecond)
	before := got.Load()
	time.Sleep(200 * time.Millisecond)
	if got.Load() != before || before == 0 {
		t.Fatalf("before %d after %d", before, got.Load())
	}
}

func TestSchedulerDNSFailure(t *testing.T) {
	var res Result
	var mu sync.Mutex
	s := NewScheduler(&slowPinger{}, NewResolver(noLookup, time.Second, func() bool { return true }), func(r Result) { mu.Lock(); res = r; mu.Unlock() }, 1)
	s.SetTargets([]Target{{DeviceID: 1, Rev: 1, Host: "nx.local", Interval: time.Second, Timeout: time.Second, Packets: 2}})
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	s.Run(ctx)
	mu.Lock()
	defer mu.Unlock()
	if res.Ping.Code != icmp.DNS || res.Ping.Sent != 2 || res.Ping.OK() {
		t.Fatalf("%+v", res)
	}
}

func TestServiceEndToEnd(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	p := &icmp.Fake{Fn: func(netip.Addr) icmp.Result { l := 1.0; return icmp.Result{Sent: 1, Received: 1, LatencyMs: &l} }}
	svc := NewService(f.st, f.set, f.hub, p, noLookup, ServiceOptions{IntervalScale: 1.0 / 600, FlushEvery: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for f.hub.Snapshot().Locations[0].State != state.Up {
		if time.Now().After(deadline) {
			t.Fatalf("not up: %+v", f.hub.Snapshot().Locations[0])
		}
		time.Sleep(20 * time.Millisecond)
	}
	if svc.Health().ICMPMode != "fake" {
		t.Fatal(svc.Health())
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTestHostRejectsInvalid(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	svc := NewService(f.st, f.set, live.NewHub(), &icmp.Fake{Fn: func(netip.Addr) icmp.Result { return icmp.Result{Sent: 1, Received: 1} }}, noLookup)
	_, err := svc.TestHost(context.Background(), "a b")
	var e *apperr.Error
	if !errors.As(err, &e) || e.Status != 400 {
		t.Fatal(err)
	}
	r, err := svc.TestHost(context.Background(), "10.0.0.1")
	if err != nil || !r.OK() {
		t.Fatal(r, err)
	}
}
