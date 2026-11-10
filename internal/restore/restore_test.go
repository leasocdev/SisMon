package restore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"netwatch/internal/app"
	"netwatch/internal/auth"
	"netwatch/internal/domain"
	"netwatch/internal/jobs"
	"netwatch/internal/settings"
	"netwatch/internal/store"
)

func addLocation(t *testing.T, st *store.Store, name string) {
	in := domain.LocationInput{Name: name, Devices: []domain.DeviceInput{{Name: "R", Host: "10.0.0.1", Primary: true}}}
	in.Normalize(settings.Defaults())
	if _, err := st.SaveLocation(context.Background(), nil, in, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func names(t *testing.T, path string) string {
	st, err := store.Open(context.Background(), path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	locs, _ := st.ListLocations(context.Background(), false)
	var out []string
	for _, l := range locs {
		out = append(out, l.Name)
	}
	return strings.Join(out, ",")
}

func TestRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := app.OpenStore(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	addLocation(t, st, "X")
	b := jobs.NewBackupService(st, dir, settings.NewHolder(settings.Defaults()))
	info, err := b.Create(ctx, "manual")
	if err != nil {
		t.Fatal(err)
	}
	addLocation(t, st, "Y")
	auth.NewSessions(st).Create(ctx, time.Now())
	st.Close()

	pre, err := Restore(ctx, dir, filepath.Join(dir, "backups", info.Name))
	if err != nil {
		t.Fatal(err)
	}
	if got := names(t, filepath.Join(dir, "netwatch.db")); got != "X" {
		t.Fatalf("restored %q", got)
	}
	if got := names(t, filepath.Join(dir, "backups", pre.Name)); got != "X,Y" || pre.Kind != "pre-restore" {
		t.Fatalf("pre-restore %q %+v", got, pre)
	}
	st2, _ := store.Open(ctx, filepath.Join(dir, "netwatch.db"), store.Options{})
	defer st2.Close()
	var n int
	st2.DB.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	if n != 0 {
		t.Fatal("sessions survived restore")
	}
}

func TestRestoreRejectsNewerSchema(t *testing.T) {
	ctx := context.Background()
	dir, srcDir := t.TempDir(), t.TempDir()
	st, _ := app.OpenStore(ctx, dir)
	addLocation(t, st, "Curent")
	st.Close()
	src, _ := store.Open(ctx, filepath.Join(srcDir, "n.db"), store.Options{})
	src.SetMeta(ctx, "schema_version", 99)
	src.Close()
	if _, err := Restore(ctx, dir, filepath.Join(srcDir, "n.db")); err == nil {
		t.Fatal("newer schema accepted")
	}
	if got := names(t, filepath.Join(dir, "netwatch.db")); got != "Curent" {
		t.Fatal("current database changed")
	}
}

func TestRestoreRejectsCorrupt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, _ := app.OpenStore(ctx, dir)
	st.Close()
	bad := filepath.Join(t.TempDir(), "bad.db")
	os.WriteFile(bad, []byte("nu sunt o bază de date"), 0o600)
	_, err := Restore(ctx, dir, bad)
	if err == nil || !strings.Contains(err.Error(), "Fișierul nu este o bază NetWatch validă.") {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, dir, filepath.Join(dir, "lipsa.db")); err == nil {
		t.Fatal("missing file accepted")
	}
}
