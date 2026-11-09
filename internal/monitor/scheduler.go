package monitor

import (
	"context"
	"sync"
	"time"

	"netwatch/internal/monitor/icmp"
)

// Target is one device configuration to check periodically.
type Target struct {
	DeviceID int64
	Rev      int64
	Host     string
	Interval time.Duration
	Timeout  time.Duration
	Packets  int
}

// SchedStats summarizes scheduler behavior.
type SchedStats struct {
	LastCycle  time.Time
	LateChecks int
	AvgCheck   time.Duration
}

type entry struct {
	t       Target
	next    time.Time
	running bool
}

const lateWindow = 10 * time.Minute

// Scheduler runs checks per device on its own interval, never overlapping
// checks of the same device, with at most `workers` concurrent checks.
type Scheduler struct {
	p       icmp.Pinger
	r       *Resolver
	submit  func(Result)
	sem     chan struct{}
	wake    chan struct{}
	mu      sync.Mutex
	entries map[int64]*entry
	late    []time.Time
	last    time.Time
	avg     time.Duration
}

// NewScheduler builds a scheduler.
func NewScheduler(p icmp.Pinger, r *Resolver, submit func(Result), workers int) *Scheduler {
	return &Scheduler{p: p, r: r, submit: submit, sem: make(chan struct{}, workers), wake: make(chan struct{}, 1), entries: map[int64]*entry{}}
}

// SetTargets replaces the target set. Unchanged targets (same device and
// revision) keep their schedule; new ones are staggered across their interval.
func (s *Scheduler) SetTargets(ts []Target) {
	s.mu.Lock()
	now := time.Now()
	next := map[int64]*entry{}
	var fresh []*entry
	for _, t := range ts {
		if e, ok := s.entries[t.DeviceID]; ok && e.t.Rev == t.Rev {
			e.t = t
			next[t.DeviceID] = e
			continue
		}
		e := &entry{t: t}
		next[t.DeviceID] = e
		fresh = append(fresh, e)
	}
	for i, e := range fresh {
		e.next = now.Add(time.Duration(i) * e.t.Interval / time.Duration(len(fresh)))
	}
	s.entries = next
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Stats returns current statistics.
func (s *Scheduler) Stats() SchedStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := time.Now().Add(-lateWindow)
	n := 0
	for _, t := range s.late {
		if t.After(cut) {
			n++
		}
	}
	return SchedStats{LastCycle: s.last, LateChecks: n, AvgCheck: s.avg}
}

// Run schedules checks until ctx is done, then waits for running checks.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		s.mu.Lock()
		now := time.Now()
		wait := time.Second
		for _, e := range s.entries {
			if e.running {
				continue
			}
			if !e.next.After(now) {
				e.running = true
				wg.Add(1)
				go func(e *entry, due time.Time) {
					defer wg.Done()
					s.check(ctx, e, due)
				}(e, e.next)
				continue
			}
			if d := e.next.Sub(now); d < wait {
				wait = d
			}
		}
		s.mu.Unlock()
		timer.Reset(wait)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-s.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}
}

func (s *Scheduler) check(ctx context.Context, e *entry, due time.Time) {
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		s.mu.Lock()
		e.running = false
		s.mu.Unlock()
		return
	}
	start := time.Now()
	t := e.t
	res := Result{DeviceID: t.DeviceID, Rev: t.Rev}
	addr, warn, err := s.r.Resolve(ctx, t.Host)
	if err != nil {
		res.Ping = icmp.Result{Sent: t.Packets, Code: icmp.DNS}
	} else {
		res.Ping = s.p.Ping(ctx, addr, t.Packets, t.Timeout)
		res.DNSWarning = warn
	}
	<-s.sem
	end := time.Now()
	res.At = start
	if ctx.Err() == nil {
		s.submit(res)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if start.Sub(due) > time.Second {
		s.late = append(s.late, start)
		cut := end.Add(-lateWindow)
		for len(s.late) > 0 && s.late[0].Before(cut) {
			s.late = s.late[1:]
		}
	}
	dur := end.Sub(start)
	if s.avg == 0 {
		s.avg = dur
	} else {
		s.avg = (s.avg*9 + dur) / 10
	}
	s.last = end
	e.running = false
	n := due.Add(t.Interval)
	if n.Before(end) {
		n = end
	}
	e.next = n
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
