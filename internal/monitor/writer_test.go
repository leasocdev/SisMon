package monitor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"sismon/internal/domain"
	"sismon/internal/live"
	"sismon/internal/monitor/icmp"
	"sismon/internal/settings"
	"sismon/internal/state"
	"sismon/internal/store"
)

var base = time.UnixMilli(1_800_000_000_000)

func step(i int) time.Time { return base.Add(time.Duration(i) * time.Minute) }

type fixture struct {
	st   *store.Store
	hub  *live.Hub
	set  *settings.Holder
	w    *Writer
	loc  int64
	devs []int64
}

func newFixture(t *testing.T, s settings.Settings, primaries ...bool) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "w.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if len(primaries) == 0 {
		primaries = []bool{true}
	}
	in := domain.LocationInput{Name: "Centru"}
	for i, p := range primaries {
		in.Devices = append(in.Devices, domain.DeviceInput{Name: "D", Host: "10.0.0." + string(rune('1'+i)), Primary: p})
	}
	if err := in.Normalize(s); err != nil {
		t.Fatal(err)
	}
	r, err := st.SaveLocation(ctx, nil, in, base.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{st: st, hub: live.NewHub(), set: settings.NewHolder(s), loc: r.LocationID, devs: r.Added}
	if err := f.hub.Load(ctx, st); err != nil {
		t.Fatal(err)
	}
	f.w = NewWriter(st, f.hub, f.set)
	if err := f.w.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

func okRes(id int64, at time.Time) Result {
	l := 1.5
	return Result{DeviceID: id, Rev: 1, At: at, Ping: icmp.Result{Sent: 3, Received: 3, LatencyMs: &l}}
}

func failRes(id int64, at time.Time) Result {
	return Result{DeviceID: id, Rev: 1, At: at, Ping: icmp.Result{Sent: 3, Code: icmp.Timeout}}
}

func (f *fixture) feed(t *testing.T, rs ...Result) {
	t.Helper()
	for _, r := range rs {
		f.w.Submit(r)
	}
	if err := f.w.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type ivl struct {
	st         state.State
	start, end int64
}

func (f *fixture) intervals(t *testing.T, e store.Entity, id int64) []ivl {
	rows, err := f.st.DB.Query(`SELECT state,start_at,COALESCE(end_at,-1) FROM state_intervals WHERE entity=? AND entity_id=? ORDER BY start_at,id`, e, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []ivl
	for rows.Next() {
		var v ivl
		rows.Scan(&v.st, &v.start, &v.end)
		out = append(out, v)
	}
	return out
}

type inc struct {
	observed, ended int64
	reason          string
}

func (f *fixture) incidents(t *testing.T) []inc {
	rows, _ := f.st.DB.Query(`SELECT observed_at,COALESCE(ended_at,-1),COALESCE(end_reason,'') FROM incidents ORDER BY id`)
	defer rows.Close()
	var out []inc
	for rows.Next() {
		var v inc
		rows.Scan(&v.observed, &v.ended, &v.reason)
		out = append(out, v)
	}
	return out
}

func ms(t time.Time) int64 { return t.UnixMilli() }

func TestWriterDownIncidentRetroactive(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	d := f.devs[0]
	f.feed(t, okRes(d, step(0)), okRes(d, step(1)), failRes(d, step(2)), failRes(d, step(3)), failRes(d, step(4)))
	iv := f.intervals(t, store.EntityDevice, d)
	last := iv[len(iv)-1]
	prev := iv[len(iv)-2]
	if prev.st != state.Up || prev.start != ms(step(0)) || prev.end != ms(step(2)) || last.st != state.Down || last.start != ms(step(2)) || last.end != -1 {
		t.Fatalf("%+v", iv)
	}
	in := f.incidents(t)
	if len(in) != 1 || in[0].observed != ms(step(2)) || in[0].ended != -1 {
		t.Fatalf("%+v", in)
	}
	li := f.intervals(t, store.EntityLocation, f.loc)
	if l := li[len(li)-1]; l.st != state.Down || l.start != ms(step(2)) {
		t.Fatalf("%+v", li)
	}
	var samples int
	f.st.DB.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&samples)
	if samples != 5 {
		t.Fatal(samples)
	}
}

func TestWriterRecovery(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	d := f.devs[0]
	f.feed(t, okRes(d, step(0)), okRes(d, step(1)), failRes(d, step(2)), failRes(d, step(3)), failRes(d, step(4)), okRes(d, step(5)), okRes(d, step(6)))
	in := f.incidents(t)
	if len(in) != 1 || in[0].ended != ms(step(5)) || in[0].reason != "recovered" {
		t.Fatalf("%+v", in)
	}
	iv := f.intervals(t, store.EntityDevice, d)
	if l := iv[len(iv)-1]; l.st != state.Up || l.start != ms(step(5)) {
		t.Fatalf("%+v", iv)
	}
}

func TestWriterMonitorError(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	d := f.devs[0]
	f.feed(t, failRes(d, step(0)), failRes(d, step(1)), failRes(d, step(2)),
		Result{DeviceID: d, Rev: 1, At: step(3), Ping: icmp.Result{Sent: 3, Code: icmp.Permission}})
	in := f.incidents(t)
	if len(in) != 1 || in[0].reason != "monitor_error" || in[0].ended != ms(step(3)) {
		t.Fatalf("%+v", in)
	}
	iv := f.intervals(t, store.EntityDevice, d)
	if iv[len(iv)-1].st != state.Error {
		t.Fatalf("%+v", iv)
	}
}

func TestWriterIncidentsPrimaryOnly(t *testing.T) {
	s := settings.Defaults()
	s.IncidentsFor = "primary"
	f := newFixture(t, s, true, false)
	sec := f.devs[1]
	f.feed(t, failRes(sec, step(0)), failRes(sec, step(1)), failRes(sec, step(2)))
	if in := f.incidents(t); len(in) != 0 {
		t.Fatalf("%+v", in)
	}
	iv := f.intervals(t, store.EntityDevice, sec)
	if iv[len(iv)-1].st != state.Down {
		t.Fatalf("%+v", iv)
	}
}

func TestWriterDropsStaleRevision(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	ctx := context.Background()
	d := f.devs[0]
	in := domain.LocationInput{Name: "Centru", Devices: []domain.DeviceInput{{ID: &d, Name: "D", Host: "10.9.9.9", Primary: true}}}
	in.Normalize(settings.Defaults())
	if _, err := f.st.SaveLocation(ctx, &f.loc, in, step(0)); err != nil {
		t.Fatal(err)
	}
	f.w.Reload(ctx)
	f.feed(t, okRes(d, step(1))) // Rev 1, current rev is 2
	var n int
	f.st.DB.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&n)
	if n != 0 {
		t.Fatalf("stale result stored: %d", n)
	}
}

func TestWriterExpiresGapAtStartup(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	ctx := context.Background()
	d := f.devs[0]
	now := base.Add(2 * time.Hour)
	last := now.Add(-time.Hour)
	lastMs := ms(last)
	tx, _ := f.st.DB.Begin()
	store.CloseAndOpen(ctx, tx, store.EntityDevice, d, state.Down, last.Add(-5*time.Minute))
	tx.Commit()
	store.PutStatus(ctx, f.st.DB, store.Status{DeviceID: d, State: state.Down, Since: lastMs, LastCheck: &lastMs})
	f.st.DB.Exec(`INSERT INTO incidents(device_id,observed_at) VALUES(?,?)`, d, lastMs-300000)
	f.w = NewWriter(f.st, f.hub, f.set)
	f.w.now = func() time.Time { return now }
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- f.w.Run(cctx) }()
	gap := lastMs + 300_000
	deadline := time.Now().Add(3 * time.Second)
	for {
		iv := f.intervals(t, store.EntityDevice, d)
		if l := iv[len(iv)-1]; l.st == state.Unknown && l.start == gap {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("gap not applied: %+v", iv)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	in := f.incidents(t)
	if len(in) != 1 || in[0].ended != gap || in[0].reason != "monitoring_gap" {
		t.Fatalf("%+v", in)
	}
}

func TestWriterGapOnLateResult(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	d := f.devs[0]
	f.feed(t, okRes(d, step(0)), okRes(d, step(1)), okRes(d, step(11)))
	iv := f.intervals(t, store.EntityDevice, d)
	l := iv[len(iv)-1]
	if l.st != state.Unknown || l.start != ms(step(1).Add(300*time.Second)) {
		t.Fatalf("%+v", iv)
	}
	f.feed(t, okRes(d, step(12)))
	iv = f.intervals(t, store.EntityDevice, d)
	if l := iv[len(iv)-1]; l.st != state.Up || l.start != ms(step(11)) {
		t.Fatalf("%+v", iv)
	}
}

func TestWriterUpdatesHub(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	d := f.devs[0]
	f.feed(t, okRes(d, step(0)), okRes(d, step(1)))
	snap := f.hub.Snapshot()
	dv := snap.Locations[0].Devices[0]
	if dv.State != state.Up || dv.LatencyMs == nil || *dv.LatencyMs != 1.5 || dv.Name != "D" || !dv.Primary || snap.Locations[0].State != state.Up || snap.Locations[0].Since != ms(step(0)) {
		t.Fatalf("%+v %+v", dv, snap.Locations[0])
	}
}

// Review fix: a rolled-back batch must not advance the in-memory machines.
func TestWriterRollbackKeepsMemoryConsistent(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	d := f.devs[0]
	f.feed(t, okRes(d, step(0)), okRes(d, step(1)), failRes(d, step(2)), failRes(d, step(3)))
	f.st.DB.Exec(`CREATE TRIGGER boom BEFORE INSERT ON incidents BEGIN SELECT RAISE(ABORT,'boom'); END`)
	f.w.Submit(failRes(d, step(4)))
	if err := f.w.Flush(context.Background()); err == nil {
		t.Fatal("expected the batch to fail")
	}
	f.st.DB.Exec(`DROP TRIGGER boom`)
	f.feed(t, failRes(d, step(5)))
	in := f.incidents(t)
	iv := f.intervals(t, store.EntityDevice, d)
	if len(in) != 1 || iv[len(iv)-1].st != state.Down {
		t.Fatalf("outage lost after rollback: incidents %+v intervals %+v", in, iv)
	}
}

// Review fix: results of an old revision are dropped even before the writer
// has reloaded the configuration.
func TestWriterDropsResultsBeforeReload(t *testing.T) {
	f := newFixture(t, settings.Defaults())
	ctx := context.Background()
	d := f.devs[0]
	in := domain.LocationInput{Name: "Centru", Devices: []domain.DeviceInput{{ID: &d, Name: "D", Host: "10.9.9.9", Primary: true}}}
	in.Normalize(settings.Defaults())
	if _, err := f.st.SaveLocation(ctx, &f.loc, in, step(0)); err != nil {
		t.Fatal(err)
	}
	f.feed(t, okRes(d, step(1))) // no Reload: in-memory rev is still 1
	var n int
	f.st.DB.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&n)
	if n != 0 {
		t.Fatalf("stale result stored: %d", n)
	}
}
