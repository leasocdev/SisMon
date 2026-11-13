package stats

import (
	"context"
	"testing"
	"time"

	"sismon/internal/state"
)

func TestLatencyResolution(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	start := int64(1_000_000)
	tx, _ := e.st.DB.Begin()
	for i := int64(0); i < 3*1440; i++ {
		tx.Exec(`INSERT INTO samples(device_id,ts,ok,latency_us,loss_pct) VALUES(1,?,1,?,0)`, (start+i*60)*1000, (i%10+1)*1000)
	}
	for h := int64(0); h < 30*24; h++ {
		tx.Exec(`INSERT INTO samples_hourly(device_id,hour_ts,checks,ok_checks,lat_avg,lat_min,lat_max,loss_avg) VALUES(1,?,60,60,5,1,10,0)`, (start-31*86400+h*3600)*1000)
	}
	tx.Commit()
	pts, err := Latency(context.Background(), e.st, 1, T(start), T(start+3*86400))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) == 0 || len(pts) > 500 {
		t.Fatalf("%d points", len(pts))
	}
	if gap := pts[1].T - pts[0].T; gap != int64(15*time.Minute/time.Millisecond) {
		t.Fatalf("bucket %d ms", gap)
	}
	for _, p := range pts {
		if p.Avg == nil || *p.Min > *p.Avg || *p.Avg > *p.Max {
			t.Fatalf("%+v", p)
		}
	}
	long, err := Latency(context.Background(), e.st, 1, T(start-31*86400), T(start-86400))
	if err != nil || len(long) == 0 || len(long) > 500 || *long[0].Avg != 5 || *long[0].Min != 1 || *long[0].Max != 10 {
		t.Fatalf("%d %v", len(long), err)
	}
	short, _ := Latency(context.Background(), e.st, 1, T(start), T(start+3600))
	if len(short) != 60 {
		t.Fatalf("raw points %d", len(short))
	}
}

func TestReportPerformanceOneYear(t *testing.T) {
	if testing.Short() {
		t.Skip("performance test skipped in -short")
	}
	if raceEnabled {
		t.Skip("time budget is not measurable under -race; CI runs this test separately without it")
	}
	e := newEnv(t)
	year := int64(365 * 86400)
	now := int64(2_000_000_000)
	start := now - year
	tx, _ := e.st.DB.Begin()
	for l := int64(1); l <= 20; l++ {
		tx.Exec(`INSERT INTO locations(id,name,created_at) VALUES(?,?,?)`, l, "L"+itoa(l), start*1000)
		for k := int64(0); k < 3; k++ {
			id := (l-1)*3 + k + 1
			tx.Exec(`INSERT INTO devices(id,location_id,name,host,is_primary,created_at) VALUES(?,?,?,?,?,?)`, id, l, "D", "10.0.0."+itoa(id), k == 0, start*1000)
		}
	}
	insert := func(entity string, id int64) {
		step := int64(8640) // 10 changes per day
		for ts := start; ts < now; ts += step {
			st := state.Up
			if (ts/step)%2 == 1 {
				st = state.Down
			}
			end := ts + step
			tx.Exec(`INSERT INTO state_intervals(entity,entity_id,state,start_at,end_at) VALUES(?,?,?,?,?)`, entity, id, st, ts*1000, end*1000)
		}
	}
	for id := int64(1); id <= 60; id++ {
		insert("device", id)
	}
	for id := int64(1); id <= 20; id++ {
		insert("location", id)
	}
	tx.Commit()
	begin := time.Now()
	r, err := Build(context.Background(), e.st, Query{From: T(start), To: T(now), Incidents: "all"}, T(now))
	took := time.Since(begin)
	t.Logf("report took %v", took)
	if err != nil || len(r.Devices) != 60 {
		t.Fatal(err)
	}
	if took > 500*time.Millisecond {
		t.Fatalf("report took %v", took)
	}
	t.Logf("one-year report: %v", took)
}

func itoa(v int64) string {
	return func() string {
		b := []byte{}
		if v == 0 {
			return "0"
		}
		for v > 0 {
			b = append([]byte{byte('0' + v%10)}, b...)
			v /= 10
		}
		return string(b)
	}()
}

func TestLatencyPointBudget(t *testing.T) {
	e := newEnv(t)
	e.device(1, 1, true, 0)
	start := int64(1_000_000)
	tx, _ := e.st.DB.Begin()
	for i := int64(0); i < 7*1440; i++ {
		tx.Exec(`INSERT INTO samples(device_id,ts,ok,latency_us,loss_pct) VALUES(1,?,1,1000,0)`, (start+i*60)*1000)
	}
	tx.Commit()
	for _, span := range []int64{7 * 86400, 72 * 3600} {
		pts, err := Latency(context.Background(), e.st, 1, T(start), T(start+span))
		if err != nil || len(pts) == 0 || len(pts) > 500 {
			t.Fatalf("span %d: %d points %v", span, len(pts), err)
		}
	}
	day, _ := Latency(context.Background(), e.st, 1, T(start), T(start+86400))
	if gap := day[1].T - day[0].T; gap >= int64(15*time.Minute/time.Millisecond) {
		t.Fatalf("24h should stay raw, bucket %d ms", gap)
	}
}
