package state

import "time"

// Device is the hysteresis state machine for one device.
type Device struct {
	State       State
	Pending     State // "" or Up/Down: direction of the running streak when it differs from State
	Streak      int
	StreakStart time.Time
	streakOK    bool
}

// Restore rebuilds a machine from persisted fields.
func Restore(st, pending State, streak int, streakStart time.Time) *Device {
	d := &Device{State: st, Pending: pending, Streak: streak, StreakStart: streakStart}
	switch {
	case pending == Up:
		d.streakOK = true
	case pending == Down:
		d.streakOK = false
	default:
		d.streakOK = st == Up
	}
	return d
}

// StreakOK reports the direction of the current streak.
func (d *Device) StreakOK() bool { return d.streakOK }

// Apply feeds one check and returns the confirmed transition, if any.
func (d *Device) Apply(c Check, t Thresholds) *Transition {
	if c.MonitorError {
		from := d.State
		d.State, d.Pending, d.Streak, d.StreakStart = Error, "", 0, time.Time{}
		if from != Error {
			return &Transition{From: from, To: Error, At: c.At}
		}
		return nil
	}
	if d.Streak == 0 || d.streakOK != c.OK {
		d.Streak, d.StreakStart, d.streakOK = 0, c.At, c.OK
	}
	d.Streak++
	var tr *Transition
	if c.OK && d.State != Up && d.Streak >= t.Recoveries {
		tr = &Transition{From: d.State, To: Up, At: d.StreakStart}
		d.State = Up
	} else if !c.OK && d.State != Down && d.Streak >= t.Failures {
		tr = &Transition{From: d.State, To: Down, At: d.StreakStart}
		d.State = Down
	}
	dir := Down
	if c.OK {
		dir = Up
	}
	if dir == d.State {
		d.Pending = ""
	} else {
		d.Pending = dir
	}
	return tr
}

// Expire marks the device as having no recent data.
func (d *Device) Expire() {
	d.State, d.Pending, d.Streak, d.StreakStart, d.streakOK = Unknown, "", 0, time.Time{}, false
}
