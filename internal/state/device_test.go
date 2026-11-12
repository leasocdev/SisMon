package state

import (
	"testing"
	"time"
)

var t0 = time.Unix(1_800_000_000, 0)

func at(i int) time.Time { return t0.Add(time.Duration(i) * time.Minute) }

func TestHysteresisDownAndRecovery(t *testing.T) {
	d := &Device{State: Unknown}
	th := Thresholds{Failures: 3, Recoveries: 2}
	oks := []bool{true, true, false, false, false, true, true}
	want := []State{Unknown, Up, Up, Up, Down, Down, Up}
	var down, up *Transition
	for i, ok := range oks {
		tr := d.Apply(Check{At: at(i), OK: ok}, th)
		if d.State != want[i] {
			t.Fatalf("step %d: state %s want %s", i, d.State, want[i])
		}
		if tr != nil && tr.To == Down {
			down = tr
		}
		if tr != nil && tr.To == Up && i > 1 {
			up = tr
		}
	}
	if down == nil || !down.At.Equal(at(2)) {
		t.Fatalf("down transition %+v", down)
	}
	if up == nil || !up.At.Equal(at(5)) || up.From != Down {
		t.Fatalf("up transition %+v", up)
	}
}

func TestPendingDirection(t *testing.T) {
	d := &Device{State: Unknown}
	th := Thresholds{3, 2}
	for i, ok := range []bool{true, true, false} {
		d.Apply(Check{At: at(i), OK: ok}, th)
	}
	if d.Pending != Down {
		t.Fatalf("pending %q", d.Pending)
	}
	d.Apply(Check{At: at(3), OK: true}, th)
	if d.Pending != "" {
		t.Fatalf("pending %q after success", d.Pending)
	}
}

func TestMonitorErrorResetsStreak(t *testing.T) {
	d := &Device{State: Unknown}
	th := Thresholds{3, 2}
	d.Apply(Check{At: at(0), OK: true}, th)
	d.Apply(Check{At: at(1), OK: true}, th)
	tr := d.Apply(Check{At: at(2), MonitorError: true}, th)
	if d.State != Error || d.Streak != 0 || tr == nil || tr.To != Error || !tr.At.Equal(at(2)) {
		t.Fatalf("after error: %+v %+v", d, tr)
	}
	var last *Transition
	for i := 3; i <= 5; i++ {
		if tr := d.Apply(Check{At: at(i)}, th); tr != nil {
			last = tr
		}
	}
	if d.State != Down || last == nil || !last.At.Equal(at(3)) || last.From != Error {
		t.Fatalf("down after error: %+v %+v", d, last)
	}
}

func TestExpire(t *testing.T) {
	d := &Device{State: Down, Streak: 4, StreakStart: at(1), Pending: Up}
	d.Expire()
	if d.State != Unknown || d.Streak != 0 || d.Pending != "" || !d.StreakStart.IsZero() {
		t.Fatalf("%+v", d)
	}
}
