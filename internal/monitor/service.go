package monitor

import (
	"context"
	"os"
	"sync"
	"time"

	"netwatch/internal/apperr"
	"netwatch/internal/domain"
	"netwatch/internal/live"
	"netwatch/internal/monitor/icmp"
	"netwatch/internal/settings"
	"netwatch/internal/store"
)

// Workers is the maximum number of concurrent checks.
const Workers = 64

// ServiceOptions tune the service (tests use them to speed time up).
type ServiceOptions struct {
	IntervalScale float64       // multiplies check intervals; 0 means 1
	FlushEvery    time.Duration // writer flush period; 0 means 1s
}

// Service ties the scheduler, the writer and the live hub together.
type Service struct {
	st    *store.Store
	set   *settings.Holder
	hub   *live.Hub
	p     icmp.Pinger
	r     *Resolver
	w     *Writer
	s     *Scheduler
	opt   ServiceOptions
	mu    sync.Mutex
	bLast *int64
	bErr  string
}

// NewService builds the monitoring service.
func NewService(st *store.Store, set *settings.Holder, hub *live.Hub, p icmp.Pinger, lookup LookupFunc, opts ...ServiceOptions) *Service {
	m := &Service{st: st, set: set, hub: hub, p: p}
	if len(opts) > 0 {
		m.opt = opts[0]
	}
	if m.opt.IntervalScale == 0 {
		m.opt.IntervalScale = 1
	}
	m.r = NewResolver(lookup, settings.DNSTimeout, func() bool { return set.Get().PreferIPv4 })
	m.w = NewWriter(st, hub, set)
	if m.opt.FlushEvery > 0 {
		m.w.FlushEvery = m.opt.FlushEvery
	}
	m.s = NewScheduler(p, m.r, m.w.Submit, Workers)
	return m
}

func (m *Service) targets(ctx context.Context) ([]Target, error) {
	devs, err := store.ListDevices(ctx, m.st.DB, false)
	if err != nil {
		return nil, err
	}
	s := m.set.Get()
	ts := make([]Target, 0, len(devs))
	for _, d := range devs {
		e, err := s.Apply(d.Overrides)
		if err != nil {
			e, _ = s.Apply(settings.Overrides{})
		}
		ts = append(ts, Target{DeviceID: d.ID, Rev: d.Rev, Host: d.Host, Packets: e.Packets, Timeout: e.Timeout,
			Interval: time.Duration(float64(e.Interval) * m.opt.IntervalScale)})
	}
	return ts, nil
}

// Reload applies configuration changes: writer state, targets, live view.
func (m *Service) Reload(ctx context.Context) error {
	if err := m.w.Reload(ctx); err != nil {
		return err
	}
	ts, err := m.targets(ctx)
	if err != nil {
		return err
	}
	m.s.SetTargets(ts)
	return m.hub.Load(ctx, m.st)
}

// Run starts the writer, the scheduler and periodic health reporting.
func (m *Service) Run(ctx context.Context) error {
	if err := m.Reload(ctx); err != nil {
		return err
	}
	var wg sync.WaitGroup
	errc := make(chan error, 1)
	wg.Add(2)
	go func() { defer wg.Done(); errc <- m.w.Run(ctx) }()
	go func() { defer wg.Done(); m.s.Run(ctx) }()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	m.hub.SetHealth(m.Health())
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return <-errc
		case <-tick.C:
			m.hub.SetHealth(m.Health())
		}
	}
}

// SetBackupStatus records the outcome of the last backup for Health.
func (m *Service) SetBackupStatus(last *int64, errMsg string) {
	m.mu.Lock()
	m.bLast, m.bErr = last, errMsg
	m.mu.Unlock()
	m.hub.SetHealth(m.Health())
}

// Health reports the monitor's own condition.
func (m *Service) Health() live.Health {
	ss := m.s.Stats()
	lastFlush, dropped, lastErr := m.w.Stats()
	h := live.Health{LateChecks: ss.LateChecks, AvgCheckMs: float64(ss.AvgCheck.Microseconds()) / 1000, ICMPMode: m.p.Mode(), Errors: []string{}}
	if !ss.LastCycle.IsZero() {
		v := ss.LastCycle.UnixMilli()
		h.LastCycle = &v
	}
	if lastErr != "" {
		h.Errors = append(h.Errors, lastErr)
	}
	if dropped > 0 {
		h.Errors = append(h.Errors, "Rezultate pierdute din cauza cozii pline.")
	}
	if fi, err := os.Stat(m.st.Path); err == nil {
		h.DBSizeBytes = fi.Size()
	}
	m.mu.Lock()
	h.LastBackup, h.BackupError = m.bLast, m.bErr
	m.mu.Unlock()
	interval := time.Duration(float64(time.Duration(m.set.Get().IntervalS)*time.Second) * m.opt.IntervalScale)
	switch {
	case m.p.Mode() == "unavailable":
		h.Warning = "Monitorul nu are permisiunea de a trimite pachete ICMP."
	case ss.LateChecks > 0:
		h.Warning = "Monitorizarea rămâne în urmă."
	case !lastFlush.IsZero() && time.Since(lastFlush) > 2*interval:
		h.Warning = "Monitorul nu a scris date recent."
	}
	return h
}

// TestHost checks a host immediately with the current settings, without saving.
func (m *Service) TestHost(ctx context.Context, host string) (icmp.Result, error) {
	if !domain.ValidHost(host) {
		return icmp.Result{}, apperr.Invalid("host", "Adresă IP sau nume DNS invalid.")
	}
	e, _ := m.set.Get().Apply(settings.Overrides{})
	addr, _, err := m.r.Resolve(ctx, host)
	if err != nil {
		return icmp.Result{Sent: e.Packets, Code: icmp.DNS}, nil
	}
	return m.p.Ping(ctx, addr, e.Packets, e.Timeout), nil
}
