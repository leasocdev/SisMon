package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/settings"
	"sismon/internal/store"
)

// Zone is the local time zone for backup names and schedules.
var Zone = mustZone("Europe/Chisinau")

func mustZone(name string) *time.Location {
	z, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return z
}

var nameRe = regexp.MustCompile(`^sismon-(\d{8}-\d{6})(?:-(\d+))?-(auto|manual|pre-migrate|pre-restore)\.db$`)

// BackupService creates, lists and opens backups.
type BackupService struct {
	Dir     string
	st      *store.Store
	set     *settings.Holder
	mu      sync.Mutex
	warning string
}

// NewBackupService stores backups in <dataDir>/backups.
func NewBackupService(st *store.Store, dataDir string, set *settings.Holder) *BackupService {
	return &BackupService{Dir: filepath.Join(dataDir, "backups"), st: st, set: set}
}

// Warning returns the warning from the last Create (e.g. secondary copy failed).
func (b *BackupService) Warning() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.warning
}

// Create writes a consistent, session-free, integrity-checked backup.
func (b *BackupService) Create(ctx context.Context, kind string) (BackupInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.warning = ""
	if err := os.MkdirAll(b.Dir, 0o750); err != nil {
		return BackupInfo{}, fmt.Errorf("directorul de backup: %w", err)
	}
	stamp := time.Now().In(Zone).Format("20060102-150405")
	name := fmt.Sprintf("sismon-%s-%s.db", stamp, kind)
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(b.Dir, name)); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("sismon-%s-%d-%s.db", stamp, i, kind)
	}
	final := filepath.Join(b.Dir, name)
	tmp := final + ".tmp"
	os.Remove(tmp)
	if err := b.st.VacuumInto(ctx, tmp); err != nil {
		return BackupInfo{}, err
	}
	if err := scrub(ctx, tmp); err != nil {
		os.Remove(tmp)
		return BackupInfo{}, err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return BackupInfo{}, err
	}
	info, err := describe(final)
	if err != nil {
		return BackupInfo{}, err
	}
	if sec := b.set.Get().BackupSecondaryDir; sec != "" {
		if err := copyFile(final, filepath.Join(sec, name)); err != nil {
			b.warning = "Copia în directorul secundar a eșuat: " + err.Error()
		}
	}
	return info, nil
}

// scrub removes sessions from a backup copy and verifies its integrity.
func scrub(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", store.DSN(path))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=DELETE`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `VACUUM`); err != nil {
		return err
	}
	var res string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("verificarea integrității backup-ului: %s", res)
	}
	return nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func describe(path string) (BackupInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return BackupInfo{}, err
	}
	m := nameRe.FindStringSubmatch(fi.Name())
	if m == nil {
		return BackupInfo{}, fmt.Errorf("nume de backup necunoscut: %s", fi.Name())
	}
	created := fi.ModTime()
	if t, err := time.ParseInLocation("20060102-150405", m[1], Zone); err == nil {
		created = t
	}
	return BackupInfo{Name: fi.Name(), Kind: m[3], Created: created.UnixMilli(), SizeBytes: fi.Size()}, nil
}

// List returns backups, newest first.
func (b *BackupService) List() ([]BackupInfo, error) {
	entries, err := os.ReadDir(b.Dir)
	if os.IsNotExist(err) {
		return []BackupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []BackupInfo{}
	for _, e := range entries {
		if e.IsDir() || !nameRe.MatchString(e.Name()) {
			continue
		}
		if info, err := describe(filepath.Join(b.Dir, e.Name())); err == nil {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Created > out[j].Created || out[i].Created == out[j].Created && out[i].Name > out[j].Name
	})
	return out, nil
}

// Open opens a backup by name for download.
func (b *BackupService) Open(name string) (io.ReadSeekCloser, BackupInfo, error) {
	if !nameRe.MatchString(name) {
		return nil, BackupInfo{}, apperr.Invalid("name", "Nume de backup invalid.")
	}
	p := filepath.Join(b.Dir, name)
	info, err := describe(p)
	if err != nil {
		return nil, BackupInfo{}, apperr.NotFound("Backup-ul nu există.")
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, BackupInfo{}, apperr.NotFound("Backup-ul nu există.")
	}
	return f, info, nil
}

// Remove deletes backups (used by rotation).
func (b *BackupService) Remove(list []BackupInfo) error {
	for _, i := range list {
		if !nameRe.MatchString(i.Name) {
			continue
		}
		if err := os.Remove(filepath.Join(b.Dir, i.Name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// listDir lists backups in dir, newest first.
func listDir(dir string) ([]BackupInfo, error) {
	b := &BackupService{Dir: dir}
	return b.List()
}

// RotateAll applies the 7/4/12 rotation to the primary and secondary directories.
func (b *BackupService) RotateAll(now time.Time) error {
	dirs := []string{b.Dir}
	if sec := b.set.Get().BackupSecondaryDir; sec != "" {
		dirs = append(dirs, sec)
	}
	for _, d := range dirs {
		list, err := listDir(d)
		if err != nil {
			return err
		}
		if err := (&BackupService{Dir: d}).Remove(Rotate(list, now)); err != nil {
			return err
		}
	}
	return nil
}
