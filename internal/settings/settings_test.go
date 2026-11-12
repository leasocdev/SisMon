package settings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"sismon/internal/apperr"
)

type memMeta map[string][]byte

func (m memMeta) GetMeta(_ context.Context, k string, v any) (bool, error) {
	b, ok := m[k]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(b, v)
}

func (m memMeta) SetMeta(_ context.Context, k string, v any) error {
	b, err := json.Marshal(v)
	m[k] = b
	return err
}

func ptr(v int) *int { return &v }

func field(t *testing.T, err error) string {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) {
		t.Fatalf("not apperr: %v", err)
	}
	if e.Status != 400 || e.Message == "" {
		t.Fatalf("bad error %+v", e)
	}
	return e.Field
}

func TestDefaultsValid(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatal(err)
	}
	d := Defaults()
	if d.IntervalS != 60 || d.Packets != 3 || d.TimeoutS != 2 || d.Failures != 3 || d.Recoveries != 2 || d.StaleS != 300 || d.IncidentsFor != "all" || !d.PreferIPv4 || d.BackupHour != 3 {
		t.Fatalf("%+v", d)
	}
}

func TestLimits(t *testing.T) {
	cases := []struct {
		f     string
		apply func(*Settings)
	}{
		{"interval_s", func(s *Settings) { s.IntervalS = 9 }},
		{"interval_s", func(s *Settings) { s.IntervalS = 3601; s.StaleS = 86400 }},
		{"packets", func(s *Settings) { s.Packets = 0 }},
		{"packets", func(s *Settings) { s.Packets = 11 }},
		{"timeout_s", func(s *Settings) { s.TimeoutS = 0 }},
		{"timeout_s", func(s *Settings) { s.TimeoutS = 11 }},
		{"failures", func(s *Settings) { s.Failures = 0 }},
		{"failures", func(s *Settings) { s.Failures = 21 }},
		{"recoveries", func(s *Settings) { s.Recoveries = 0 }},
		{"recoveries", func(s *Settings) { s.Recoveries = 21 }},
		{"stale_s", func(s *Settings) { s.StaleS = 29 }},
		{"stale_s", func(s *Settings) { s.StaleS = 86401 }},
		{"backup_hour", func(s *Settings) { s.BackupHour = 24 }},
	}
	for _, c := range cases {
		s := Defaults()
		c.apply(&s)
		if got := field(t, s.Validate()); got != c.f {
			t.Errorf("want field %s got %s (%+v)", c.f, got, s)
		}
	}
}

func TestCheckFitsInterval(t *testing.T) {
	s := Defaults()
	s.IntervalS, s.Packets, s.TimeoutS, s.StaleS = 10, 3, 3, 300
	if f := field(t, s.Validate()); f != "interval_s" {
		t.Fatalf("field %s", f)
	}
	s.IntervalS = 12
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStaleRule(t *testing.T) {
	s := Defaults()
	s.StaleS = 239
	if f := field(t, s.Validate()); f != "stale_s" {
		t.Fatalf("field %s", f)
	}
	s.StaleS = 240
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOverridesValidated(t *testing.T) {
	s := Defaults()
	if _, err := s.Apply(Overrides{IntervalS: ptr(5)}); field(t, err) != "interval_s" {
		t.Fatal("interval override")
	}
	if _, err := s.Apply(Overrides{Failures: ptr(10)}); field(t, err) != "stale_s" {
		t.Fatal("failures override")
	}
	e, err := s.Apply(Overrides{Recoveries: ptr(4)})
	if err != nil || e.Recoveries != 4 || e.Failures != 3 || e.Interval.Seconds() != 60 || e.Stale.Seconds() != 300 || e.Packets != 3 || e.Timeout.Seconds() != 2 {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestIncidentsForValues(t *testing.T) {
	s := Defaults()
	s.IncidentsFor = "x"
	if f := field(t, s.Validate()); f != "incidents_for" {
		t.Fatal(f)
	}
	s.IncidentsFor = "primary"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestHolderPersists(t *testing.T) {
	ctx := context.Background()
	st := memMeta{}
	h, err := Load(ctx, st)
	if err != nil || h.Get() != Defaults() {
		t.Fatalf("%v %+v", err, h.Get())
	}
	s := Defaults()
	s.Packets = 4
	if err := h.Save(ctx, st, s); err != nil {
		t.Fatal(err)
	}
	h2, _ := Load(ctx, st)
	if h2.Get().Packets != 4 {
		t.Fatal("not persisted")
	}
	bad := Defaults()
	bad.Packets = 0
	if err := h.Save(ctx, st, bad); err == nil || h.Get().Packets != 4 {
		t.Fatal("invalid saved")
	}
}
