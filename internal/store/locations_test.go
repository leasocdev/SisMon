package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/domain"
	"sismon/internal/settings"
	"sismon/internal/state"
)

var now = time.Unix(1_800_000_000, 0)

func input(t *testing.T, name string, hosts ...string) domain.LocationInput {
	t.Helper()
	in := domain.LocationInput{Name: name}
	for i, h := range hosts {
		in.Devices = append(in.Devices, domain.DeviceInput{Name: "D" + h, Host: h, Primary: i == 0})
	}
	if err := in.Normalize(settings.Defaults()); err != nil {
		t.Fatal(err)
	}
	return in
}

func save(t *testing.T, s *Store, id *int64, in domain.LocationInput, at time.Time) SaveResult {
	t.Helper()
	r, err := s.SaveLocation(context.Background(), id, in, at)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSaveNewLocation(t *testing.T) {
	s := openTemp(t)
	r := save(t, s, nil, input(t, "Centru", "10.0.0.1", "10.0.0.2"), now)
	if len(r.Added) != 2 {
		t.Fatalf("%+v", r)
	}
	locs, err := s.ListLocations(context.Background(), false)
	if err != nil || len(locs) != 1 || len(locs[0].Devices) != 2 || !locs[0].Devices[0].Primary || locs[0].Devices[1].Primary {
		t.Fatalf("%+v %v", locs, err)
	}
	ctx := context.Background()
	for _, id := range r.Added {
		st, start, ok, _ := OpenState(ctx, s.DB, EntityDevice, id)
		if !ok || st != state.Unknown || !start.Equal(now) {
			t.Fatalf("device %d interval %s %v", id, st, ok)
		}
	}
	if st, _, ok, _ := OpenState(ctx, s.DB, EntityLocation, r.LocationID); !ok || st != state.Unknown {
		t.Fatal("location interval")
	}
}

func TestSaveDuplicateNameConflict(t *testing.T) {
	s := openTemp(t)
	save(t, s, nil, input(t, "Centru", "10.0.0.1"), now)
	_, err := s.SaveLocation(context.Background(), nil, input(t, "centru", "10.0.0.9"), now)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Status != 409 || e.Field != "name" {
		t.Fatalf("%v", err)
	}
}

func TestSaveForeignDeviceIDRejected(t *testing.T) {
	s := openTemp(t)
	a := save(t, s, nil, input(t, "A", "10.0.0.1"), now)
	b := save(t, s, nil, input(t, "B", "10.0.0.2"), now)
	in := input(t, "B renamed", "10.0.0.3")
	in.Devices[0].ID = &a.Added[0]
	_, err := s.SaveLocation(context.Background(), &b.LocationID, in, now)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Status != 400 {
		t.Fatalf("%v", err)
	}
	l, _ := s.GetLocation(context.Background(), b.LocationID)
	if l.Name != "B" {
		t.Fatal("transaction not rolled back")
	}
}

func TestHostChangeResets(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	r := save(t, s, nil, input(t, "A", "10.0.0.1"), now)
	id := r.Added[0]
	s.DB.Exec(`INSERT INTO incidents(device_id,observed_at,confirmed_at) VALUES(?,?,?)`, id, Ms(now), Ms(now))
	s.DB.Exec(`UPDATE device_status SET state='down' WHERE device_id=?`, id)
	tx, _ := s.DB.Begin()
	CloseAndOpen(ctx, tx, EntityDevice, id, state.Down, now.Add(time.Minute))
	tx.Commit()
	in := input(t, "A", "10.0.0.5")
	in.Devices[0].ID = &id
	r2 := save(t, s, &r.LocationID, in, now.Add(2*time.Minute))
	if len(r2.HostChanged) != 1 {
		t.Fatalf("%+v", r2)
	}
	var rev int64
	var reason string
	s.DB.QueryRow(`SELECT rev FROM devices WHERE id=?`, id).Scan(&rev)
	s.DB.QueryRow(`SELECT end_reason FROM incidents WHERE device_id=?`, id).Scan(&reason)
	st, _, _, _ := OpenState(ctx, s.DB, EntityDevice, id)
	if rev != 2 || reason != "configuration" || st != state.Unknown {
		t.Fatalf("rev %d reason %q state %s", rev, reason, st)
	}
}

func TestRemovedDeviceArchived(t *testing.T) {
	s := openTemp(t)
	r := save(t, s, nil, input(t, "A", "10.0.0.1", "10.0.0.2"), now)
	in := input(t, "A", "10.0.0.1")
	in.Devices[0].ID = &r.Added[0]
	r2 := save(t, s, &r.LocationID, in, now.Add(time.Minute))
	if len(r2.Archived) != 1 || r2.Archived[0] != r.Added[1] {
		t.Fatalf("%+v", r2)
	}
	if _, _, ok, _ := OpenState(context.Background(), s.DB, EntityDevice, r.Added[1]); ok {
		t.Fatal("archived device interval still open")
	}
	all, _ := s.ListLocations(context.Background(), true)
	if len(all[0].Devices) != 2 || all[0].Devices[1].ArchivedAt == nil {
		t.Fatalf("%+v", all[0].Devices)
	}
}

func TestArchiveLocation(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	r := save(t, s, nil, input(t, "A", "10.0.0.1"), now)
	if _, err := s.ArchiveLocation(ctx, r.LocationID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	active, _ := s.ListLocations(ctx, false)
	all, _ := s.ListLocations(ctx, true)
	if len(active) != 0 || len(all) != 1 || all[0].ArchivedAt == nil {
		t.Fatalf("active %d all %d", len(active), len(all))
	}
	if _, err := s.ArchiveLocation(ctx, r.LocationID, now); err == nil {
		t.Fatal("double archive allowed")
	}
	// name becomes reusable
	save(t, s, nil, input(t, "A", "10.0.0.1"), now.Add(2*time.Minute))
}

func TestSwapHosts(t *testing.T) {
	s := openTemp(t)
	r := save(t, s, nil, input(t, "A", "10.0.0.1", "10.0.0.2"), now)
	in := input(t, "A", "10.0.0.2", "10.0.0.1")
	in.Devices[0].ID, in.Devices[1].ID = &r.Added[0], &r.Added[1]
	r2 := save(t, s, &r.LocationID, in, now.Add(time.Minute))
	if len(r2.HostChanged) != 2 {
		t.Fatalf("%+v", r2)
	}
}

func TestAuditWritten(t *testing.T) {
	s := openTemp(t)
	save(t, s, nil, input(t, "A", "10.0.0.1"), now)
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM audit WHERE action='save_location'`).Scan(&n)
	if n != 1 {
		t.Fatalf("audit rows %d", n)
	}
}

func TestRecomputeLocation(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	r := save(t, s, nil, input(t, "A", "10.0.0.1", "10.0.0.2"), now)
	s.DB.Exec(`UPDATE device_status SET state='up'`)
	s.DB.Exec(`UPDATE device_status SET state='down' WHERE device_id=?`, r.Added[1])
	tx, _ := s.DB.Begin()
	res, err := RecomputeLocation(ctx, tx, r.LocationID, now.Add(time.Minute))
	tx.Commit()
	if err != nil || res.State != state.Degraded {
		t.Fatalf("%+v %v", res, err)
	}
	if st, _, _, _ := OpenState(ctx, s.DB, EntityLocation, r.LocationID); st != state.Degraded {
		t.Fatal(st)
	}
}

func TestSetLocationOrder(t *testing.T) {
	s := openTemp(t)
	a := save(t, s, nil, input(t, "A", "10.0.0.1"), now).LocationID
	b := save(t, s, nil, input(t, "B", "10.0.0.2"), now).LocationID
	if err := s.SetLocationOrder(context.Background(), []int64{b, a}, now); err != nil {
		t.Fatal(err)
	}
	locs, _ := s.ListLocations(context.Background(), false)
	if len(locs) != 2 || locs[0].ID != b || locs[1].ID != a {
		t.Fatalf("%+v", locs)
	}
	err := s.SetLocationOrder(context.Background(), []int64{a, 999}, now)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Field != "ids" {
		t.Fatalf("%v", err)
	}
}

func TestSaveLocationsAtomic(t *testing.T) {
	s := openTemp(t)
	save(t, s, nil, input(t, "Existentă", "10.0.0.1"), now)
	_, err := s.SaveLocations(context.Background(), []domain.LocationInput{input(t, "Nouă", "10.0.0.2"), input(t, "Existentă", "10.0.0.3")}, now)
	if err == nil {
		t.Fatal("duplicate name must fail")
	}
	locs, _ := s.ListLocations(context.Background(), false)
	if len(locs) != 1 {
		t.Fatalf("partial import committed: %d locations", len(locs))
	}
	res, err := s.SaveLocations(context.Background(), []domain.LocationInput{input(t, "A", "10.0.1.1", "10.0.1.2"), input(t, "B", "10.0.2.1")}, now)
	if err != nil || len(res) != 2 {
		t.Fatal(res, err)
	}
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM audit WHERE action='import'`).Scan(&n)
	if n != 1 {
		t.Fatal("audit import", n)
	}
}

func TestSetLocationOrderRequiresFullSet(t *testing.T) {
	s := openTemp(t)
	a := save(t, s, nil, input(t, "A", "10.0.0.1"), now).LocationID
	save(t, s, nil, input(t, "B", "10.0.0.2"), now)
	err := s.SetLocationOrder(context.Background(), []int64{a}, now)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Field != "ids" || !strings.Contains(e.Message, "reîncărcați") {
		t.Fatalf("partial list must be rejected: %v", err)
	}
}
