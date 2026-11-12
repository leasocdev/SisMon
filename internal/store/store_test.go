package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "sismon.db"), Options{})
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
	if from != 0 || to != SchemaVersion {
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
	type X struct {
		A string
		B int
	}
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

// Review fix: read-then-write transactions must wait for the write lock
// instead of failing with SQLITE_BUSY_SNAPSHOT.
func TestImmediateTransactionsWait(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta`).Scan(&n)
	done := make(chan error, 1)
	go func() { done <- s.SetMeta(ctx, "other", 1) }()
	time.Sleep(200 * time.Millisecond)
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta(key,value) VALUES('mine','1')`); err != nil {
		t.Fatalf("write after concurrent commit failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("concurrent writer failed: %v", err)
	}
}

// Review fix: 60 devices × 1 check/min × 90 days of raw samples must fit the
// spec's 300 MB budget together with the rest of the data.
func TestSamplesStorageBudget(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	s := openTemp(t)
	ctx := context.Background()
	tx, _ := s.DB.BeginTx(ctx, nil)
	const days = 2
	base := int64(1_800_000_000_000)
	for m := int64(0); m < days*1440; m++ {
		for d := int64(1); d <= 60; d++ {
			if _, err := tx.ExecContext(ctx, `INSERT INTO samples(device_id,ts,ok,latency_us,loss_pct,error_code) VALUES(?,?,1,?,0,'')`, d, base+m*60_000+d*37, 400+(m*7+d*13)%9000); err != nil {
				t.Fatal(err)
			}
		}
	}
	tx.Commit()
	s.DB.Exec(`VACUUM`)
	var pages, size int64
	s.DB.QueryRow(`PRAGMA page_count`).Scan(&pages)
	s.DB.QueryRow(`PRAGMA page_size`).Scan(&size)
	perRow := float64(pages*size) / float64(days*1440*60)
	projected := perRow * 60 * 1440 * 90 / 1e6
	t.Logf("%.1f bytes/row, 90-day projection %.0f MB", perRow, projected)
	if projected > 250 {
		t.Fatalf("raw samples alone project to %.0f MB (> 250 MB)", projected)
	}
}
