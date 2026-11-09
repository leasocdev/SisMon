// Package store owns the SQLite database: schema, migrations and typed queries.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SchemaVersion is the schema version this build understands.
const SchemaVersion = 1

var (
	ErrSchemaTooNew = errors.New("baza de date are o versiune de schemă mai nouă decât această aplicație")
	ErrNotFound     = errors.New("nu a fost găsit")
)

// Options configure Open.
type Options struct {
	// BeforeMigrate runs before migrations are applied to a non-empty database.
	BeforeMigrate func(s *Store, from, to int) error
}

// Store wraps the database handle.
type Store struct {
	DB   *sql.DB
	Path string
}

// Querier is implemented by *sql.DB and *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DSN returns the driver connection string with the required pragmas.
func DSN(path string) string {
	q := url.Values{}
	for _, p := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "foreign_keys(1)", "synchronous(NORMAL)"} {
		q.Add("_pragma", p)
	}
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}

// Open opens (creating if needed) and migrates the database at path.
func Open(ctx context.Context, path string, opt Options) (*Store, error) {
	db, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db, Path: path}
	if err := s.migrate(ctx, opt); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }

// VacuumInto writes a consistent compacted copy of the database to dest.
func (s *Store) VacuumInto(ctx context.Context, dest string) error {
	_, err := s.DB.ExecContext(ctx, `VACUUM INTO ?`, dest)
	return wrap("vacuum into", err)
}

// Ms converts a time to Unix milliseconds.
func Ms(t time.Time) int64 { return t.UnixMilli() }

// FromMs converts Unix milliseconds to a time.
func FromMs(ms int64) time.Time { return time.UnixMilli(ms) }

func timePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := FromMs(v.Int64)
	return &t
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}
