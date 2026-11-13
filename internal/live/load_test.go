package live

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"sismon/internal/domain"
	"sismon/internal/settings"
	"sismon/internal/store"
)

func TestLoadListsDisabledDevices(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "s.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now()
	until := now.Add(time.Hour).UnixMilli()
	in := domain.LocationInput{Name: "A", Devices: []domain.DeviceInput{
		{Name: "Router", Host: "10.0.0.1", Primary: true},
		{Name: "AP", Host: "10.0.0.2", Disabled: true, DisabledUntil: &until},
	}}
	if err := in.Normalize(settings.Defaults()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveLocation(ctx, nil, in, now); err != nil {
		t.Fatal(err)
	}
	h := NewHub()
	if err := h.Load(ctx, st); err != nil {
		t.Fatal(err)
	}
	l := h.Snapshot().Locations[0]
	if len(l.Devices) != 1 || l.Devices[0].Name != "Router" {
		t.Fatalf("monitored devices %+v", l.Devices)
	}
	if len(l.DisabledDevices) != 1 {
		t.Fatalf("disabled devices %+v", l.DisabledDevices)
	}
	d := l.DisabledDevices[0]
	if d.Name != "AP" || d.Host != "10.0.0.2" || d.Since != now.UnixMilli() || d.Until == nil || *d.Until != until {
		t.Fatalf("disabled device %+v", d)
	}
}
