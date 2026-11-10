package stats

import (
	"context"
	"errors"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"netwatch/internal/apperr"
	"netwatch/internal/live"
	"netwatch/internal/state"
	"netwatch/internal/store"
)

// T converts test seconds to a time.
func T(sec int64) time.Time { return time.Unix(sec, 0) }

type env struct {
	t  *testing.T
	st *store.Store
}

func newEnv(t *testing.T) *env {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "s.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &env{t, st}
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.st.DB.Exec(q, args...); err != nil {
		e.t.Fatal(err)
	}
}

// location + device created at createdSec
func (e *env) device(locID, devID int64, primary bool, createdSec int64) {
	e.exec(`INSERT OR IGNORE INTO locations(id,name,created_at) VALUES(?,?,?)`, locID, "L"+string(rune('0'+locID)), createdSec*1000)
	e.exec(`INSERT INTO devices(id,location_id,name,host,kind,is_primary,created_at) VALUES(?,?,?,?,?,?,?)`, devID, locID, "D"+string(rune('0'+devID)), "10.0.0."+string(rune('0'+devID)), "Router", primary, createdSec*1000)
}

func (e *env) ivl(entity string, id int64, st state.State, a int64, b int64) {
	var end any
	if b >= 0 {
		end = b * 1000
	}
	e.exec(`INSERT INTO state_intervals(entity,entity_id,state,start_at,end_at) VALUES(?,?,?,?,?)`, entity, id, st, a*1000, end)
}

func (e *env) incident(dev, obs, conf, ended int64, reason string) {
	var end, rsn any
	if ended >= 0 {
		end, rsn = ended*1000, reason
	}
	e.exec(`INSERT INTO incidents(device_id,observed_at,confirmed_at,ended_at,end_reason) VALUES(?,?,?,?,?)`, dev, obs*1000, conf*1000, end, rsn)
}

func (e *env) build(q Query, nowSec int64) Report {
	e.t.Helper()
	r, err := Build(context.Background(), e.st, q, T(nowSec))
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestDurationsClipping(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("device", 1, state.Up, 100, 150)
	e.ivl("device", 1, state.Down, 150, -1)
	r := e.build(Query{From: T(125), To: T(400)}, 400)
	d := r.Devices[0].Durations
	if !near(d.Up, 25) || !near(d.Down, 250) || !near(d.Unknown, 0) {
		t.Fatalf("%+v", d)
	}
}

func TestDurationsUncoveredIsUnknown(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("device", 1, state.Up, 200, 300)
	r := e.build(Query{From: T(100), To: T(400)}, 500)
	if d := r.Devices[0].Durations; !near(d.Unknown, 200) || !near(d.Up, 100) {
		t.Fatalf("%+v", d)
	}
}

func TestAvailabilityExcludesUnknown(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("device", 1, state.Up, 0, 60)
	e.ivl("device", 1, state.Down, 60, 80)
	e.ivl("device", 1, state.Unknown, 80, 100)
	r := e.build(Query{From: T(0), To: T(100)}, 100)
	d := r.Devices[0]
	if d.Availability == nil || !near(*d.Availability, 75) || d.Coverage == nil || !near(*d.Coverage, 80) {
		t.Fatalf("%+v %+v", d.Availability, d.Coverage)
	}
	if r.Summary.Availability == nil || !near(*r.Summary.Availability, 75) || !near(r.Summary.DownS, 20) {
		t.Fatalf("%+v", r.Summary)
	}
}

func TestLocationDegradedCountsAgainst(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("location", 1, state.Up, 0, 50)
	e.ivl("location", 1, state.Degraded, 50, 75)
	e.ivl("location", 1, state.Down, 75, 100)
	r := e.build(Query{From: T(0), To: T(100)}, 100)
	if l := r.Locations[0]; l.Availability == nil || !near(*l.Availability, 50) || !near(l.Durations.Degraded, 25) {
		t.Fatalf("%+v", l)
	}
}

func TestReportDeviceCreatedAfterPeriod(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.device(1, 2, false, 500)
	r := e.build(Query{From: T(0), To: T(100)}, 1000)
	if len(r.Devices) != 1 || r.Devices[0].ID != 1 {
		t.Fatalf("%+v", r.Devices)
	}
	if r.Devices[0].Availability != nil {
		t.Fatal("availability without up/down should be nil")
	}
	// archived before the period: no eligible time
	e.exec(`UPDATE devices SET archived_at=? WHERE id=1`, 50*1000)
	r = e.build(Query{From: T(60), To: T(100)}, 1000)
	if len(r.Devices) != 0 || r.Summary.Availability != nil {
		t.Fatalf("%+v", r)
	}
}

func TestIncidentDurationFromObserved(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.incident(1, 100, 220, 400, "recovered")
	r := e.build(Query{From: T(0), To: T(300)}, 1000)
	if len(r.Incidents) != 1 || !near(r.Incidents[0].DurationS, 200) || r.Incidents[0].ReasonText != "Revenire" || r.Devices[0].Incidents != 1 {
		t.Fatalf("%+v", r.Incidents)
	}
}

func TestIncidentsFilter(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("device", 1, state.Up, 0, 100)
	e.incident(1, 10, 20, 30, "recovered")
	e.incident(1, 50, 60, -1, "")
	all := e.build(Query{From: T(0), To: T(100)}, 100)
	act := e.build(Query{From: T(0), To: T(100), Incidents: "active"}, 100)
	cl := e.build(Query{From: T(0), To: T(100), Incidents: "closed"}, 100)
	if len(all.Incidents) != 2 || len(act.Incidents) != 1 || act.Incidents[0].EndedAt != nil || len(cl.Incidents) != 1 || cl.Incidents[0].EndedAt == nil {
		t.Fatalf("%d %d %d", len(all.Incidents), len(act.Incidents), len(cl.Incidents))
	}
	if *all.Devices[0].Availability != *act.Devices[0].Availability {
		t.Fatal("filter changed availability")
	}
	if act.Incidents[0].ReasonText != "În curs" {
		t.Fatal(act.Incidents[0].ReasonText)
	}
}

func TestMTTR(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.incident(1, 10, 20, 70, "recovered")
	e.incident(1, 100, 120, 280, "recovered")
	e.incident(1, 300, 320, 330, "monitoring_gap")
	r := e.build(Query{From: T(0), To: T(400)}, 400)
	if r.Summary.MTTRS == nil || !near(*r.Summary.MTTRS, 120) || r.Devices[0].MTTRS == nil || !near(*r.Devices[0].MTTRS, 120) {
		t.Fatalf("%+v", r.Summary.MTTRS)
	}
}

func TestFilters(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.device(2, 2, true, 0)
	e.exec(`UPDATE devices SET kind='PC' WHERE id=2`)
	loc := int64(2)
	r := e.build(Query{From: T(0), To: T(100), LocationID: &loc}, 100)
	if len(r.Devices) != 1 || r.Devices[0].ID != 2 || len(r.Locations) != 1 || r.Locations[0].ID != 2 {
		t.Fatalf("%+v", r.Devices)
	}
	r = e.build(Query{From: T(0), To: T(100), Kind: "Router"}, 100)
	if len(r.Devices) != 1 || r.Devices[0].ID != 1 {
		t.Fatalf("%+v", r.Devices)
	}
}

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Status != 400 {
		t.Fatalf("want 400, got %v", err)
	}
	return e.Field
}

