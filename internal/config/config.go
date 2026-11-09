// Package config loads startup options from netwatch.toml and flags.
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config are the startup options.
type Config struct {
	DataDir        string
	Listen         string
	TLS            string // "" | "auto" | "files"
	TLSCert        string
	TLSKey         string
	TrustedProxies []string
	Demo           bool
}

type fileConfig struct {
	Listen       *string  `toml:"listen"`
	TLS          *string  `toml:"tls"`
	TLSCert      *string  `toml:"tls_cert"`
	TLSKey       *string  `toml:"tls_key"`
	TrustedProxy []string `toml:"trusted_proxy"`
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// Load parses flags, then overlays them on <data>/netwatch.toml.
func Load(fs *flag.FlagSet, args []string) (Config, error) {
	var c Config
	var proxies multi
	fs.StringVar(&c.DataDir, "data", DefaultDataDir(), "directorul de date")
	fs.StringVar(&c.Listen, "listen", ":8501", "adresa de ascultare")
	fs.StringVar(&c.TLS, "tls", "", `HTTPS: "auto" (certificat autosemnat) sau "files"`)
	fs.StringVar(&c.TLSCert, "tls-cert", "", "fișierul certificatului (cu --tls files)")
	fs.StringVar(&c.TLSKey, "tls-key", "", "fișierul cheii private (cu --tls files)")
	fs.Var(&proxies, "trusted-proxy", "IP/CIDR al unui reverse proxy de încredere (repetabil)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DataDir = abs
	var fc fileConfig
	if _, err := toml.DecodeFile(filepath.Join(c.DataDir, "netwatch.toml"), &fc); err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, fmt.Errorf("netwatch.toml: %w", err)
	}
	apply := func(name string, dst *string, v *string) {
		if v != nil && !set[name] {
			*dst = *v
		}
	}
	apply("listen", &c.Listen, fc.Listen)
	apply("tls", &c.TLS, fc.TLS)
	apply("tls-cert", &c.TLSCert, fc.TLSCert)
	apply("tls-key", &c.TLSKey, fc.TLSKey)
	c.TrustedProxies = fc.TrustedProxy
	if set["trusted-proxy"] {
		c.TrustedProxies = proxies
	}
	switch c.TLS {
	case "", "auto":
	case "files":
		if c.TLSCert == "" || c.TLSKey == "" {
			return c, errors.New("cu --tls files trebuie indicate certificatul (--tls-cert) și cheia (--tls-key)")
		}
	default:
		return c, fmt.Errorf("valoare --tls necunoscută: %q (acceptat: auto, files)", c.TLS)
	}
	return c, nil
}
