package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"netwatch/internal/app"
	"netwatch/internal/config"
	"netwatch/internal/demo"
	"netwatch/internal/monitor/icmp"
)

// Test hooks: demoReady receives the listen address, demoCtx replaces the
// signal context.
var (
	demoReady func(addr string)
	demoCtx   context.Context
)

func cmdDemo(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	data := fs.String("data", "demo-data", "directorul de date al demonstrației")
	listen := fs.String("listen", ":8502", "adresa de ascultare")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir, err := filepath.Abs(*data)
	if err != nil {
		return fail(stderr, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fail(stderr, err)
	}
	_, closeLog := app.SetupLogging(dir, slog.LevelInfo)
	defer closeLog()

	ctx := demoCtx
	if ctx == nil {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
	}
	st, err := app.OpenStore(ctx, dir)
	if err != nil {
		return fail(stderr, err)
	}
	err = demo.Seed(ctx, st)
	st.Close()
	if err != nil {
		return fail(stderr, err)
	}

	sc := demo.Scenario{Start: time.Now()}
	fmt.Fprintf(stdout, "NetWatch — MOD DEMONSTRATIV (date simulate)\nDirector: %s\nAdresă: %s\nParola de administrare: %s\nOprire: Ctrl+C\n", dir, *listen, demo.Password)
	err = app.Run(ctx, config.Config{DataDir: dir, Listen: *listen}, app.Options{
		Pinger: &icmp.Fake{Fn: func(a netip.Addr) icmp.Result { return sc.Result(a, time.Now()) }},
		Lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
			a, err := netip.ParseAddr(host)
			if err != nil {
				return nil, fmt.Errorf("nume necunoscut în demo: %s", host)
			}
			return []netip.Addr{a}, nil
		},
		IntervalScale: 1.0 / 6,
		Demo:          true,
		Ready:         demoReady,
	})
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}
