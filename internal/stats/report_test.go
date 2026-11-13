package stats

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/live"
	"sismon/internal/state"
	"sismon/internal/store"
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

func (e *env) incident(dev, obs, ended int64, reason string) {
	var end, rsn any
	if ended >= 0 {
		end, rsn = ended*1000, reason
	}
	e.exec(`INSERT INTO incidents(device_id,observed_at,ended_at,end_reason) VALUES(?,?,?,?)`, dev, obs*1000, end, rsn)
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
	if !near(d.Durations.Down, 20) {
		t.Fatalf("%+v", d.Durations)
	}
}

func TestLocationDegradedCountsAsAvailable(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("location", 1, state.Up, 0, 50)
	e.ivl("location", 1, state.Degraded, 50, 75)
	e.ivl("location", 1, state.Down, 75, 100)
	r := e.build(Query{From: T(0), To: T(100)}, 100)
	if l := r.Locations[0]; l.Availability == nil || !near(*l.Availability, 75) || !near(l.Durations.Degraded, 25) {
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
	r = e.build(Query{From: T(60), To: T(100), Archived: true}, 1000)
	if len(r.Devices) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestIncidentDurationFromObserved(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.incident(1, 100, 400, "recovered")
	r := e.build(Query{From: T(0), To: T(300)}, 1000)
	if len(r.Incidents) != 1 || !near(r.Incidents[0].DurationS, 200) || r.Incidents[0].ReasonText != "Revenire" || r.Devices[0].Incidents != 1 {
		t.Fatalf("%+v", r.Incidents)
	}
}

func TestIncidentsFilter(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.ivl("device", 1, state.Up, 0, 100)
	e.incident(1, 10, 30, "recovered")
	e.incident(1, 50, -1, "")
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
	e.incident(1, 10, 70, "recovered")
	e.incident(1, 100, 280, "recovered")
	e.incident(1, 300, 330, "monitoring_gap")
	r := e.build(Query{From: T(0), To: T(400)}, 400)
	if r.Devices[0].MTTRS == nil || !near(*r.Devices[0].MTTRS, 120) {
		t.Fatalf("%+v", r.Devices[0].MTTRS)
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

func TestArchivedFilter(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.device(1, 2, false, 0) // removed from an active location
	e.device(2, 3, true, 0)  // whole location archived
	e.exec(`UPDATE devices SET archived_at=? WHERE id IN (2,3)`, 50*1000)
	e.exec(`UPDATE locations SET archived_at=? WHERE id=2`, 50*1000)
	ids := func(r Report) (devs, locs []int64) {
		for _, d := range r.Devices {
			devs = append(devs, d.ID)
		}
		for _, l := range r.Locations {
			locs = append(locs, l.ID)
		}
		return
	}
	d, l := ids(e.build(Query{From: T(0), To: T(100)}, 100))
	if fmt.Sprint(d, l) != "[1] [1]" {
		t.Fatalf("excluded: devices %v locations %v", d, l)
	}
	d, l = ids(e.build(Query{From: T(0), To: T(100), Archived: true}, 100))
	if fmt.Sprint(d, l) != "[1 2 3] [1 2]" {
		t.Fatalf("included: devices %v locations %v", d, l)
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
		{url.Values{"archived": {"yes"}}, "archived"},
		{url.Values{"degraded": {"2"}}, "degraded"},
	}
	for _, c := range cases {
		if _, err := ParseQuery(c.v, now); fieldOf(t, err) != c.want {
			t.Errorf("%v: want %s", c.v, c.want)
		}
	}
	q, err = ParseQuery(url.Values{"from": {ms(now.Add(-time.Hour))}, "to": {ms(now.Add(time.Hour))}, "location": {"3"}}, now)
	if err != nil || !q.To.Equal(now) || q.LocationID == nil || *q.LocationID != 3 || q.Archived {
		t.Fatalf("%+v %v", q, err)
	}
	if q, err = ParseQuery(url.Values{"archived": {"1"}}, now); err != nil || !q.Archived {
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
	e.incident(1, now-600, -1, "")
	snap := live.Snapshot{Locations: []live.LocationView{
		{ID: 1, Name: "L1", State: state.Down, Devices: []live.DeviceView{
			{ID: 1, LocationID: 1, Name: "D1", Primary: true, State: state.Down, Since: (now - 480) * 1000, ErrorCode: "timeout"},
			{ID: 2, LocationID: 1, Name: "S", State: state.Down},
			{ID: 3, LocationID: 1, Name: "D3", Primary: true, State: state.Down, Since: (now - 60) * 1000, ErrorCode: "host_unreachable"},
		}},
		{ID: 2, Name: "L2", State: state.Down, Devices: []live.DeviceView{
			{ID: 4, LocationID: 2, Name: "D4", Primary: true, State: state.Down, Since: (now - 9000) * 1000},
		}},
	}}
	r, err := BuildReport24(context.Background(), e.st, snap, T(now))
	if err != nil {
		t.Fatal(err)
	}
	// One entry per location, the oldest outage first.
	if r.AffectedNow != 2 || len(r.Current) != 2 || r.Current[0].LocationID != 2 || r.Locations != 2 || len(r.Incidents) != 1 {
		t.Fatalf("%+v", r)
	}
	l1 := r.Current[1]
	if l1.Since != (now-600)*1000 || len(l1.Devices) != 2 {
		t.Fatalf("%+v", l1)
	}
	// The secondary device is left out; each primary keeps its own cause.
	if l1.Devices[0].ID != 1 || l1.Devices[0].ErrorCode != "timeout" || l1.Devices[1].ID != 3 || l1.Devices[1].ErrorCode != "host_unreachable" {
		t.Fatalf("%+v", l1.Devices)
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

func TestLocationOutages(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	e.device(1, 2, false, 0)
	e.ivl("location", 1, state.Up, 0, 10)
	e.ivl("location", 1, state.Down, 10, 20)
	e.ivl("location", 1, state.Down, 20, 30) // adjacent: one period
	e.ivl("location", 1, state.Unknown, 30, 32)
	e.ivl("location", 1, state.Down, 32, 40) // after missing data: separate
	e.ivl("location", 1, state.Degraded, 40, 50)
	e.ivl("location", 1, state.Up, 50, 60)
	e.ivl("location", 1, state.Down, 60, -1) // ongoing
	e.ivl("device", 1, state.Down, 10, 30)
	e.ivl("device", 1, state.Down, 32, 40)
	e.ivl("device", 1, state.Down, 60, -1)
	e.ivl("device", 2, state.Down, 40, 50)

	r := e.build(Query{From: T(0), To: T(100)}, 100)
	got := func(r Report) string {
		var s []string
		for _, o := range r.Outages {
			end := "…"
			if o.End != nil {
				end = fmt.Sprint(*o.End / 1000)
			}
			var devs []int64
			for _, d := range o.Devices {
				devs = append(devs, d.ID)
			}
			s = append(s, fmt.Sprintf("%s %d-%s %gs %v", o.State, o.Start/1000, end, o.DurationS, devs))
		}
		return strings.Join(s, "; ")
	}
	if want := "down 60-… 40s [1]; down 32-40 8s [1]; down 10-30 20s [1]"; got(r) != want {
		t.Fatalf("outages\n got %s\nwant %s", got(r), want)
	}
	if r.OutagesTotal != 3 || r.Locations[0].Outages != 3 {
		t.Fatalf("total %d, location %d", r.OutagesTotal, r.Locations[0].Outages)
	}
	if d := r.Locations[0].Durations; d.Down != 68 || d.Degraded != 10 || d.Unknown != 2 {
		t.Fatalf("durations %+v", d)
	}
	r = e.build(Query{From: T(0), To: T(100), Degraded: true}, 100)
	if want := "down 60-… 40s [1]; degraded 40-50 10s [2]; down 32-40 8s [1]; down 10-30 20s [1]"; got(r) != want {
		t.Fatalf("with degraded\n got %s\nwant %s", got(r), want)
	}
	if r.Locations[0].Outages != 3 {
		t.Fatal("degraded periods must not count as outages")
	}
	// a period starting before the report keeps its real start
	r = e.build(Query{From: T(15), To: T(35)}, 100)
	if want := "down 32-40 8s [1]; down 10-30 20s [1]"; got(r) != want {
		t.Fatalf("clipped period\n got %s\nwant %s", got(r), want)
	}
	segs := r.Locations[0].Segments
	if len(segs) == 0 || segs[0].Start != 15000 || segs[len(segs)-1].End != 35000 {
		t.Fatalf("segments %+v", segs)
	}
}

func TestCompactKeepsShortOutages(t *testing.T) {
	var segs []Segment
	for i := int64(0); i < 10*MaxSegments; i++ {
		segs = append(segs, Segment{i * 1000, (i + 1) * 1000, state.Up})
	}
	segs[4321].State = state.Down // one second in ~1.7 hours
	out := compact(segs, 0, int64(len(segs))*1000)
	if len(out) > MaxSegments {
		t.Fatalf("%d segments", len(out))
	}
	down := 0
	for _, s := range out {
		if s.State == state.Down {
			down++
			if s.Start > 4321000 || s.End < 4322000 {
				t.Fatalf("down slot %+v does not cover the outage", s)
			}
		}
	}
	if down != 1 {
		t.Fatalf("%d down slots", down)
	}
	if short := compact(segs[:5], 0, 5000); len(short) != 5 {
		t.Fatal("short timelines must stay unchanged")
	}
}
