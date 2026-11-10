package jobs

import (
	"context"
	"time"

	"netwatch/internal/store"
)

// AggregateHour writes samples_hourly for the UTC hour containing `hour`.
// It is idempotent.
func AggregateHour(ctx context.Context, st *store.Store, hour time.Time) error {
	h := hour.UTC().Truncate(time.Hour)
	_, err := st.DB.ExecContext(ctx, `INSERT OR REPLACE INTO samples_hourly(device_id,hour_ts,checks,ok_checks,lat_avg,lat_min,lat_max,loss_avg)
		SELECT device_id, ?1, COUNT(*), SUM(ok), AVG(latency_us)/1000.0, MIN(latency_us)/1000.0, MAX(latency_us)/1000.0, AVG(loss_pct)
		FROM samples WHERE ts>=?1 AND ts<?2 GROUP BY device_id`, store.Ms(h), store.Ms(h.Add(time.Hour)))
	return err
}

// Retention deletes raw samples older than keep whose hour is aggregated,
// in batches. It returns the number of deleted rows.
func Retention(ctx context.Context, st *store.Store, now time.Time, keep time.Duration) (int64, error) {
	cut := store.Ms(now.Add(-keep))
	var total int64
	for {
		res, err := st.DB.ExecContext(ctx, `DELETE FROM samples WHERE (device_id, ts) IN (
			SELECT s.device_id, s.ts FROM samples s WHERE s.ts < ? AND EXISTS (
				SELECT 1 FROM samples_hourly h WHERE h.device_id=s.device_id AND h.hour_ts = s.ts - (s.ts % 3600000))
			LIMIT 10000)`, cut)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < 10000 {
			return total, nil
		}
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
}

// Optimize runs SQLite maintenance.
func Optimize(ctx context.Context, st *store.Store) error {
	if _, err := st.DB.ExecContext(ctx, `PRAGMA optimize`); err != nil {
		return err
	}
	_, err := st.DB.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}
