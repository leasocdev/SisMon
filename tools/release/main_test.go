package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArtifactName(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"windows", "amd64"}: "netwatch-windows-amd64.exe",
		{"linux", "amd64"}:   "netwatch-linux-amd64",
		{"linux", "arm64"}:   "netwatch-linux-arm64",
	} {
		if got := artifactName(in[0], in[1]); got != want {
			t.Errorf("%v → %s", in, got)
		}
	}
}

func TestWriteChecksums(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "b"), []byte("beta"), 0o644)
	os.WriteFile(filepath.Join(dir, "a"), []byte("alpha"), 0o644)
	if err := writeChecksums(dir, []string{"b", "a"}); err != nil {
		t.Fatal(err)
	}
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	want := sum("alpha") + "  a\n" + sum("beta") + "  b\n"
	got, _ := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if string(got) != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
}

func TestReleaseBuildHost(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	if _, err := os.Stat("../../internal/webui/dist/index.html"); err != nil {
		t.Skip("web UI not built")
	}
	out := t.TempDir()
	target := runtime.GOOS + "/" + runtime.GOARCH
	if err := release(options{root: "../..", out: out, version: "9.9.9", targets: []string{target}, skipWeb: true}); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(out, artifactName(runtime.GOOS, runtime.GOARCH))
	v, err := exec.Command(bin, "version").Output()
	if err != nil || !strings.Contains(string(v), "9.9.9") {
		t.Fatal(string(v), err)
	}
	sums, _ := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if !strings.Contains(string(sums), artifactName(runtime.GOOS, runtime.GOARCH)) {
		t.Fatal(string(sums))
	}
}