func TestParseQuery(t *testing.T) {
	now := T(1_000_000_000)
	q, err := ParseQuery(url.Values{}, now)
	if err != nil || !q.To.Equal(now) || q.To.Sub(q.From) != 24*time.Hour || q.Incidents != "all" {
		t.Fatalf("%+v %v", q, err)
	}
	ms := func(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }
	cases := []struct {
		v    url.Values
		want string
	}{
		{url.Values{"from": {ms(now.Add(-401 * 24 * time.Hour))}, "to": {ms(now)}}, "from"},
		{url.Values{"from": {ms(now)}, "to": {ms(now)}}, "to"},
		{url.Values{"from": {"abc"}}, "from"},
		{url.Values{"from": {"NaN"}}, "from"},
		{url.Values{"location": {"x"}}, "location"},
		{url.Values{"incidents": {"maybe"}}, "incidents"},
	}
	for _, c := range cases {
		if _, err := ParseQuery(c.v, now); fieldOf(t, err) != c.want {
			t.Errorf("%v: want %s", c.v, c.want)
		}
	}
	q, err = ParseQuery(url.Values{"from": {ms(now.Add(-time.Hour))}, "to": {ms(now.Add(time.Hour))}, "location": {"3"}}, now)
	if err != nil || !q.To.Equal(now) || q.LocationID == nil || *q.LocationID != 3 {
		t.Fatalf("%+v %v", q, err)
	}
}

func TestTimelineFillsGaps(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("device", 1, state.Up, 100, 250)
	e.ivl("device", 1, state.Down, 300, 400)
	segs, err := Timeline(context.Background(), e.st, 1, T(100), T(450), T(1000))
	if err != nil {
		t.Fatal(err)
	}
	want := []Segment{{100000, 250000, state.Up}, {250000, 300000, state.Unknown}, {300000, 400000, state.Down}, {400000, 450000, state.Unknown}}
	if len(segs) != len(want) {
		t.Fatalf("%+v", segs)
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Fatalf("%d: %+v want %+v", i, segs[i], want[i])
		}
	}
}

func TestReport24Current(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	now := int64(100_000)
	e.incident(1, now-600, now-480, -1, "")
	snap := live.Snapshot{Locations: []live.LocationView{{ID: 1, Name: "L1", State: state.Down, Devices: []live.DeviceView{
		{ID: 1, LocationID: 1, Name: "D1", Primary: true, State: state.Down, Since: (now - 480) * 1000},
		{ID: 2, LocationID: 1, Name: "S", State: state.Down},
	}}}}
	r, err := BuildReport24(context.Background(), e.st, snap, T(now))
	if err != nil {
		t.Fatal(err)
	}
	if r.Current[0].DeviceID != 1 {
		t.Fatalf("device id missing: %+v", r.Current[0])
	}
	if r.AffectedNow != 1 || len(r.Current) != 1 || r.Current[0].Since != (now-600)*1000 || r.Locations != 1 || len(r.Incidents) != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestTimelineForLocation(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("location", 1, state.Up, 100, 200)
	e.ivl("location", 1, state.Degraded, 200, 300)
	segs, err := TimelineFor(context.Background(), e.st, store.EntityLocation, 1, T(100), T(400), T(1000))
	if err != nil {
		t.Fatal(err)
	}
	want := []Segment{{100000, 200000, state.Up}, {200000, 300000, state.Degraded}, {300000, 400000, state.Unknown}}
	if len(segs) != len(want) {
		t.Fatalf("%+v", segs)
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Fatalf("%d: %+v", i, segs[i])
		}
	}
	if _, err := TimelineFor(context.Background(), e.st, store.EntityLocation, 99, T(100), T(400), T(1000)); err == nil {
		t.Fatal("unknown location must fail")
	}
}
