package stats

import (
	"context"
	"database/sql"
	"time"

	"netwatch/internal/store"
)

// Point is one bucket of the latency chart (T = bucket start, ms).
type Point struct {
	T       int64    `json:"t"`
	Avg     *float64 `json:"avg"`
	Min     *float64 `json:"min"`
	Max     *float64 `json:"max"`
	LossPct *float64 `json:"loss_pct"`
}

const maxPoints = 500

// Latency returns at most ~500 points: raw samples up to 48 h, 15-minute
// buckets up to 7 days, hourly aggregates beyond.
func Latency(ctx context.Context, st *store.Store, deviceID int64, from, to time.Time) ([]Point, error) {
	span := to.Sub(from)
	a, b := store.Ms(from), store.Ms(to)
	var rows *sql.Rows
	var err error
	switch {
	case span <= 7*24*time.Hour:
		w := 15 * time.Minute
		if span <= 48*time.Hour {
			w = max(span/maxPoints, time.Second)
		}
		wm := w.Milliseconds()
		rows, err = st.DB.QueryContext(ctx, `SELECT ?2 + ((ts-?2)/?1)*?1 AS b, AVG(latency_us)/1000.0, MIN(latency_us)/1000.0, MAX(latency_us)/1000.0, AVG(loss_pct)
			FROM samples WHERE device_id=?4 AND ts>=?2 AND ts<?3 GROUP BY (ts-?2)/?1 ORDER BY b`, wm, a, b, deviceID)
	default:
		w := max(span/maxPoints, time.Hour)
		w = (w + time.Hour - 1) / time.Hour * time.Hour
		wm := w.Milliseconds()
		rows, err = st.DB.QueryContext(ctx, `SELECT ?2 + ((hour_ts-?2)/?1)*?1 AS b,
			SUM(lat_avg*ok_checks)/NULLIF(SUM(CASE WHEN lat_avg IS NULL THEN 0 ELSE ok_checks END),0), MIN(lat_min), MAX(lat_max),
			SUM(loss_avg*checks)/NULLIF(SUM(checks),0)
			FROM samples_hourly WHERE device_id=?4 AND hour_ts>=?2 AND hour_ts<?3 GROUP BY (hour_ts-?2)/?1 ORDER BY b`, wm, a, b, deviceID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Point{}
	for rows.Next() {
		var p Point
		var avg, mn, mx, loss sql.NullFloat64
		if err := rows.Scan(&p.T, &avg, &mn, &mx, &loss); err != nil {
			return nil, err
		}
		p.Avg, p.Min, p.Max, p.LossPct = nf(avg), nf(mn), nf(mx), nf(loss)
		out = append(out, p)
	}
	return out, rows.Err()
}

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}
