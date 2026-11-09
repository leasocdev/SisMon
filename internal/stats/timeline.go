package stats

import (
	"context"
	"database/sql"
	"time"

	"netwatch/internal/apperr"
	"netwatch/internal/live"
	"netwatch/internal/state"
	"netwatch/internal/store"
)

// Segment is a contiguous state span (ms).
type Segment struct {
	Start int64       `json:"start"`
	End   int64       `json:"end"`
	State state.State `json:"state"`
}

// Timeline returns a device's state segments in [from, to], gaps as unknown.
func Timeline(ctx context.Context, st *store.Store, deviceID int64, from, to, now time.Time) ([]Segment, error) {
	var exists int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE id=?`, deviceID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, apperr.NotFound("Echipamentul nu există.")
	}
	a, b, n := store.Ms(from), store.Ms(to), store.Ms(now)
	if b > n {
		b = n
	}
	rows, err := st.DB.QueryContext(ctx, `SELECT state, MAX(start_at,?1), MIN(COALESCE(end_at,?3),?2) FROM state_intervals
		WHERE entity='device' AND entity_id=?4 AND start_at < ?2 AND COALESCE(end_at,?3) > ?1 ORDER BY start_at, id`, a, b, n, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Segment{}
	add := func(s, e int64, stt state.State) {
		if e <= s {
			return
		}
		if k := len(out); k > 0 && out[k-1].State == stt && out[k-1].End == s {
			out[k-1].End = e
			return
		}
		out = append(out, Segment{s, e, stt})
	}
	cursor := a
	for rows.Next() {
		var stt string
		var s, e int64
		if err := rows.Scan(&stt, &s, &e); err != nil {
			return nil, err
		}
		add(cursor, s, state.Unknown)
		add(max(s, cursor), e, state.State(stt))
		cursor = max(cursor, e)
	}
	add(cursor, b, state.Unknown)
	return out, rows.Err()
}

// Report24 is the operational 24-hour report.
type Report24 struct {
	From        int64         `json:"from"`
	To          int64         `json:"to"`
	Locations   int           `json:"locations"`
	AffectedNow int           `json:"affected_now"`
	Current     []CurrentDown `json:"current"`
	Incidents   []IncidentRow `json:"incidents"`
}

// CurrentDown is a primary device that is down right now.
type CurrentDown struct {
	LocationID int64  `json:"location_id"`
	Location   string `json:"location"`
	Device     string `json:"device"`
	Since      int64  `json:"since"`
}

// BuildReport24 builds the last-24-hours report from a live snapshot.
func BuildReport24(ctx context.Context, st *store.Store, snap live.Snapshot, now time.Time) (Report24, error) {
	q := Query{From: now.Add(-24 * time.Hour), To: now, Incidents: "all"}
	rep, err := Build(ctx, st, q, now)
	if err != nil {
		return Report24{}, err
	}
	out := Report24{From: rep.From, To: rep.To, Locations: len(snap.Locations), Current: []CurrentDown{}, Incidents: rep.Incidents}
	affected := map[int64]bool{}
	for _, l := range snap.Locations {
		for _, d := range l.Devices {
			if !d.Primary || d.State != state.Down {
				continue
			}
			since := d.Since
			var obs sql.NullInt64
			if err := st.DB.QueryRowContext(ctx, `SELECT observed_at FROM incidents WHERE device_id=? AND ended_at IS NULL`, d.ID).Scan(&obs); err == nil && obs.Valid {
				since = obs.Int64
			}
			out.Current = append(out.Current, CurrentDown{LocationID: l.ID, Location: l.Name, Device: d.Name, Since: since})
			affected[l.ID] = true
		}
	}
	out.AffectedNow = len(affected)
	return out, nil
}
