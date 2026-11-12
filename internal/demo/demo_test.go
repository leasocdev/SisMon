package demo

import (
	"context"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"sismon/internal/auth"
	"sismon/internal/monitor/icmp"
	"sismon/internal/store"
)


func TestScenarioDeterministic(t *testing.T) {
	start := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	s := Scenario{Start: start}
	at := func(min float64) time.Time { return start.Add(time.Duration(min * float64(time.Minute))) }
	cahul := netip.MustParseAddr(HostOf("Cahul", 0))
	if s.Result(cahul, at(2)).OK() {
		t.Fatal("Cahul router must be down at minute 2")
	}
	if !s.Result(cahul, at(10)).OK() {
		t.Fatal("Cahul router must be up at minute 10")
	}
	a, b := s.Result(cahul, at(10)), s.Result(cahul, at(10))
	if *a.LatencyMs != *b.LatencyMs {
		t.Fatal("not deterministic")
	}
	ap := netip.MustParseAddr(HostOf("Edineț", 1))
	if r := s.Result(ap, at(1)); r.Code != icmp.Permission {
		t.Fatalf("Edineț AP must be a monitor error at minute 1: %+v", r)
	}
	ok := netip.MustParseAddr(HostOf("Ungheni", 0))
	for m := 0.0; m < 120; m += 0.5 {
		if r := s.Result(ok, at(m)); !r.OK() || *r.LatencyMs < 2 || *r.LatencyMs > 15 {
			t.Fatalf("Ungheni at %v: %+v", m, r)
		}
	}
}

func TestSeedOnce(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "d.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for i := 0; i < 2; i++ {
		if err := Seed(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	locs, _ := st.ListLocations(ctx, false)
	if len(locs) != 12 {
		t.Fatal(len(locs))
	}
	var hash string
	st.GetMeta(ctx, "password_hash", &hash)
	if !auth.VerifyPassword(hash, Password) {
		t.Fatal("password")
	}
	var id struct{ Name string }
	st.GetMeta(ctx, "identity", &id)
	if id.Name != "SisMon Demo" {
		t.Fatal(id.Name)
	}
}
