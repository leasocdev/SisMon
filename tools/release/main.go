// Command release builds the SisMon release binaries:
//
//	go run ./tools/release [--version V] [--out dist] [--targets windows/amd64,...] [--skip-web]
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var defaultTargets = []string{"windows/amd64", "linux/amd64", "linux/arm64"}

type options struct {
	root, out, version string
	targets            []string
	skipWeb            bool
}

func artifactName(goos, goarch string) string {
	name := "sismon-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func writeChecksums(dir string, names []string) error {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	var b strings.Builder
	for _, n := range sorted {
		f, err := os.Open(filepath.Join(dir, n))
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), n)
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(b.String()), 0o644)
}

func run(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func gitVersion(root string) string {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "0.0.0-dev"
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
}

func release(o options) error {
	if !o.skipWeb {
		npm := "npm"
		if runtime.GOOS == "windows" {
			npm = "npm.cmd"
		}
		web := filepath.Join(o.root, "web")
		if err := run(web, nil, npm, "ci"); err != nil {
			return err
		}
		if err := run(web, nil, npm, "run", "build"); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(o.root, "internal", "webui", "dist", "index.html")); err != nil {
		return errors.New("interfața web nu este compilată; rulați fără --skip-web")
	}
	if o.version == "" {
		o.version = gitVersion(o.root)
	}
	out, err := filepath.Abs(o.out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	var names []string
	for _, t := range o.targets {
		goos, goarch, ok := strings.Cut(t, "/")
		if !ok {
			return fmt.Errorf("țintă invalidă %q (de exemplu linux/amd64)", t)
		}
		name := artifactName(goos, goarch)
		fmt.Printf("→ %s (versiunea %s)\n", name, o.version)
		ldflags := "-s -w -X sismon/internal/version.Version=" + o.version
		env := []string{"CGO_ENABLED=0", "GOOS=" + goos, "GOARCH=" + goarch}
		if err := run(o.root, env, "go", "build", "-trimpath", "-ldflags", ldflags, "-o", filepath.Join(out, name), "./cmd/sismon"); err != nil {
			return err
		}
		names = append(names, name)
	}
	if err := writeChecksums(out, names); err != nil {
		return err
	}
	fmt.Printf("Gata: %s\n", out)
	return nil
}

func main() {
	var o options
	var targets string
	flag.StringVar(&o.version, "version", "", "versiunea (implicit: git describe)")
	flag.StringVar(&o.out, "out", "dist", "directorul de ieșire")
	flag.StringVar(&targets, "targets", strings.Join(defaultTargets, ","), "ținte GOOS/GOARCH separate prin virgulă")
	flag.BoolVar(&o.skipWeb, "skip-web", false, "nu recompila interfața web")
	flag.Parse()
	o.root = "."
	o.targets = strings.Split(targets, ",")
	if err := release(o); err != nil {
		fmt.Fprintln(os.Stderr, "Eroare:", err)
		os.Exit(1)
	}
}
