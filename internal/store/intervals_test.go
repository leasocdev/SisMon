package store

import (
	"context"
	"testing"
	"time"

	"sismon/internal/state"
)

func ts(sec int64) time.Time { return time.Unix(1_800_000_000+sec, 0) }

type row struct {
	st    string
	start int64
	end   *int64
}

func intervals(t *testing.T, s *Store, id int64) []row {
	rs, err := s.DB.Query(`SELECT state,start_at,end_at FROM state_intervals WHERE entity='device' AND entity_id=? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	var out []row
	for rs.Next() {
		var r row
		rs.Scan(&r.st, &r.start, &r.end)
		out = append(out, r)
	}
	return out
}

func apply(t *testing.T, s *Store, st state.State, at time.Time) {
	ctx := context.Background()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := CloseAndOpen(ctx, tx, EntityDevice, 1, st, at); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseAndOpen(t *testing.T) {
	s := openTemp(t)
	apply(t, s, state.Up, ts(1))
	apply(t, s, state.Down, ts(3))
	apply(t, s, state.Down, ts(5))
	r := intervals(t, s, 1)
	if len(r) != 2 || r[0].st != "up" || r[0].start != Ms(ts(1)) || r[0].end == nil || *r[0].end != Ms(ts(3)) ||
		r[1].st != "down" || r[1].start != Ms(ts(3)) || r[1].end != nil {
		t.Fatalf("%+v", r)
	}
	st, start, ok, err := OpenState(context.Background(), s.DB, EntityDevice, 1)
	if err != nil || !ok || st != state.Down || !start.Equal(ts(3)) {
		t.Fatalf("open state %s %v %v %v", st, start, ok, err)
	}
}

func TestCloseAndOpenClampsBackwardsTime(t *testing.T) {
	s := openTemp(t)
	apply(t, s, state.Up, ts(5))
	apply(t, s, state.Down, ts(2))
	r := intervals(t, s, 1)
	if len(r) != 2 || *r[0].end != Ms(ts(5)) || r[0].start != Ms(ts(5)) || r[1].start != Ms(ts(5)) {
		t.Fatalf("%+v", r)
	}
}

func TestCloseOpen(t *testing.T) {
	s := openTemp(t)
	apply(t, s, state.Up, ts(1))
	ctx := context.Background()
	tx, _ := s.DB.BeginTx(ctx, nil)
	if err := CloseOpen(ctx, tx, EntityDevice, 1, ts(4)); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	if _, _, ok, _ := OpenState(ctx, s.DB, EntityDevice, 1); ok {
		t.Fatal("still open")
	}
}
