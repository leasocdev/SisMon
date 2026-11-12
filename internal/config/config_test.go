package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, args ...string) (Config, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	return Load(fs, args)
}

func TestConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	c, err := load(t, "--data", dir)
	if err != nil || c.Listen != ":8501" || c.DataDir != dir || c.TLS != "" {
		t.Fatalf("%+v %v", c, err)
	}
	if DefaultDataDir() == "" {
		t.Fatal("empty default data dir")
	}
}

func TestConfigFlagsOverrideToml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sismon.toml"), []byte("listen = \":9000\"\ntrusted_proxy = [\"10.0.0.5\"]\n"), 0o600)
	c, err := load(t, "--data", dir)
	if err != nil || c.Listen != ":9000" || len(c.TrustedProxies) != 1 {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = load(t, "--data", dir, "--listen", ":9100", "--trusted-proxy", "10.0.0.6", "--trusted-proxy", "10.0.0.7/32")
	if err != nil || c.Listen != ":9100" || len(c.TrustedProxies) != 2 || c.TrustedProxies[1] != "10.0.0.7/32" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestConfigTLSFilesRequiresPaths(t *testing.T) {
	dir := t.TempDir()
	if _, err := load(t, "--data", dir, "--tls", "files"); err == nil || !strings.Contains(err.Error(), "certificat") {
		t.Fatal(err)
	}
	if _, err := load(t, "--data", dir, "--tls", "weird"); err == nil {
		t.Fatal("invalid tls mode accepted")
	}
	if c, err := load(t, "--data", dir, "--tls", "auto"); err != nil || c.TLS != "auto" {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "sismon.toml"), []byte("listen = "), 0o600)
	if _, err := load(t, "--data", dir); err == nil {
		t.Fatal("broken toml accepted")
	}
}
