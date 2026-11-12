package stats

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"sismon/internal/state"
	"sismon/internal/store"
)

// Durations are seconds spent in each state.
type Durations struct {
	Up       float64 `json:"up"`
	Down     float64 `json:"down"`
	Degraded float64 `json:"degraded"`
	Unknown  float64 `json:"unknown"`
	Error    float64 `json:"error"`
}

func (d Durations) known() float64 { return d.Up + d.Down + d.Degraded }

// DeviceRow is one device in a report.
type DeviceRow struct {
	ID           int64     `json:"id"`
	LocationID   int64     `json:"location_id"`
	Location     string    `json:"location"`
	Name         string    `json:"name"`
	Host         string    `json:"host"`
	Kind         string    `json:"kind"`
	Primary      bool      `json:"is_primary"`
	Archived     bool      `json:"archived"`
	Durations    Durations `json:"durations"`
	Availability *float64  `json:"availability"`
	Coverage     *float64  `json:"coverage"`
	LatencyMs    *float64  `json:"latency_ms"`
	LossPct      *float64  `json:"loss_pct"`
	Incidents    int       `json:"incidents"`
	MTTRS        *float64  `json:"mttr_s"`
}

// LocationRow is one location in a report.
type LocationRow struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Archived     bool      `json:"archived"`
	Durations    Durations `json:"durations"`
	Availability *float64  `json:"availability"`
	Coverage     *float64  `json:"coverage"`
}

// IncidentRow is one incident in a report.
type IncidentRow struct {
	ID         int64   `json:"id"`
	DeviceID   int64   `json:"device_id"`
	LocationID int64   `json:"location_id"`
	Location   string  `json:"location"`
	Device     string  `json:"device"`
	ObservedAt int64   `json:"observed_at"`
	EndedAt    *int64  `json:"ended_at"`
	Reason     string  `json:"reason"`
	ReasonText string  `json:"reason_text"`
	DurationS  float64 `json:"duration_s"`
}

// Report is the statistics result.
type Report struct {
	From      int64         `json:"from"`
	To        int64         `json:"to"`
	Devices   []DeviceRow   `json:"devices"`
	Locations []LocationRow `json:"locations"`
	Incidents []IncidentRow `json:"incidents"`
}

func pct(num, den float64) *float64 {
	if den <= 0 {
		return nil
	}
	v := 100 * num / den
	return &v
}

type window struct{ a, b int64 } // ms

func eligible(q Query, now time.Time, created int64, archived sql.NullInt64) (window, bool) {
	a, b := store.Ms(q.From), store.Ms(q.To)
	if n := store.Ms(now); b > n {
		b = n
	}
	if created > a {
		a = created
	}
	if archived.Valid && archived.Int64 < b {
		b = archived.Int64
	}
	return window{a, b}, b > a
}

// durations sums the overlap of an entity's intervals with w; open intervals
// end at now. Uncovered time counts as unknown.
func durations(ctx context.Context, q store.Querier, e store.Entity, id int64, w window, now int64) (Durations, error) {
	rows, err := q.QueryContext(ctx, `SELECT state, SUM(MIN(COALESCE(end_at,?2),?4) - MAX(start_at,?3)) FROM state_intervals
		WHERE entity=?5 AND entity_id=?6 AND start_at < ?4 AND COALESCE(end_at,?2) > ?3 GROUP BY state`, nil, now, w.a, w.b, e, id)
	if err != nil {
		return Durations{}, err
	}
	defer rows.Close()
	var d Durations
	var covered float64
	for rows.Next() {
		var st string
		var ms float64
		if err := rows.Scan(&st, &ms); err != nil {
			return d, err
		}
		s := ms / 1000
		covered += s
		switch state.State(st) {
		case state.Up:
			d.Up += s
		case state.Down:
			d.Down += s
		case state.Degraded:
			d.Degraded += s
		case state.Error:
			d.Error += s
		default:
			d.Unknown += s
		}
	}
	if gap := float64(w.b-w.a)/1000 - covered; gap > 0 {
		d.Unknown += gap
	}
	return d, rows.Err()
}

// latencyAvg combines raw samples (recent) with hourly aggregates (older).
func latencyAvg(ctx context.Context, q store.Querier, id int64, w window) (lat, loss *float64, err error) {
	var latSum, latN, lossSum, n sql.NullFloat64
	err = q.QueryRowContext(ctx, `SELECT SUM(ls), SUM(ln), SUM(xs), SUM(xn) FROM (
		SELECT SUM(latency_us)/1000.0 ls, COUNT(latency_us) ln, SUM(loss_pct) xs, COUNT(*) xn FROM samples WHERE device_id=?1 AND ts>=?2 AND ts<?3
		UNION ALL
		SELECT SUM(lat_avg*ok_checks), SUM(CASE WHEN lat_avg IS NULL THEN 0 ELSE ok_checks END), SUM(loss_avg*checks), SUM(checks)
		FROM samples_hourly WHERE device_id=?1 AND hour_ts>=?2 AND hour_ts<?3
		  AND hour_ts < COALESCE((SELECT MIN(ts) FROM samples WHERE device_id=?1 AND ts>=?2), ?3))`, id, w.a, w.b).Scan(&latSum, &latN, &lossSum, &n)
	if err != nil {
		return nil, nil, err
	}
	if latN.Valid && latN.Float64 > 0 {
		v := latSum.Float64 / latN.Float64
		lat = &v
	}
	if n.Valid && n.Float64 > 0 {
		v := lossSum.Float64 / n.Float64
		loss = &v
	}
	return lat, loss, nil
}

