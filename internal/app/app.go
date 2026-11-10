// Package app assembles and runs the whole NetWatch server.
package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"sync"
	"time"

	"netwatch/internal/api"
	"netwatch/internal/auth"
	"netwatch/internal/branding"
	"netwatch/internal/config"
	"netwatch/internal/jobs"
	"netwatch/internal/live"
	"netwatch/internal/lockfile"
	"netwatch/internal/monitor"
	"netwatch/internal/monitor/icmp"
	"netwatch/internal/settings"
	"netwatch/internal/store"
	"netwatch/internal/tiles"
	"netwatch/internal/tlsutil"
	"netwatch/internal/version"
	"netwatch/internal/webui"
)

// Options inject dependencies (tests, demo mode, embedded UI).
type Options struct {
	Pinger        icmp.Pinger
	Lookup        monitor.LookupFunc
	Ready         func(addr string)
	Web           fs.FS
	IntervalScale float64
	FlushEvery    time.Duration
	// Wrap, if set, wraps the final HTTP handler (end-to-end test harness).
	Wrap func(http.Handler) http.Handler
	// TilesUpstream overrides the OSM tile server (end-to-end tests).
	TilesUpstream string
	// Demo marks the demo mode in /api/state.
	Demo bool
}

func defaultLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// OpenStore opens the database, backing it up before any migration.
func OpenStore(ctx context.Context, dataDir string) (*store.Store, error) {
	return store.Open(ctx, filepath.Join(dataDir, "netwatch.db"), store.Options{BeforeMigrate: func(s *store.Store, from, to int) error {
		_, err := jobs.NewBackupService(s, dataDir, settings.NewHolder(settings.Defaults())).Create(ctx, "pre-migrate")
		return err
	}})
}

// Run starts every component and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg config.Config, opt Options) error {
	release, err := lockfile.Acquire(cfg.DataDir)
	if err != nil {
		if errors.Is(err, lockfile.ErrLocked) {
			return fmt.Errorf("%w: %s", lockfile.ErrLocked, cfg.DataDir)
		}
		return err
	}
	defer release()

	st, err := OpenStore(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	set, err := settings.Load(ctx, st)
	if err != nil {
		return err
	}
	proxy, err := auth.ParseProxies(cfg.TrustedProxies)
	if err != nil {
		return err
	}
	hub := live.NewHub()
	if err := hub.Load(ctx, st); err != nil {
		return err
	}
	if opt.Pinger == nil {
		if opt.Pinger, err = icmp.New(); err != nil {
			return err
		}
	}
	if opt.Lookup == nil {
		opt.Lookup = defaultLookup
	}
	if opt.Pinger.Mode() == "unavailable" {
		slog.Warn("ICMP indisponibil: lipsește permisiunea pentru socket ICMP (Linux: CAP_NET_RAW sau net.ipv4.ping_group_range)")
	}
	svc := monitor.NewService(st, set, hub, opt.Pinger, opt.Lookup, monitor.ServiceOptions{IntervalScale: opt.IntervalScale, FlushEvery: opt.FlushEvery})
	backups := jobs.NewBackupService(st, cfg.DataDir, set)
	sessions := auth.NewSessions(st)
	runner := jobs.NewRunner(st, set, backups, sessions, svc.SetBackupStatus)

	tp, err := tiles.New(filepath.Join(cfg.DataDir, "tiles"), version.Version)
	if err != nil {
		return err
	}
	if opt.TilesUpstream != "" {
		tp.Upstream = opt.TilesUpstream
	}
	if opt.Web == nil {
		opt.Web = webui.FS()
	}
	handler := api.New(api.Deps{Store: st, Hub: hub, Settings: set, Monitor: svc, Backups: backups, Sessions: sessions,
		Limiter: auth.NewLimiter(5, 5*time.Minute, 10000), Proxy: proxy, Web: opt.Web, Demo: opt.Demo, Tiles: tp, Logo: func() []byte { b, _ := branding.Load(cfg.DataDir); return b }, Version: version.Version, DataDir: cfg.DataDir})
	if opt.Wrap != nil {
		handler = opt.Wrap(handler)
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn)}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("portul %s nu poate fi deschis: %w", cfg.Listen, err)
	}
	switch cfg.TLS {
	case "auto", "files":
		certFile, keyFile := cfg.TLSCert, cfg.TLSKey
		if cfg.TLS == "auto" {
			if certFile, keyFile, err = tlsutil.EnsureSelfSigned(cfg.DataDir, nil); err != nil {
				ln.Close()
				return err
			}
		}
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			ln.Close()
			return fmt.Errorf("certificat TLS: %w", err)
		}
		ln = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	errc := make(chan error, 3)
	wg.Add(3)
	go func() { defer wg.Done(); errc <- svc.Run(ctx) }()
	go func() { defer wg.Done(); runner.Run(ctx); errc <- nil }()
	go func() {
		defer wg.Done()
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			cancel()
			return
		}
		errc <- nil
	}()
	slog.Info("NetWatch pornit", "versiune", version.Version, "adresa", ln.Addr().String(), "date", cfg.DataDir, "icmp", opt.Pinger.Mode())
	if opt.Ready != nil {
		opt.Ready(ln.Addr().String())
	}
	<-ctx.Done()
	shut, cancelShut := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShut()
	srv.Shutdown(shut)
	srv.Close()
	wg.Wait()
	close(errc)
	var first error
	for e := range errc {
		if e != nil && !errors.Is(e, context.Canceled) && first == nil {
			first = e
		}
	}
	return first
}
