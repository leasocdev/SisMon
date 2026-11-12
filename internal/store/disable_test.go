package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/state"
)

func TestDisableAndEnableDevice(t *testing.T) {
	ctx := context.Background()
	s := openTemp(t)
	r := save(t, s, nil, input(t, "Centru", "10.0.0.1", "10.0.0.2"), now)
	prim, sec := r.Added[0], r.Added[1]

	// Secondary down → location degraded, with an open incident.
	for _, id := range []int64{prim, sec} {
		st := state.Up
		if id == sec {
			st = state.Down
		}
		if err := PutStatus(ctx, s.DB, Status{DeviceID: id, State: st}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec(`INSERT INTO incidents(device_id,observed_at) VALUES(?,?)`, sec, Ms(now)); err != nil {
		t.Fatal(err)
	}

	// Disable the secondary until tomorrow.
	t1 := now.Add(time.Hour)
	until := Ms(t1.Add(24 * time.Hour))
	in := input(t, "Centru", "10.0.0.1", "10.0.0.2")
	in.Devices[0].ID, in.Devices[1].ID = &prim, &sec
	in.Devices[1].Disabled, in.Devices[1].DisabledUntil = true, &until
	res := save(t, s, &r.LocationID, in, t1)
	if len(res.Disabled) != 1 || res.Disabled[0] != sec {
		t.Fatalf("%+v", res)
	}
	locs, _ := s.ListLocations(ctx, false)
	d := locs[0].Devices[1]
	if !d.Disabled || d.DisabledAt == nil || *d.DisabledAt != Ms(t1) || d.DisabledUntil == nil || *d.DisabledUntil != until {
		t.Fatalf("device %+v", d)
	}
	if _, _, ok, _ := OpenState(ctx, s.DB, EntityDevice, sec); ok {
		t.Fatal("disabled device keeps an open interval")
	}
	var reason string
	if err := s.DB.QueryRow(`SELECT end_reason FROM incidents WHERE device_id=?`, sec).Scan(&reason); err != nil || reason != "disabled" {
		t.Fatalf("incident reason %q %v", reason, err)
	}
	if st, _, _, _ := OpenState(ctx, s.DB, EntityLocation, r.LocationID); st != state.Up {
		t.Fatalf("location %s, want up without the disabled device", st)
	}
	devs, _ := ListDevices(ctx, s.DB, false)
	if len(devs) != 2 {
		t.Fatalf("disabled devices stay in the configuration: %d", len(devs))
	}

	// Saving again with the same date keeps it disabled, no new interval.
	save(t, s, &r.LocationID, in, t1.Add(time.Minute))
	if _, _, ok, _ := OpenState(ctx, s.DB, EntityDevice, sec); ok {
		t.Fatal("re-save opened an interval")
	}

	// Manual enable restarts from unknown.
	t2 := t1.Add(2 * time.Hour)
	in.Devices[1].Disabled, in.Devices[1].DisabledUntil = false, nil
	res = save(t, s, &r.LocationID, in, t2)
	if len(res.Enabled) != 1 {
		t.Fatalf("%+v", res)
	}
	if st, start, ok, _ := OpenState(ctx, s.DB, EntityDevice, sec); !ok || st != state.Unknown || !start.Equal(t2) {
		t.Fatalf("enabled interval %s %v %v", st, start, ok)
	}
	if st, _, _, _ := OpenState(ctx, s.DB, EntityLocation, r.LocationID); st != state.Unknown {
		t.Fatalf("location %s, want unknown while the secondary has no data", st)
	}
}

func TestDisableUntilMustBeFuture(t *testing.T) {
	s := openTemp(t)
	past := Ms(now.Add(-time.Minute))
	in := input(t, "Centru", "10.0.0.1", "10.0.0.2")
	in.Devices[1].Disabled, in.Devices[1].DisabledUntil = true, &past
	_, err := s.SaveLocation(context.Background(), nil, in, now)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Field != "devices.1.disabled_until" {
		t.Fatalf("%v", err)
	}
}

func TestEnableExpired(t *testing.T) {
	ctx := context.Background()
	s := openTemp(t)
	until := Ms(now.Add(time.Hour))
	in := input(t, "Centru", "10.0.0.1", "10.0.0.2", "10.0.0.3")
	in.Devices[1].Disabled, in.Devices[1].DisabledUntil = true, &until
	in.Devices[2].Disabled = true // no date: stays disabled
	r := save(t, s, nil, in, now)
	if len(r.Disabled) != 2 {
		t.Fatalf("%+v", r)
	}
	if n, err := s.EnableExpired(ctx, now.Add(59*time.Minute)); err != nil || n != 0 {
		t.Fatalf("early: %d %v", n, err)
	}
	if n, err := s.EnableExpired(ctx, now.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("due: %d %v", n, err)
	}
	devs, _ := ListDevices(ctx, s.DB, false)
	if devs[1].Disabled || !devs[2].Disabled {
		t.Fatalf("%+v", devs)
	}
	if st, _, ok, _ := OpenState(ctx, s.DB, EntityDevice, r.Added[1]); !ok || st != state.Unknown {
		t.Fatal("re-enabled device has no unknown interval")
	}
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM audit WHERE action='enable_device_auto'`).Scan(&n)
	if n != 1 {
		t.Fatalf("audit %d", n)
	}
}

func TestMigrateV1KeepsDevices(t *testing.T) {
	path := t.TempDir() + "/v1.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{schemaV1, `INSERT INTO meta(key,value) VALUES('schema_version','1')`,
		`INSERT INTO locations(id,name,created_at) VALUES(1,'A',0)`,
		`INSERT INTO devices(id,location_id,name,host,is_primary,created_at) VALUES(1,1,'R','10.0.0.1',1,0)`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	s, err := Open(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	devs, err := ListDevices(context.Background(), s.DB, false)
	if err != nil || len(devs) != 1 || devs[0].Disabled || devs[0].DisabledUntil != nil {
		t.Fatalf("%+v %v", devs, err)
	}
}