// Build computes a report.
func Build(ctx context.Context, st *store.Store, q Query, now time.Time) (Report, error) {
	if q.Incidents == "" {
		q.Incidents = "all"
	}
	tx, err := st.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback()
	nowMs := store.Ms(now)
	rep := Report{From: store.Ms(q.From), To: min(store.Ms(q.To), nowMs), Devices: []DeviceRow{}, Locations: []LocationRow{}, Incidents: []IncidentRow{}}

	where, args := []string{"1=1"}, []any{}
	if q.LocationID != nil {
		where, args = append(where, "d.location_id=?"), append(args, *q.LocationID)
	}
	if q.DeviceID != nil {
		where, args = append(where, "d.id=?"), append(args, *q.DeviceID)
	}
	if q.Kind != "" {
		where, args = append(where, "d.kind=?"), append(args, q.Kind)
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.id,d.location_id,l.name,d.name,d.host,d.kind,d.is_primary,d.created_at,d.archived_at,
		l.created_at,l.archived_at FROM devices d JOIN locations l ON l.id=d.location_id WHERE `+strings.Join(where, " AND ")+
		` ORDER BY l.name COLLATE NOCASE, l.id, d.sort_order, d.id`, args...)
	if err != nil {
		return rep, err
	}
	type devMeta struct {
		row DeviceRow
		w   window
	}
	type locMeta struct {
		name     string
		created  int64
		archived sql.NullInt64
	}
	var devs []devMeta
	locs := map[int64]locMeta{}
	var locOrder []int64
	for rows.Next() {
		var r DeviceRow
		var created, lcreated int64
		var archived, larchived sql.NullInt64
		if err := rows.Scan(&r.ID, &r.LocationID, &r.Location, &r.Name, &r.Host, &r.Kind, &r.Primary, &created, &archived, &lcreated, &larchived); err != nil {
			rows.Close()
			return rep, err
		}
		w, ok := eligible(q, now, created, archived)
		if !ok {
			continue
		}
		r.Archived = archived.Valid
		devs = append(devs, devMeta{r, w})
		if _, seen := locs[r.LocationID]; !seen {
			locs[r.LocationID] = locMeta{r.Location, lcreated, larchived}
			locOrder = append(locOrder, r.LocationID)
		}
	}
	rows.Close()

	byDevice := map[int64]int{}
	for i := range devs {
		dm := &devs[i]
		d, err := durations(ctx, tx, store.EntityDevice, dm.row.ID, dm.w, nowMs)
		if err != nil {
			return rep, err
		}
		dm.row.Durations = d
		dm.row.Availability = pct(d.Up, d.Up+d.Down)
		elig := float64(dm.w.b-dm.w.a) / 1000
		dm.row.Coverage = pct(d.known(), elig)
		if dm.row.LatencyMs, dm.row.LossPct, err = latencyAvg(ctx, tx, dm.row.ID, dm.w); err != nil {
			return rep, err
		}

		inc, err := tx.QueryContext(ctx, `SELECT id,observed_at,ended_at,COALESCE(end_reason,'') FROM incidents
			WHERE device_id=? AND observed_at<? AND (ended_at IS NULL OR ended_at>?) ORDER BY observed_at DESC`, dm.row.ID, dm.w.b, dm.w.a)
		if err != nil {
			return rep, err
		}
		var devMttr float64
		var devMttrN int
		for inc.Next() {
			var r IncidentRow
			var ended sql.NullInt64
			if err := inc.Scan(&r.ID, &r.ObservedAt, &ended, &r.Reason); err != nil {
				inc.Close()
				return rep, err
			}
			if ended.Valid {
				r.EndedAt = &ended.Int64
			}
			if r.Reason == "recovered" && ended.Valid && ended.Int64 <= dm.w.b {
				full := float64(ended.Int64-r.ObservedAt) / 1000
				devMttr, devMttrN = devMttr+full, devMttrN+1
			}
			if (q.Incidents == "active" && ended.Valid) || (q.Incidents == "closed" && !ended.Valid) {
				continue
			}
			end := dm.w.b
			if ended.Valid && ended.Int64 < end {
				end = ended.Int64
			}
			r.DurationS = float64(end-max(r.ObservedAt, dm.w.a)) / 1000
			if r.DurationS < 0 {
				r.DurationS = 0
			}
			r.DeviceID, r.LocationID, r.Location, r.Device, r.ReasonText = dm.row.ID, dm.row.LocationID, dm.row.Location, dm.row.Name, ReasonText(r.Reason)
			dm.row.Incidents++
			rep.Incidents = append(rep.Incidents, r)
		}
		inc.Close()
		if devMttrN > 0 {
			v := devMttr / float64(devMttrN)
			dm.row.MTTRS = &v
		}
		byDevice[dm.row.ID] = i
		rep.Devices = append(rep.Devices, dm.row)
	}
	for _, lid := range locOrder {
		lm := locs[lid]
		w, ok := eligible(q, now, lm.created, lm.archived)
		row := LocationRow{ID: lid, Name: lm.name, Archived: lm.archived.Valid}
		if ok {
			d, err := durations(ctx, tx, store.EntityLocation, lid, w, nowMs)
			if err != nil {
				return rep, err
			}
			row.Durations = d
			row.Availability = pct(d.Up+d.Degraded, d.Up+d.Degraded+d.Down) // „Degradată" = doar echipamente secundare căzute; locația funcționează
			row.Coverage = pct(d.known(), float64(w.b-w.a)/1000)
		}
		rep.Locations = append(rep.Locations, row)
	}
	sort.SliceStable(rep.Incidents, func(i, j int) bool { return rep.Incidents[i].ObservedAt > rep.Incidents[j].ObservedAt })
	return rep, nil
}
