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

// The template shipped in deploy/ must parse and, copied unchanged, keep the
// defaults; its commented-out options must be valid once enabled.
func TestDeployTemplate(t *testing.T) {
	tpl, err := os.ReadFile(filepath.Join("..", "..", "deploy", "sismon.toml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sismon.toml"), tpl, 0o600)
	c, err := load(t, "--data", dir)
	if err != nil || c.Listen != ":8501" || c.TLS != "" || len(c.TrustedProxies) != 0 {
		t.Fatalf("%+v %v", c, err)
	}
	on := strings.NewReplacer("# tls = \"files\"", "tls = \"files\"", "# tls_cert", "tls_cert", "# tls_key", "tls_key",
		"# trusted_proxy", "trusted_proxy").Replace(string(tpl))
	os.WriteFile(filepath.Join(dir, "sismon.toml"), []byte(on), 0o600)
	c, err = load(t, "--data", dir)
	if err != nil || c.TLS != "files" || c.TLSCert != `C:\certs\sismon.crt` || c.TLSKey != `C:\certs\sismon.key` || len(c.TrustedProxies) != 1 {
		t.Fatalf("%+v %v", c, err)
	}
}
