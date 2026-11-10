package domain

import (
	"errors"
	"strings"
	"testing"

	"netwatch/internal/apperr"
	"netwatch/internal/settings"
)

func TestValidHost(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1": true, "2001:db8::1": true, "router.local": true, "a-b.example.md": true,
		"-c": false, "x;whoami": false, "a b": false, "a..b": false, "$(id)": false, "": false,
		strings.Repeat("a", 254): false, "bad-.x": false, " 10.0.0.1": false,
	}
	for h, want := range cases {
		if got := ValidHost(h); got != want {
			t.Errorf("ValidHost(%q)=%v want %v", h, got, want)
		}
	}
}

func valid() LocationInput {
	return LocationInput{Name: " Centru ", Devices: []DeviceInput{
		{Name: "Router", Host: "10.0.0.1", Primary: true},
		{Name: "PC", Host: "10.0.0.2", Kind: "PC"},
	}}
}

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Status != 400 {
		t.Fatalf("want 400 apperr, got %v", err)
	}
	return e.Field
}

func TestNormalizeOK(t *testing.T) {
	in := valid()
	if err := in.Normalize(settings.Defaults()); err != nil {
		t.Fatal(err)
	}
	if in.Name != "Centru" || in.Devices[0].Kind != "Altele" {
		t.Fatalf("%+v", in)
	}
}

func TestNormalizeErrors(t *testing.T) {
	lat := 47.0
	cases := []struct {
		want string
		mod  func(*LocationInput)
	}{
		{"name", func(l *LocationInput) { l.Name = "  " }},
		{"devices", func(l *LocationInput) { l.Devices[0].Primary = false }},
		{"devices.1.host", func(l *LocationInput) { l.Devices[0].Host = "A.local"; l.Devices[1].Host = "a.local" }},
		{"devices.0.host", func(l *LocationInput) { l.Devices[0].Host = "x;y" }},
		{"devices.1.name", func(l *LocationInput) { l.Devices[1].Name = "" }},
		{"lon", func(l *LocationInput) { l.Lat = &lat }},
		{"devices", func(l *LocationInput) {
			l.Devices = nil
			for i := 0; i < 51; i++ {
				l.Devices = append(l.Devices, DeviceInput{Name: "d", Host: "10.0.1." + string(rune('0'+i%10)) + "x", Primary: true})
			}
		}},
		{"devices", func(l *LocationInput) { l.Devices = nil }},
		{"address", func(l *LocationInput) { l.Address = strings.Repeat("a", 501) }},
		{"devices.0.interval_s", func(l *LocationInput) { v := 5; l.Devices[0].Overrides.IntervalS = &v }},
	}
	for _, c := range cases {
		in := valid()
		c.mod(&in)
		if got := fieldOf(t, in.Normalize(settings.Defaults())); got != c.want {
			t.Errorf("want field %s got %s", c.want, got)
		}
	}
}

func TestValidHostRejectsMalformedIPv4(t *testing.T) {
	for _, h := range []string{"999.1.1.1", "192.168.1.300", "10.0.0", "1.2.3.4.5"} {
		if ValidHost(h) {
			t.Errorf("%q must be rejected (numeric top-level label)", h)
		}
	}
	for _, h := range []string{"10.0.0.1", "router1.local", "a1.example.com", "host-2", "::1"} {
		if !ValidHost(h) {
			t.Errorf("%q must be accepted", h)
		}
	}
}
