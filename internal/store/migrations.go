package store

import (
	"context"
	_ "embed"
	"fmt"
)

//go:embed schema_v1.sql
var schemaV1 string

// migrations[i] upgrades the schema from version i to i+1.
var migrations = []string{schemaV1}

func (s *Store) migrate(ctx context.Context, opt Options) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return wrap("meta", err)
	}
	var current int
	if _, err := s.GetMeta(ctx, "schema_version", &current); err != nil {
		return err
	}
	if current > SchemaVersion {
		return fmt.Errorf("%w (%d > %d)", ErrSchemaTooNew, current, SchemaVersion)
	}
	if current == SchemaVersion {
		return nil
	}
	var tables int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name<>'meta'`).Scan(&tables); err != nil {
		return err
	}
	if tables > 0 && opt.BeforeMigrate != nil {
		if err := opt.BeforeMigrate(s, current, SchemaVersion); err != nil {
			return fmt.Errorf("backup înainte de migrare: %w", err)
		}
	}
	for v := current; v < SchemaVersion; v++ {
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrarea %d→%d: %w", v, v+1, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta(key,value) VALUES('schema_version',?)`, fmt.Sprint(v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
