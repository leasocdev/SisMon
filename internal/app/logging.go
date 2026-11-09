package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			h.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (f fanout) WithAttrs(a []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(a)
	}
	return out
}

func (f fanout) WithGroup(n string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(n)
	}
	return out
}

// SetupLogging logs JSON to <data>/logs/netwatch.log (5 × 5 MB) and text to
// stdout, and installs the logger as the slog default.
func SetupLogging(dataDir string, level slog.Level) (*slog.Logger, func()) {
	lj := &lumberjack.Logger{Filename: filepath.Join(dataDir, "logs", "netwatch.log"), MaxSize: 5, MaxBackups: 5}
	var stdout io.Writer = os.Stdout
	opts := &slog.HandlerOptions{Level: level}
	l := slog.New(fanout{slog.NewJSONHandler(lj, opts), slog.NewTextHandler(stdout, opts)})
	slog.SetDefault(l)
	return l, func() { lj.Close() }
}
