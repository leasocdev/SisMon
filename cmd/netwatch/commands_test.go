package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"netwatch/internal/auth"
	"netwatch/internal/lockfile"
	"netwatch/internal/store"
)

func cli(t *testing.T, stdin string, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &out)
	return code, out.String()
}

func TestUnknownCommandUsage(t *testing.T) {
	code, out := cli(t, "", "x")
	if code != 2 || !strings.Contains(out, "Utilizare") {
		t.Fatal(code, out)
	}
}

func TestSetPasswordNonInteractive(t *testing.T) {
	dir := t.TempDir()
	code, out := cli(t, "parola-de-test-12\nparola-de-test-12\n", "set-password", "--data", dir)
	if code != 0 {
		t.Fatal(code, out)
	}
	st, err := store.Open(t.Context(), filepath.Join(dir, "netwatch.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var h string
	st.GetMeta(t.Context(), "password_hash", &h)
	st.Close()
	if !auth.VerifyPassword(h, "parola-de-test-12") {
		t.Fatal("hash not stored")
	}
	code, out = cli(t, "parola-de-test-12\nalta-parola-1234\n", "set-password", "--data", dir)
	if code != 1 || !strings.Contains(out, "Parolele nu coincid.") {
		t.Fatal(code, out)
	}
	code, out = cli(t, "scurta\nscurta\n", "set-password", "--data", dir)
	if code != 1 || !strings.Contains(out, "12 și 200") {
		t.Fatal(code, out)
	}
}

func TestBackupCommandPrintsPath(t *testing.T) {
	dir := t.TempDir()
	code, out := cli(t, "", "backup", "--data", dir)
	path := strings.TrimSpace(out[strings.LastIndex(strings.TrimSpace(out), "\n")+1:])
	if code != 0 || !strings.HasSuffix(path, "-manual.db") {
		t.Fatal(code, out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestBackupCommandLocked(t *testing.T) {
	dir := t.TempDir()
	release, err := lockfile.Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	code, out := cli(t, "", "backup", "--data", dir)
	if code != 1 || !strings.Contains(out, "Directorul de date este folosit de o instanță activă.") {
		t.Fatal(code, out)
	}
}

func TestOutputUTF8Redirected(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	exe := filepath.Join(t.TempDir(), "netwatch")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	cmd := exec.Command(exe, "set-password", "--data", t.TempDir())
	cmd.Stdin = strings.NewReader("parola-de-test-12\naltceva-altceva\n")
	out, _ := cmd.CombinedOutput()
	if !utf8.Valid(out) || !strings.Contains(string(out), "Parolele nu coincid.") {
		t.Fatalf("%q", out)
	}
}

func TestCmdDemoStarts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo-data")
	ready := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	demoReady, demoCtx = func(a string) { ready <- a }, ctx
	defer func() { demoReady, demoCtx = nil, nil }()
	done := make(chan int, 1)
	var out bytes.Buffer
	go func() { done <- run([]string{"demo", "--data", dir, "--listen", "127.0.0.1:0"}, strings.NewReader(""), &out, &out) }()
	var addr string
	select {
	case addr = <-ready:
	case code := <-done:
		t.Fatalf("exited %d: %s", code, out.String())
	case <-time.After(20 * time.Second):
		t.Fatal("not ready")
	}
	resp, err := http.Get("http://" + addr + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Demo      bool
		Locations []struct{ Name string }
	}
	json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if !st.Demo || len(st.Locations) != 12 {
		t.Fatalf("%+v", st)
	}
	if !strings.Contains(out.String(), "netwatch-demo") {
		t.Fatalf("password not printed: %s", out.String())
	}
}
