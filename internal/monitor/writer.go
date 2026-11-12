package monitor

import (
	"context"
	"database/sql"
	"log/slog"
	"math"
	"sync"
	"time"

	"sismon/internal/live"
	"sismon/internal/settings"
	"sismon/internal/state"
	"sismon/internal/store"
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
		if d.Disabled {
			continue
		}
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
		revs, err := activeRevs(ctx, tx)
		if err != nil {
			return err
		}
		for _, r := range batch {
			if err := w.apply(ctx, tx, r, tc, revs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// The batch rolled back: its working copies are discarded, memory
		// still matches the database.
		w.lastErr = err.Error()
		return err
	}
	w.commit(tc)
	w.lastFlush = w.now()
	w.publish(ctx, tc)
	return nil
}

// activeRevs reads the current revision of every active device inside the
// batch transaction, so results of a configuration that changed (or was
// archived) after the check started are dropped even before Reload runs.
func activeRevs(ctx context.Context, tx *sql.Tx) (map[int64]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,rev FROM devices WHERE archived_at IS NULL AND disabled_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, rev int64
		if err := rows.Scan(&id, &rev); err != nil {
			return nil, err
		}
		out[id] = rev
	}
	return out, rows.Err()
}

// work returns the batch's working copy of a device, cloning it on first use.
func (w *Writer) work(tc *touched, id int64) *devInfo {
	if c, ok := tc.devices[id]; ok {
		return c
	}
	orig := w.devices[id]
	c := *orig
	m := *orig.machine
	c.machine = &m
	tc.devices[id] = &c
	return &c
}

// commit installs the working copies after the transaction committed.
func (w *Writer) commit(tc *touched) {
	for id, c := range tc.devices {
		if cur, ok := w.devices[id]; ok && cur.rev == c.rev {
			w.devices[id] = c
		}
	}
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

func (w *Writer) apply(ctx context.Context, tx *sql.Tx, r Result, tc *touched, revs map[int64]int64) error {
	orig, ok := w.devices[r.DeviceID]
	if !ok || orig.rev != r.Rev || revs[r.DeviceID] != r.Rev {
		return nil // archived or reconfigured while the check was in flight
	}
	info := w.work(tc, r.DeviceID)
	eff := w.effective(info)
	if lc := info.status.LastCheck; lc != nil && time.UnixMilli(*lc).Add(eff.Stale).Before(r.At) {
		if err := w.expire(ctx, tx, r.DeviceID, info, time.UnixMilli(*lc).Add(eff.Stale), tc); err != nil {
			return err
		}
	}
	p := r.Ping
	var latUs any
	if p.LatencyMs != nil {
		latUs = int64(math.Round(*p.LatencyMs * 1000))
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO samples(device_id,ts,ok,latency_us,loss_pct,error_code) VALUES(?,?,?,?,?,?)`,
		r.DeviceID, store.Ms(r.At), p.OK(), latUs, p.LossPct(), string(p.Code)); err != nil {
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
	return nil
}

func (w *Writer) incident(ctx context.Context, tx *sql.Tx, id int64, info *devInfo, tr *state.Transition) error {
	if tr.To == state.Down {
		if w.set.Get().IncidentsFor == "primary" && !info.primary {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO incidents(device_id,observed_at)
			SELECT ?,? WHERE NOT EXISTS (SELECT 1 FROM incidents WHERE device_id=? AND ended_at IS NULL)`,
			id, store.Ms(tr.At), id)
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
	return nil
}

// ExpireGaps marks devices whose last check is older than the stale limit as
// unknown, closing open incidents with monitoring_gap.
func (w *Writer) ExpireGaps(ctx context.Context, now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	tc := newTouched()
	err := w.inTx(ctx, func(tx *sql.Tx) error {
		revs, err := activeRevs(ctx, tx)
		if err != nil {
			return err
		}
		for id, info := range w.devices {
			lc := info.status.LastCheck
			if lc == nil || revs[id] != info.rev {
				continue
			}
			g := time.UnixMilli(*lc).Add(w.effective(info).Stale)
			if g.After(now) || (info.machine.State == state.Unknown && info.machine.Streak == 0) {
				continue
			}
			if err := w.expire(ctx, tx, id, w.work(tc, id), g, tc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		w.lastErr = err.Error()
		return err
	}
	w.commit(tc)
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
