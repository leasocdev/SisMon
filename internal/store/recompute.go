package store

import (
	"context"
	"database/sql"
	"time"

	"sismon/internal/state"
)

// LocationMembers returns the active, enabled devices of a location as state members.
func LocationMembers(ctx context.Context, q Querier, locationID int64) ([]state.Member, error) {
	rows, err := q.QueryContext(ctx, `SELECT d.is_primary, COALESCE(s.state,'unknown') FROM devices d
		LEFT JOIN device_status s ON s.device_id=d.id WHERE d.location_id=? AND d.archived_at IS NULL AND d.disabled_at IS NULL`, locationID)
	if err != nil {
		return nil, wrap("location members", err)
	}
	defer rows.Close()
	var out []state.Member
	for rows.Next() {
		var m state.Member
		var st string
		if err := rows.Scan(&m.Primary, &st); err != nil {
			return nil, err
		}
		m.State = state.State(st)
		out = append(out, m)
	}
	return out, rows.Err()
}

// RecomputeLocation derives the location state from its devices' current
// status and records it in state_intervals at `at` when it changed.
func RecomputeLocation(ctx context.Context, tx *sql.Tx, locationID int64, at time.Time) (state.LocationResult, error) {
	ms, err := LocationMembers(ctx, tx, locationID)
	if err != nil {
		return state.LocationResult{}, err
	}
	r := state.Location(ms)
	return r, CloseAndOpen(ctx, tx, EntityLocation, locationID, r.State, at)
}
