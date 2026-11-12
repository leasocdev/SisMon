package store

import (
	"context"
	"database/sql"

	"sismon/internal/state"
)

// Status is the persisted current status of a device.
type Status struct {
	DeviceID    int64
	State       state.State
	Pending     state.State
	Streak      int
	StreakStart *int64
	Since       int64
	LastCheck   *int64
	LatencyMs   *float64
	LossPct     *float64
	ErrorCode   string
	DNSWarning  bool
}

func i64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func f64(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}

// DeviceStatuses returns the status of every device, keyed by device id.
func DeviceStatuses(ctx context.Context, q Querier) (map[int64]Status, error) {
	rows, err := q.QueryContext(ctx, `SELECT device_id,state,pending,streak,streak_start,COALESCE(since,0),last_check_at,latency_ms,loss_pct,error_code,dns_warning FROM device_status`)
	if err != nil {
		return nil, wrap("device status", err)
	}
	defer rows.Close()
	out := map[int64]Status{}
	for rows.Next() {
		var s Status
		var st, pend string
		var ss, lc sql.NullInt64
		var lat, loss sql.NullFloat64
		if err := rows.Scan(&s.DeviceID, &st, &pend, &s.Streak, &ss, &s.Since, &lc, &lat, &loss, &s.ErrorCode, &s.DNSWarning); err != nil {
			return nil, err
		}
		s.State, s.Pending, s.StreakStart, s.LastCheck, s.LatencyMs, s.LossPct = state.State(st), state.State(pend), i64(ss), i64(lc), f64(lat), f64(loss)
		out[s.DeviceID] = s
	}
	return out, rows.Err()
}

// PutStatus upserts a device status.
func PutStatus(ctx context.Context, q Querier, s Status) error {
	_, err := q.ExecContext(ctx, `INSERT OR REPLACE INTO device_status(device_id,state,pending,streak,streak_start,since,last_check_at,latency_ms,loss_pct,error_code,dns_warning)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, s.DeviceID, s.State, s.Pending, s.Streak, s.StreakStart, s.Since, s.LastCheck, s.LatencyMs, s.LossPct, s.ErrorCode, s.DNSWarning)
	return wrap("put status", err)
}
