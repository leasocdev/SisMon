package monitor

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"netwatch/internal/live"
	"netwatch/internal/settings"
	"netwatch/internal/state"
	"netwatch/internal/store"
)

type devInfo struct {
	rev        int64
	locationID int64
	primary    bool
	overrides  settings.Overrides
	machine    *state.Device
	status     store.Status
}

// Writer is the single goroutine that applies check results to the database.
type Writer struct {
	FlushEvery time.Duration
	GapEvery   time.Duration

	st  *store.Store
	hub *live.Hub
	set *settings.Holder
	ch  chan Result
	now func() time.Time

	mu        sync.Mutex // guards devices, applies are serialized by it
	devices   map[int64]*devInfo
	lastFlush time.Time
	dropped   int
	lastErr   string
}

// NewWriter builds a writer.
func NewWriter(st *store.Store, hub *live.Hub, set *settings.Holder) *Writer {
	return &Writer{FlushEvery: time.Second, GapEvery: 30 * time.Second, st: st, hub: hub, set: set,
		ch: make(chan Result, 1024), now: time.Now, devices: map[int64]*devInfo{}}
}

// Submit queues a result without blocking; it is dropped if the queue is full.
func (w *Writer) Submit(r Result) {
	select {
	case w.ch <- r:
	default:
		w.mu.Lock()
		w.dropped++
		w.mu.Unlock()
	}
}

// Stats reports the last successful flush and the last error.
func (w *Writer) Stats() (lastFlush time.Time, dropped int, lastErr string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastFlush, w.dropped, w.lastErr
}

// Reload refreshes device configuration and persisted status.
func (w *Writer) Reload(ctx context.Context) error {
	devs, err := store.ListDevices(ctx, w.st.DB, false)
	if err != nil {
		return err
	}
	stats, err := store.DeviceStatuses(ctx, w.st.DB)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	next := map[int64]*devInfo{}
	for _, d := range devs {
		s := stats[d.ID]
		s.DeviceID = d.ID
		if s.State == "" {
			s.State = state.Unknown
		}
		var ss time.Time
		if s.StreakStart != nil {
			ss = time.UnixMilli(*s.StreakStart)
		}
		info := &devInfo{rev: d.Rev, locationID: d.LocationID, primary: d.Primary, overrides: d.Overrides, status: s}
		if old, ok := w.devices[d.ID]; ok && old.rev == d.Rev {
			info.machine = old.machine
			info.status = old.status
		} else {
			info.machine = state.Restore(s.State, s.Pending, s.Streak, ss)
		}
		next[d.ID] = info
	}
	w.devices = next
	return nil
}

// Run applies queued results every FlushEvery and expires gaps every GapEvery.
func (w *Writer) Run(ctx context.Context) error {
	if err := w.Reload(ctx); err != nil {
		return err
	}
	if err := w.ExpireGaps(ctx, w.now()); err != nil {
		slog.Error("expirare pauze", "err", err)
	}
	flush := time.NewTicker(w.FlushEvery)
	gap := time.NewTicker(w.GapEvery)
	defer flush.Stop()
	defer gap.Stop()
	for {
		select {
		case <-ctx.Done():
			return w.Flush(context.WithoutCancel(ctx))
		case <-flush.C:
			if err := w.Flush(ctx); err != nil {
				slog.Error("scriere rezultate", "err", err)
			}
		case <-gap.C:
			if err := w.ExpireGaps(ctx, w.now()); err != nil {
				slog.Error("expirare pauze", "err", err)
			}
		}
	}
}

type touched struct {
	devices   map[int64]*devInfo
	locations map[int64]bool
}

func newTouched() *touched {
	return &touched{devices: map[int64]*devInfo{}, locations: map[int64]bool{}}
}

