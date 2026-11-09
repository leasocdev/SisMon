package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "netwatch.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenCreatesSchema(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	var v int
	if ok, err := s.GetMeta(ctx, "schema_version", &v); err != nil || !ok || v != SchemaVersion {
		t.Fatalf("schema_version %d ok=%v err=%v", v, ok, err)
	}
	for _, tbl := range []string{"locations", "devices", "device_status", "state_intervals", "samples", "samples_hourly", "incidents", "audit", "sessions", "meta"} {
		var n int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s missing", tbl)
		}
	}
	var mode string
	s.DB.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	if mode != "wal" {
		t.Fatalf("journal_mode %q", mode)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	s, err := Open(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s.SetMeta(context.Background(), "schema_version", 99)
	s.Close()
	if _, err := Open(context.Background(), path, Options{}); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("err %v", err)
	}
}

func TestBeforeMigrateCalled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE legacy(x)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var from, to = -1, -1
	s, err := Open(context.Background(), path, Options{BeforeMigrate: func(_ *Store, f, tt int) error { from, to = f, tt; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if from != 0 || to != 1 {
		t.Fatalf("callback (%d,%d)", from, to)
	}
}

func TestFreshDatabaseSkipsBeforeMigrate(t *testing.T) {
	called := false
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "f.db"), Options{BeforeMigrate: func(*Store, int, int) error { called = true; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if called {
		t.Fatal("callback on fresh db")
	}
}

func TestMetaRoundTrip(t *testing.T) {
	s := openTemp(t)
	type X struct{ A string; B int }
	ctx := context.Background()
	if err := s.SetMeta(ctx, "x", X{"ă", 3}); err != nil {
		t.Fatal(err)
	}
	var got X
	if ok, err := s.GetMeta(ctx, "x", &got); !ok || err != nil || got != (X{"ă", 3}) {
		t.Fatalf("%+v %v %v", got, ok, err)
	}
	if ok, _ := s.GetMeta(ctx, "missing", &got); ok {
		t.Fatal("missing key reported present")
	}
}

func TestVacuumInto(t *testing.T) {
	s := openTemp(t)
	dest := filepath.Join(t.TempDir(), "copy.db")
	if err := s.VacuumInto(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	c, _ := sql.Open("sqlite", dest)
	defer c.Close()
	var r string
	c.QueryRow(`PRAGMA integrity_check`).Scan(&r)
	if r != "ok" {
		t.Fatalf("integrity %q", r)
	}
}
