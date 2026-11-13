package stats

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/live"
	"sismon/internal/state"
	"sismon/internal/store"
)

// Segment is a contiguous state span (ms).
type Segment struct {
	Start int64       `json:"start"`
	End   int64       `json:"end"`
	State state.State `json:"state"`
}

// Timeline returns a device's state segments in [from, to], gaps as unknown.
func Timeline(ctx context.Context, st *store.Store, deviceID int64, from, to, now time.Time) ([]Segment, error) {
	return TimelineFor(ctx, st, store.EntityDevice, deviceID, from, to, now)
}

// TimelineFor returns the state segments of a device or location.
func TimelineFor(ctx context.Context, st *store.Store, e store.Entity, id int64, from, to, now time.Time) ([]Segment, error) {
	table, missing := "devices", "Echipamentul nu există."
	if e == store.EntityLocation {
		table, missing = "locations", "Locația nu există."
	}
	var exists int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, apperr.NotFound(missing)
	}
	a, b, n := store.Ms(from), store.Ms(to), store.Ms(now)
	if b > n {
		b = n
	}
	return segments(ctx, st.DB, e, id, a, b, n)
}

// interval is a stored state interval; End is nil while it is open.
type interval struct {
	State state.State
	Start int64
	End   *int64
}

// intervals returns an entity's stored intervals overlapping [a, b] (ms),
// unclipped and in order; open intervals count as ending at n.
func intervals(ctx context.Context, q store.Querier, e store.Entity, id, a, b, n int64) ([]interval, error) {
	rows, err := q.QueryContext(ctx, `SELECT state, start_at, end_at FROM state_intervals
		WHERE entity=?5 AND entity_id=?4 AND start_at < ?2 AND COALESCE(end_at,?3) > ?1 ORDER BY start_at, id`, a, b, n, id, e)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []interval
	for rows.Next() {
		var iv interval
		var end sql.NullInt64
		if err := rows.Scan(&iv.State, &iv.Start, &end); err != nil {
			return nil, err
		}
		if end.Valid {
			iv.End = &end.Int64
		}
		out = append(out, iv)
	}
	return out, rows.Err()
}

// segments returns the state segments of an entity in [a, b] (ms), gaps as
// unknown; open intervals end at n.
func segments(ctx context.Context, q store.Querier, e store.Entity, id, a, b, n int64) ([]Segment, error) {
	ivs, err := intervals(ctx, q, e, id, a, b, n)
	if err != nil {
		return nil, err
	}
	return toSegments(ivs, a, b, n), nil
}

// toSegments clips intervals to [a, b] and fills the gaps as unknown.
func toSegments(ivs []interval, a, b, n int64) []Segment {
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
	for _, iv := range ivs {
		s, e := max(iv.Start, a), n
		if iv.End != nil {
			e = *iv.End
		}
		e = min(e, b)
		add(cursor, s, state.Unknown)
		add(max(s, cursor), e, iv.State)
		cursor = max(cursor, e)
	}
	add(cursor, b, state.Unknown)
	return out
}

// Report24 is the operational 24-hour report.
type Report24 struct {
	From        int64         `json:"from"`
	To          int64         `json:"to"`
	Locations   int           `json:"locations"`
	AffectedNow int           `json:"affected_now"`
	Current     []CurrentDown `json:"current"`         // o intrare per locație, cele mai vechi căderi primele
	Incidents   []IncidentRow `json:"incidents"`       // doar echipamente primare
	Minor       []IncidentRow `json:"minor_incidents"` // echipamente secundare
}

// CurrentDown is a location that is unavailable right now: its primary
// devices that are down, with the earliest start.
type CurrentDown struct {
	LocationID int64           `json:"location_id"`
	Location   string          `json:"location"`
	Since      int64           `json:"since"`
	Devices    []CurrentDevice `json:"devices"`
}

// CurrentDevice is a primary device that is down right now.
type CurrentDevice struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ErrorCode string `json:"error_code"`
	Since     int64  `json:"since"`
}

// BuildReport24 builds the last-24-hours report from a live snapshot.
func BuildReport24(ctx context.Context, st *store.Store, snap live.Snapshot, now time.Time) (Report24, error) {
	// The last day's incidents stay in the report even if archived since.
	q := Query{From: now.Add(-24 * time.Hour), To: now, Incidents: "all", Archived: true}
	rep, err := Build(ctx, st, q, now)
	if err != nil {
		return Report24{}, err
	}
	out := Report24{From: rep.From, To: rep.To, Locations: len(snap.Locations), Current: []CurrentDown{}, Incidents: []IncidentRow{}, Minor: []IncidentRow{}}
	primary := map[int64]bool{}
	for _, d := range rep.Devices {
		primary[d.ID] = d.Primary
	}
	for _, i := range rep.Incidents {
		if primary[i.DeviceID] {
			out.Incidents = append(out.Incidents, i)
		} else {
			out.Minor = append(out.Minor, i)
		}
	}
	for _, l := range snap.Locations {
		var cur *CurrentDown
		for _, d := range l.Devices {
			if !d.Primary || d.State != state.Down {
				continue
			}
			since := d.Since
			var obs sql.NullInt64
			if err := st.DB.QueryRowContext(ctx, `SELECT observed_at FROM incidents WHERE device_id=? AND ended_at IS NULL`, d.ID).Scan(&obs); err == nil && obs.Valid {
				since = obs.Int64
			}
			if cur == nil {
				cur = &CurrentDown{LocationID: l.ID, Location: l.Name, Since: since}
			}
			cur.Since = min(cur.Since, since)
			cur.Devices = append(cur.Devices, CurrentDevice{ID: d.ID, Name: d.Name, ErrorCode: d.ErrorCode, Since: since})
		}
		if cur != nil {
			out.Current = append(out.Current, *cur)
		}
	}
	sort.SliceStable(out.Current, func(i, j int) bool { return out.Current[i].Since < out.Current[j].Since })
	out.AffectedNow = len(out.Current)
	return out, nil
}
