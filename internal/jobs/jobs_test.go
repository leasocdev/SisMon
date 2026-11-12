package jobs

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sismon/internal/auth"
	"sismon/internal/domain"
	"sismon/internal/settings"
	"sismon/internal/store"
)

func setup(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "sismon.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	in := domain.LocationInput{Name: "Centru", Devices: []domain.DeviceInput{{Name: "R", Host: "10.0.0.1", Primary: true}}}
	in.Normalize(settings.Defaults())
	if _, err := st.SaveLocation(context.Background(), nil, in, time.Now()); err != nil {
		t.Fatal(err)
	}
	return st, dir
}

func count(t *testing.T, path, q string) int {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBackupCreateExcludesSessions(t *testing.T) {
	st, dir := setup(t)
	auth.NewSessions(st).Create(context.Background(), time.Now())
	b := NewBackupService(st, dir, settings.NewHolder(settings.Defaults()))
	info, err := b.Create(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(info.Name, "-manual.db") || info.SizeBytes == 0 {
		t.Fatalf("%+v", info)
	}
	p := filepath.Join(b.Dir, info.Name)
	if count(t, p, `SELECT COUNT(*) FROM sessions`) != 0 || count(t, p, `SELECT COUNT(*) FROM locations`) != 1 {
		t.Fatal("backup content")
	}
	list, _ := b.List()
	if len(list) != 1 || list[0].Name != info.Name || list[0].Kind != "manual" {
		t.Fatalf("%+v", list)
	}
	f, got, err := b.Open(info.Name)
	if err != nil || got.Name != info.Name {
		t.Fatal(err)
	}
	f.Close()
	if _, _, err := b.Open(`..\sismon.db`); err == nil {
		t.Fatal("traversal accepted")
	}
	info2, err := b.Create(context.Background(), "manual")
	if err != nil || info2.Name == info.Name {
		t.Fatal("same-second backup must get a distinct name", err)
	}
}

func TestBackupCreatesNestedDir(t *testing.T) {
	st, dir := setup(t)
	b := NewBackupService(st, dir, settings.NewHolder(settings.Defaults()))
	b.Dir = filepath.Join(dir, "a", "b", "c")
	if _, err := b.Create(context.Background(), "auto"); err != nil {
		t.Fatal(err)
	}
}

func TestBackupSecondaryFailureIsWarning(t *testing.T) {
	st, dir := setup(t)
	s := settings.Defaults()
	blocker := filepath.Join(dir, "file")
	os.WriteFile(blocker, []byte("x"), 0o600)
	s.BackupSecondaryDir = filepath.Join(blocker, "sub") // parent is a file → cannot create
	b := NewBackupService(st, dir, settings.NewHolder(s))
	if _, err := b.Create(context.Background(), "auto"); err != nil {
		t.Fatal(err)
	}
	if b.Warning() == "" {
		t.Fatal("expected warning")
	}
	s.BackupSecondaryDir = filepath.Join(dir, "second")
	b2 := NewBackupService(st, dir, settings.NewHolder(s))
	info, _ := b2.Create(context.Background(), "auto")
	if _, err := os.Stat(filepath.Join(dir, "second", info.Name)); err != nil || b2.Warning() != "" {
		t.Fatal("secondary copy missing", err)
	}
}

func TestRotate(t *testing.T) {
	now := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	var all []BackupInfo
	for i := 0; i < 400; i++ {
		ts := now.AddDate(0, 0, -i)
		all = append(all, BackupInfo{Name: "a" + ts.Format("20060102"), Kind: "auto", Created: ts.UnixMilli()})
	}
	all = append(all, BackupInfo{Name: "m1", Kind: "manual", Created: now.AddDate(-2, 0, 0).UnixMilli()}, BackupInfo{Name: "m2", Kind: "manual"})
	remove := Rotate(all, now)
	removed := map[string]bool{}
	for _, r := range remove {
		removed[r.Name] = true
	}
	kept := len(all) - len(remove)
	if removed["m1"] || removed["m2"] || removed[all[0].Name] || kept > 23+2 || kept < 15 {
		t.Fatalf("kept %d", kept)
	}
}

func TestAggregateHour(t *testing.T) {
	st, _ := setup(t)
	h := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 60; i++ {
		ok, lat := 1, any((i+1)*1000)
		if i < 6 {
			ok, lat = 0, nil
		}
		st.DB.Exec(`INSERT INTO samples(device_id,ts,ok,latency_us,loss_pct) VALUES(1,?,?,?,?)`, h.Add(time.Duration(i)*time.Minute).UnixMilli(), ok, lat, float64(100*(1-ok)))
	}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := AggregateHour(ctx, st, h.Add(17*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	var checks, okc, rows int
	var mn, mx float64
	st.DB.QueryRow(`SELECT COUNT(*),SUM(checks),SUM(ok_checks),MIN(lat_min),MAX(lat_max) FROM samples_hourly`).Scan(&rows, &checks, &okc, &mn, &mx)
	if rows != 1 || checks != 60 || okc != 54 || mn != 7 || mx != 60 {
		t.Fatalf("rows %d checks %d ok %d min %v max %v", rows, checks, okc, mn, mx)
	}
}

func TestRetentionOnlyAggregated(t *testing.T) {
	st, _ := setup(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 4, 0, 0, 0, time.UTC)
	old1 := now.Add(-100 * 24 * time.Hour).Truncate(time.Hour)
	old2 := old1.Add(time.Hour)
	recent := now.Add(-time.Hour)
	for _, ts := range []time.Time{old1, old1.Add(time.Minute), old2, recent} {
		st.DB.Exec(`INSERT INTO samples(device_id,ts,ok,latency_us,loss_pct) VALUES(1,?,1,1000,0)`, ts.UnixMilli())
	}
	AggregateHour(ctx, st, old1)
	n, err := Retention(ctx, st, now, 90*24*time.Hour)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	var left int
	st.DB.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&left)
	if left != 2 {
		t.Fatal(left)
	}
	if err := Optimize(ctx, st); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerBackupRetryAfterFailure(t *testing.T) {
	st, dir := setup(t)
	set := settings.NewHolder(settings.Defaults())
	b := NewBackupService(st, dir, set)
	var reports []string
	r := NewRunner(st, set, b, auth.NewSessions(st), func(last *int64, msg string) { reports = append(reports, msg) })
	calls := 0
	r.backupFn = func(context.Context) (BackupInfo, error) { calls++; return BackupInfo{}, errors.New("disc plin") }
	day := time.Date(2026, 11, 9, 1, 0, 0, 0, r.Zone) // before backup hour
	ctx := context.Background()
	r.tickBackup(ctx, day)
	if calls != 0 {
		t.Fatal("backup before the configured hour")
	}
	at3 := time.Date(2026, 11, 9, 3, 0, 30, 0, r.Zone)
	r.tickBackup(ctx, at3)
	r.tickBackup(ctx, at3.Add(10*time.Minute))
	if calls != 1 || len(reports) != 1 || !strings.Contains(reports[0], "disc plin") {
		t.Fatalf("calls %d reports %v", calls, reports)
	}
	r.backupFn = func(ctx context.Context) (BackupInfo, error) { calls++; return b.Create(ctx, "auto") }
	r.tickBackup(ctx, at3.Add(61*time.Minute))
	r.tickBackup(ctx, at3.Add(70*time.Minute))
	if calls != 2 || reports[len(reports)-1] != "" {
		t.Fatalf("calls %d reports %v", calls, reports)
	}
}

// Review fix: copies in the secondary directory follow the same rotation.
func TestRotateAllIncludesSecondary(t *testing.T) {
	st, dir := setup(t)
	s := settings.Defaults()
	s.BackupSecondaryDir = filepath.Join(dir, "second")
	b := NewBackupService(st, dir, settings.NewHolder(s))
	now := time.Date(2026, 11, 9, 3, 0, 0, 0, Zone)
	for _, d := range []string{b.Dir, s.BackupSecondaryDir} {
		os.MkdirAll(d, 0o750)
		for i := 0; i < 60; i++ {
			name := "sismon-" + now.AddDate(0, 0, -i).Format("20060102-150405") + "-auto.db"
			os.WriteFile(filepath.Join(d, name), []byte("x"), 0o600)
		}
	}
	if err := b.RotateAll(now); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{b.Dir, s.BackupSecondaryDir} {
		entries, _ := os.ReadDir(d)
		if len(entries) > 23 || len(entries) < 7 {
			t.Fatalf("%s: %d files after rotation", d, len(entries))
		}
	}
}

// Review fix: an hour is aggregated only once it ended at least 5 minutes ago,
// so results flushed late still land in it.
func TestRunnerAggregatesOnlySettledHours(t *testing.T) {
	st, dir := setup(t)
	set := settings.NewHolder(settings.Defaults())
	r := NewRunner(st, set, NewBackupService(st, dir, set), auth.NewSessions(st), nil)
	now := time.Date(2026, 11, 9, 11, 2, 0, 0, time.UTC)
	r.tickAggregate(context.Background(), now)
	if want := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC); !r.lastAgg.Equal(want) {
		t.Fatalf("aggregated up to %v, want %v", r.lastAgg, want)
	}
	r.tickAggregate(context.Background(), now.Add(4*time.Minute)) // 11:06
	if want := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC); !r.lastAgg.Equal(want) {
		t.Fatalf("aggregated up to %v, want %v", r.lastAgg, want)
	}
}
