// Package restore verifies and restores database backups.
package restore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sismon/internal/app"
	"sismon/internal/jobs"
	"sismon/internal/settings"
	"sismon/internal/store"
)

// ErrInvalid is returned for files that are not SisMon databases.
var ErrInvalid = errors.New("Fișierul nu este o bază SisMon validă.")

var required = []string{"meta", "locations", "devices", "device_status", "state_intervals", "samples", "samples_hourly", "incidents", "audit", "sessions"}

// Verify checks integrity, schema version and required tables (read-only).
func Verify(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("fișierul de backup nu poate fi citit: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return ErrInvalid
	}
	defer db.Close()
	var res string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&res); err != nil || res != "ok" {
		return ErrInvalid
	}
	for _, t := range required {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, t).Scan(&n); err != nil || n != 1 {
			return ErrInvalid
		}
	}
	var v int
	if err := db.QueryRow(`SELECT CAST(value AS INTEGER) FROM meta WHERE key='schema_version'`).Scan(&v); err != nil {
		return ErrInvalid
	}
	if v > store.SchemaVersion {
		return fmt.Errorf("backup-ul are schema %d, mai nouă decât această versiune (%d); actualizați aplicația", v, store.SchemaVersion)
	}
	return nil
}

// Restore replaces the database in dataDir with src. The caller must hold
// the data directory lock. The current database is backed up first.
func Restore(ctx context.Context, dataDir, src string) (jobs.BackupInfo, error) {
	if err := Verify(src); err != nil {
		return jobs.BackupInfo{}, err
	}
	dbPath := filepath.Join(dataDir, "sismon.db")
	cur, err := app.OpenStore(ctx, dataDir)
	if err != nil {
		return jobs.BackupInfo{}, err
	}
	pre, err := jobs.NewBackupService(cur, dataDir, settings.NewHolder(settings.Defaults())).Create(ctx, "pre-restore")
	cur.Close()
	if err != nil {
		return jobs.BackupInfo{}, fmt.Errorf("backup-ul bazei curente: %w", err)
	}
	tmp := dbPath + ".restore.tmp"
	os.Remove(tmp)
	sdb, err := sql.Open("sqlite", "file:"+filepath.ToSlash(src)+"?mode=ro")
	if err != nil {
		return pre, err
	}
	_, err = sdb.ExecContext(ctx, `VACUUM INTO ?`, tmp)
	sdb.Close()
	if err != nil {
		os.Remove(tmp)
		return pre, fmt.Errorf("copierea backup-ului: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		os.Remove(dbPath + suffix)
	}
	if err := os.Rename(tmp, dbPath); err != nil {
		os.Remove(tmp)
		return pre, err
	}
	st, err := app.OpenStore(ctx, dataDir)
	if err != nil {
		return pre, err
	}
	defer st.Close()
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return pre, err
	}
	return pre, nil
}
