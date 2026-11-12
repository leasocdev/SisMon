package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"sismon/internal/state"
)

// Entity is the kind of entity a state interval belongs to.
type Entity string

const (
	EntityDevice   Entity = "device"
	EntityLocation Entity = "location"
)

// OpenState returns the state and start of the open interval, if any.
func OpenState(ctx context.Context, q Querier, e Entity, id int64) (state.State, time.Time, bool, error) {
	var st string
	var start int64
	err := q.QueryRowContext(ctx, `SELECT state,start_at FROM state_intervals WHERE entity=? AND entity_id=? AND end_at IS NULL`, e, id).Scan(&st, &start)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, wrap("open interval", err)
	}
	return state.State(st), FromMs(start), true, nil
}

// CloseAndOpen ends the open interval and starts one with state st at `at`.
// It is a no-op when the open interval already has st. Times earlier than the
// open interval's start (clock moved backwards) are clamped to that start.
func CloseAndOpen(ctx context.Context, tx *sql.Tx, e Entity, id int64, st state.State, at time.Time) error {
	cur, start, ok, err := OpenState(ctx, tx, e, id)
	if err != nil {
		return err
	}
	if ok && cur == st {
		return nil
	}
	if ok && at.Before(start) {
		at = start
	}
	if ok {
		if _, err := tx.ExecContext(ctx, `UPDATE state_intervals SET end_at=? WHERE entity=? AND entity_id=? AND end_at IS NULL`, Ms(at), e, id); err != nil {
			return wrap("close interval", err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO state_intervals(entity,entity_id,state,start_at) VALUES(?,?,?,?)`, e, id, st, Ms(at))
	return wrap("open interval", err)
}

// CloseOpen ends the open interval (if any) at `at`, clamped to its start.
func CloseOpen(ctx context.Context, tx *sql.Tx, e Entity, id int64, at time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE state_intervals SET end_at=MAX(start_at,?) WHERE entity=? AND entity_id=? AND end_at IS NULL`, Ms(at), e, id)
	return wrap("close interval", err)
}
