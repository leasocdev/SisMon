// Package api serves the HTTP API (public, admin, SSE) and the web UI.
package api

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/auth"
	"sismon/internal/jobs"
	"sismon/internal/live"
	"sismon/internal/monitor/icmp"
	"sismon/internal/settings"
	"sismon/internal/store"
)

// Monitor is the part of the monitoring service the API uses.
type Monitor interface {
	Reload(ctx context.Context) error
	TestHost(ctx context.Context, host string) (icmp.Result, error)
	Health() live.Health
}

// Backups is the backup service as seen by the API.
type Backups interface {
	List() ([]jobs.BackupInfo, error)
	Create(ctx context.Context, kind string) (jobs.BackupInfo, error)
	Open(name string) (io.ReadSeekCloser, jobs.BackupInfo, error)
}

// Deps are the server's dependencies.
type Deps struct {
	Store    *store.Store
	Hub      *live.Hub
	Settings *settings.Holder
	Monitor  Monitor
	Backups  Backups
	Sessions *auth.Sessions
	Limiter  *auth.Limiter
	Proxy    auth.Proxy
	Web      fs.FS // nil until the web UI is embedded (Plan 2)
	// Demo marks the simulated demo mode (shown as a banner).
	Demo bool
	// Logo returns the uploaded logo (PNG) or nil; used by the PDF export.
	Logo func() []byte
	// Tiles serves /tiles/{z}/{x}/{y}.png (OSM proxy); nil disables it.
	Tiles   http.Handler
	Version string
	Now     func() time.Time
	DataDir string
	// SSEPing overrides the SSE keep-alive period (tests).
	SSEPing time.Duration
	// Sleep applies the login failure delay; nil means a real, ctx-aware sleep.
	Sleep func(ctx context.Context, d time.Duration)

	// exportSem caps concurrent statistics exports (each one builds a whole report in memory).
	exportSem chan struct{}

	// loginSem caps concurrent password verifications (argon2id uses 64 MiB each).
	loginSem chan struct{}
}

// LoginConcurrency is the number of password checks allowed at once.
const LoginConcurrency = 2

// ExportConcurrency is the number of statistics exports built at once.
const ExportConcurrency = 2

func realSleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// New builds the HTTP handler.
func New(d Deps) http.Handler { return build(d, nil) }

func build(d Deps, extra func(*http.ServeMux, Deps)) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.SSEPing == 0 {
		d.SSEPing = 15 * time.Second
	}
	if d.Sleep == nil {
		d.Sleep = realSleep
	}
	if d.loginSem == nil {
		d.loginSem = make(chan struct{}, LoginConcurrency)
	}
	if d.exportSem == nil {
		d.exportSem = make(chan struct{}, ExportConcurrency)
	}
	mux := http.NewServeMux()
	p := &public{d}
	mux.HandleFunc("GET /api/state", p.state)
	mux.HandleFunc("GET /api/catalog", p.catalog)
	mux.HandleFunc("GET /api/stats", p.stats)
	mux.HandleFunc("GET /api/devices/{id}/timeline", p.timeline)
	mux.HandleFunc("GET /api/devices/{id}/latency", p.latency)
	mux.HandleFunc("GET /api/report24", p.report24)
	mux.HandleFunc("GET /api/timelines", p.timelines)
	mux.HandleFunc("GET /api/locations/{id}/timeline", p.locationTimeline)
	mux.HandleFunc("GET /api/events", p.events)
	mux.HandleFunc("GET /api/export/{format}", p.exportStats)
	mux.HandleFunc("GET /branding/logo", p.logo)
	if d.Tiles != nil {
		mux.Handle("GET /tiles/", d.Tiles)
	}
	registerAdmin(mux, d)
	if extra != nil {
		extra(mux, d)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, apperr.NotFound("Ruta API nu există."))
	})
	if d.Web != nil {
		mux.Handle("/", spaHandler(d.Web))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			io.WriteString(w, "SisMon API "+d.Version+"\n")
		})
	}
	return recoverer(secure(accessLog(mux)))
}