// Flush applies everything currently queued in one transaction.
func (w *Writer) Flush(ctx context.Context) error {
	var batch []Result
	for {
		select {
		case r := <-w.ch:
			batch = append(batch, r)
			continue
		default:
		}
		break
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(batch) == 0 {
		w.lastFlush = w.now()
		return nil
	}
	tc := newTouched()
	err := w.inTx(ctx, func(tx *sql.Tx) error {
		for _, r := range batch {
			if err := w.apply(ctx, tx, r, tc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		w.lastErr = err.Error()
		// Reload machines from the database: the rolled-back batch must not linger in memory.
		w.mu.Unlock()
		rerr := w.Reload(ctx)
		w.mu.Lock()
		if rerr != nil {
			slog.Error("reîncărcare după eroare", "err", rerr)
		}
		return err
	}
	w.lastFlush = w.now()
	w.publish(ctx, tc)
	return nil
}

func (w *Writer) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := w.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (w *Writer) effective(info *devInfo) settings.Effective {
	s := w.set.Get()
	if e, err := s.Apply(info.overrides); err == nil {
		return e
	}
	e, _ := s.Apply(settings.Overrides{})
	return e
}

func (w *Writer) apply(ctx context.Context, tx *sql.Tx, r Result, tc *touched) error {
	info, ok := w.devices[r.DeviceID]
	if !ok || info.rev != r.Rev {
		return nil // archived or reconfigured while the check was in flight
	}
	eff := w.effective(info)
	if lc := info.status.LastCheck; lc != nil && time.UnixMilli(*lc).Add(eff.Stale).Before(r.At) {
		if err := w.expire(ctx, tx, r.DeviceID, info, time.UnixMilli(*lc).Add(eff.Stale), tc); err != nil {
			return err
		}
	}
	p := r.Ping
	if _, err := tx.ExecContext(ctx, `INSERT INTO samples(device_id,ts,ok,latency_ms,loss_pct,error_code) VALUES(?,?,?,?,?,?)`,
		r.DeviceID, store.Ms(r.At), p.OK(), p.LatencyMs, p.LossPct(), string(p.Code)); err != nil {
		return err
	}
	tr := info.machine.Apply(state.Check{At: r.At, OK: p.OK(), MonitorError: p.MonitorError()},
		state.Thresholds{Failures: eff.Failures, Recoveries: eff.Recoveries})
	s := &info.status
	if tr != nil {
		if err := store.CloseAndOpen(ctx, tx, store.EntityDevice, r.DeviceID, tr.To, tr.At); err != nil {
			return err
		}
		if err := w.incident(ctx, tx, r.DeviceID, info, tr); err != nil {
			return err
		}
		s.Since = store.Ms(tr.At)
	}
	at := store.Ms(r.At)
	loss := p.LossPct()
	s.State, s.Pending, s.Streak = info.machine.State, info.machine.Pending, info.machine.Streak
	s.StreakStart = nil
	if !info.machine.StreakStart.IsZero() {
		v := store.Ms(info.machine.StreakStart)
		s.StreakStart = &v
	}
	s.LastCheck, s.LatencyMs, s.LossPct, s.ErrorCode, s.DNSWarning = &at, p.LatencyMs, &loss, string(p.Code), r.DNSWarning
	if err := store.PutStatus(ctx, tx, *s); err != nil {
		return err
	}
	if tr != nil {
		if _, err := store.RecomputeLocation(ctx, tx, info.locationID, tr.At); err != nil {
			return err
		}
		tc.locations[info.locationID] = true
	}
	tc.devices[r.DeviceID] = info
	return nil
}

func (w *Writer) incident(ctx context.Context, tx *sql.Tx, id int64, info *devInfo, tr *state.Transition) error {
	if tr.To == state.Down {
		if w.set.Get().IncidentsFor == "primary" && !info.primary {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO incidents(device_id,observed_at,confirmed_at)
			SELECT ?,?,? WHERE NOT EXISTS (SELECT 1 FROM incidents WHERE device_id=? AND ended_at IS NULL)`,
			id, store.Ms(tr.At), store.Ms(tr.ConfirmedAt), id)
		return err
	}
	reason := "recovered"
	if tr.To == state.Error {
		reason = "monitor_error"
	}
	_, err := tx.ExecContext(ctx, `UPDATE incidents SET ended_at=MAX(observed_at,?),end_reason=? WHERE device_id=? AND ended_at IS NULL`,
		store.Ms(tr.At), reason, id)
	return err
}

func (w *Writer) expire(ctx context.Context, tx *sql.Tx, id int64, info *devInfo, at time.Time, tc *touched) error {
	wasUnknown := info.machine.State == state.Unknown
	info.machine.Expire()
	s := &info.status
	s.State, s.Pending, s.Streak, s.StreakStart = state.Unknown, "", 0, nil
	if !wasUnknown {
		s.Since = store.Ms(at)
		if err := store.CloseAndOpen(ctx, tx, store.EntityDevice, id, state.Unknown, at); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE incidents SET ended_at=MAX(observed_at,?),end_reason='monitoring_gap' WHERE device_id=? AND ended_at IS NULL`, store.Ms(at), id); err != nil {
			return err
		}
	}
	if err := store.PutStatus(ctx, tx, *s); err != nil {
		return err
	}
	if !wasUnknown {
		if _, err := store.RecomputeLocation(ctx, tx, info.locationID, at); err != nil {
			return err
		}
		tc.locations[info.locationID] = true
	}
	tc.devices[id] = info
	return nil
}

// ExpireGaps marks devices whose last check is older than the stale limit as
// unknown, closing open incidents with monitoring_gap.
func (w *Writer) ExpireGaps(ctx context.Context, now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	tc := newTouched()
	err := w.inTx(ctx, func(tx *sql.Tx) error {
		for id, info := range w.devices {
			lc := info.status.LastCheck
			if lc == nil {
				continue
			}
			g := time.UnixMilli(*lc).Add(w.effective(info).Stale)
			if g.After(now) || (info.machine.State == state.Unknown && info.machine.Streak == 0) {
				continue
			}
			if err := w.expire(ctx, tx, id, info, g, tc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	w.publish(ctx, tc)
	return nil
}

func (w *Writer) publish(ctx context.Context, tc *touched) {
	for id, info := range tc.devices {
		s := info.status
		w.hub.SetDevice(live.DeviceView{ID: id, LocationID: info.locationID, State: s.State, Pending: s.Pending, Since: s.Since,
			LastCheck: s.LastCheck, LatencyMs: s.LatencyMs, LossPct: s.LossPct, ErrorCode: s.ErrorCode, DNSWarning: s.DNSWarning})
	}
	for lid := range tc.locations {
		st, start, ok, err := store.OpenState(ctx, w.st.DB, store.EntityLocation, lid)
		if err != nil || !ok {
			continue
		}
		ms, err := store.LocationMembers(ctx, w.st.DB, lid)
		if err != nil {
			continue
		}
		r := state.Location(ms)
		r.State = st
		w.hub.SetLocationState(lid, r, store.Ms(start))
	}
}
