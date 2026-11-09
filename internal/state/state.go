// Package state holds the pure rules for device and location states:
// hysteresis, retroactive attribution and location aggregation.
package state

import "time"

// State is the state of a device or location.
type State string

const (
	Up       State = "up"
	Down     State = "down"
	Degraded State = "degraded"
	Unknown  State = "unknown"
	Error    State = "error"
)

// Valid reports whether s is one of the known states.
func (s State) Valid() bool {
	switch s {
	case Up, Down, Degraded, Unknown, Error:
		return true
	}
	return false
}

// Thresholds are the consecutive check counts needed to confirm a change.
type Thresholds struct{ Failures, Recoveries int }

// Check is the outcome of one device verification.
type Check struct {
	At           time.Time
	OK           bool
	MonitorError bool
}

// Transition describes a confirmed state change. At is the effective
// (retroactive) moment; ConfirmedAt is when the change was confirmed.
type Transition struct {
	From, To    State
	At          time.Time
	ConfirmedAt time.Time
}
