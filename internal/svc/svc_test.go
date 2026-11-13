package svc

import (
	"flag"
	"io"
	"slices"
	"testing"

	"sismon/internal/config"
)

func load(t *testing.T, args ...string) config.Config {
	t.Helper()
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg, err := config.Load(fs, append([]string{"--data", t.TempDir()}, args...))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Review fix: options not given to `install` must not be frozen into the
// service, otherwise later edits of sismon.toml are silently ignored.
func TestServiceArgsOnlyExplicitFlags(t *testing.T) {
	cfg := load(t)
	if got, want := serviceArgs(cfg), []string{"run", "--data", cfg.DataDir}; !slices.Equal(got, want) {
		t.Fatalf("args %q, want %q", got, want)
	}
	cfg = load(t, "--listen", ":9000", "--tls", "auto", "--trusted-proxy", "10.0.0.5", "--trusted-proxy", "10.0.0.6")
	want := []string{"run", "--data", cfg.DataDir, "--listen", ":9000", "--tls", "auto",
		"--trusted-proxy", "10.0.0.5", "--trusted-proxy", "10.0.0.6"}
	if got := serviceArgs(cfg); !slices.Equal(got, want) {
		t.Fatalf("args %q, want %q", got, want)
	}
}
