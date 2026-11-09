package state

// Member is one active device of a location, as seen by the location rules.
type Member struct {
	Primary bool
	State   State
}

// LocationResult is the aggregated location state.
type LocationResult struct {
	State      State
	Incomplete bool
}

// Location applies the spec §4 rules. Error counts as Unknown.
func Location(members []Member) LocationResult {
	incomplete := false
	hasPrimary := false
	primaryDown, primaryNotUp, secDown, secNotUp := false, false, false, false
	for _, m := range members {
		s := m.State
		if s == Error {
			s = Unknown
		}
		if s != Up && s != Down {
			incomplete = true
		}
		if m.Primary {
			hasPrimary = true
			if s == Down {
				primaryDown = true
			} else if s != Up {
				primaryNotUp = true
			}
		} else {
			if s == Down {
				secDown = true
			} else if s != Up {
				secNotUp = true
			}
		}
	}
	if !hasPrimary {
		return LocationResult{State: Unknown, Incomplete: true}
	}
	switch {
	case primaryDown:
		return LocationResult{Down, incomplete}
	case primaryNotUp:
		return LocationResult{Unknown, incomplete}
	case secDown:
		return LocationResult{Degraded, incomplete}
	case secNotUp:
		return LocationResult{Unknown, incomplete}
	}
	return LocationResult{Up, incomplete}
}
