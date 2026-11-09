package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// GetMeta decodes the JSON value stored under key into v.
func (s *Store) GetMeta(ctx context.Context, key string, v any) (bool, error) {
	return GetMetaQ(ctx, s.DB, key, v)
}

// SetMeta stores v as JSON under key.
func (s *Store) SetMeta(ctx context.Context, key string, v any) error {
	return SetMetaQ(ctx, s.DB, key, v)
}

// GetMetaQ is GetMeta on any Querier.
func GetMetaQ(ctx context.Context, q Querier, key string, v any) (bool, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, wrap("meta "+key, err)
	}
	return true, wrap("meta "+key, json.Unmarshal([]byte(raw), v))
}

// SetMetaQ is SetMeta on any Querier.
func SetMetaQ(ctx context.Context, q Querier, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT OR REPLACE INTO meta(key,value) VALUES(?,?)`, key, string(b))
	return wrap("meta "+key, err)
}
