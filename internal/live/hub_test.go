package live

import (
	"testing"
	"time"

	"sismon/internal/state"
)

func seed() *Hub {
	h := NewHub()
	h.Replace([]LocationView{
		{ID: 1, Name: "A", State: state.Up, Devices: []DeviceView{
			{ID: 10, LocationID: 1, Primary: true, State: state.Up},
			{ID: 11, LocationID: 1, State: state.Up},
		}},
		{ID: 2, Name: "Z", State: state.Down, Devices: []DeviceView{{ID: 20, LocationID: 2, Primary: true, State: state.Down}}},
	})
	return h
}

func recv(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case e, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		return e
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	return Event{}
}

func TestHubSetDeviceRecomputesLocation(t *testing.T) {
	h := seed()
	ch, cancel := h.Subscribe(h.Snapshot().Seq)
	defer cancel()
	h.SetDevice(DeviceView{ID: 11, LocationID: 1, State: state.Down})
	var loc LocationView
	for _, l := range h.Snapshot().Locations {
		if l.ID == 1 {
			loc = l
		}
	}
	if loc.State != state.Degraded || len(loc.Devices) != 2 {
		t.Fatalf("%+v", loc)
	}
	e := recv(t, ch)
	if e.Type != "state" || e.Data.(LocationView).State != state.Degraded {
		t.Fatalf("%+v", e)
	}
}

func TestHubReplayAfterSeq(t *testing.T) {
	h := NewHub()
	h.SetHealth(Health{AvgCheckMs: 1})
	first := h.Snapshot().Seq
	h.SetHealth(Health{AvgCheckMs: 2})
	h.SetHealth(Health{AvgCheckMs: 3})
	ch, cancel := h.Subscribe(first)
	defer cancel()
	if e := recv(t, ch); e.Seq != first+1 {
		t.Fatalf("%+v", e)
	}
	if e := recv(t, ch); e.Seq != first+2 {
		t.Fatalf("%+v", e)
	}
	if _, ok := h.Since(0); ok {
		t.Fatal("seq 0 should require a snapshot")
	}
}

func TestHubSlowSubscriberDropped(t *testing.T) {
	h := seed()
	ch, _ := h.Subscribe(h.Snapshot().Seq)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 300; i++ {
			h.SetHealth(Health{LateChecks: i})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("hub blocked by slow subscriber")
	}
	n := 0
	for range ch {
		n++
	}
	if n >= 300 || h.Subscribers() != 0 {
		t.Fatalf("received %d, subscribers %d", n, h.Subscribers())
	}
}

func TestSnapshotOrder(t *testing.T) {
	h := seed()
	if ls := h.Snapshot().Locations; ls[0].Name != "Z" || ls[1].Name != "A" {
		t.Fatalf("%v %v", ls[0].Name, ls[1].Name)
	}
}
