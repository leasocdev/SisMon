package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"netwatch/internal/auth"
	"netwatch/internal/settings"
	"netwatch/internal/store"
)

// SamplesRetention is how long raw samples are kept.
const SamplesRetention = 90 * 24 * time.Hour

// Runner schedules the background jobs, each on its own goroutine.
type Runner struct {
	Now  func() time.Time
	Tick time.Duration
	Zone *time.Location

	st       *store.Store
	set      *settings.Holder
	b        *BackupService
	sessions *auth.Sessions
	onBackup func(last *int64, msg string)
	backupFn func(context.Context) (BackupInfo, error)

	mu          sync.Mutex
	lastBackup  string // local date of the last successful automatic backup
	nextAttempt time.Time
	lastAgg     time.Time
	lastMaint   string
}

// NewRunner builds the job runner.
func NewRunner(st *store.Store, set *settings.Holder, b *BackupService, s *auth.Sessions, onBackup func(last *int64, msg string)) *Runner {
	r := &Runner{Now: time.Now, Tick: time.Minute, Zone: Zone, st: st, set: set, b: b, sessions: s, onBackup: onBackup}
	r.backupFn = func(ctx context.Context) (BackupInfo, error) { return b.Create(ctx, "auto") }
	if onBackup == nil {
		r.onBackup = func(*int64, string) {}
	}
	if list, err := b.List(); err == nil {
		for _, i := range list {
			if i.Kind == "auto" {
				r.lastBackup = time.UnixMilli(i.Created).In(r.Zone).Format("2006-01-02")
				break
			}
		}
	}
	return r
}

// tickBackup runs the daily backup once the configured hour has passed,
// retrying one hour after a failure.
func (r *Runner) tickBackup(ctx context.Context, now time.Time) {
	r.mu.Lock()
	local := now.In(r.Zone)
	day := local.Format("2006-01-02")
	due := local.Hour() >= r.set.Get().BackupHour && r.lastBackup != day && !now.Before(r.nextAttempt)
	r.mu.Unlock()
	if !due {
		return
	}
	info, err := r.backupFn(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.nextAttempt = now.Add(time.Hour)
		slog.Error("backup automat", "err", err)
		r.onBackup(nil, "Backup-ul automat a eșuat: "+err.Error()+" Următoarea încercare peste o oră.")
		return
	}
	r.lastBackup = day
	if err := r.b.RotateAll(now); err != nil {
		slog.Warn("rotație backup", "err", err)
	}
	created := info.Created
	r.onBackup(&created, r.b.Warning())
}

func (r *Runner) tickAggregate(ctx context.Context, now time.Time) {
	// Last hour that ended at least 5 minutes ago, so late flushes are included.
	last := now.Add(-5 * time.Minute).UTC().Truncate(time.Hour).Add(-time.Hour)
	r.mu.Lock()
	from := r.lastAgg
	if from.IsZero() {
		from = last.Add(-47 * time.Hour) // catch up after a restart
	} else {
		from = from.Add(time.Hour)
	}
	r.mu.Unlock()
	for h := from; !h.After(last); h = h.Add(time.Hour) {
		if err := AggregateHour(ctx, r.st, h); err != nil {
			slog.Error("agregare orară", "err", err)
			return
		}
		r.mu.Lock()
		r.lastAgg = h
		r.mu.Unlock()
	}
}

func (r *Runner) tickMaintenance(ctx context.Context, now time.Time) {
	local := now.In(r.Zone)
	day := local.Format("2006-01-02")
	r.mu.Lock()
	due := local.Hour() >= 4 && r.lastMaint != day
	if due {
		r.lastMaint = day
	}
	r.mu.Unlock()
	if !due {
		return
	}
	if _, err := Retention(ctx, r.st, now, SamplesRetention); err != nil {
		slog.Error("retenție", "err", err)
	}
	if err := Optimize(ctx, r.st); err != nil {
		slog.Error("optimizare", "err", err)
	}
}

func (r *Runner) loop(ctx context.Context, every time.Duration, fn func(context.Context, time.Time)) {
	fn(ctx, r.Now())
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx, r.Now())
		}
	}
}

// Run starts all jobs and blocks until ctx is done.
func (r *Runner) Run(ctx context.Context) {
	var wg sync.WaitGroup
	jobs := []struct {
		every time.Duration
		fn    func(context.Context, time.Time)
	}{
		{r.Tick, r.tickBackup},
		{r.Tick, r.tickAggregate},
		{r.Tick, r.tickMaintenance},
		{10 * r.Tick, func(ctx context.Context, now time.Time) {
			if err := r.sessions.Cleanup(ctx, now); err != nil {
				slog.Warn("curățare sesiuni", "err", err)
			}
		}},
	}
	for _, j := range jobs {
		wg.Add(1)
		go func() { defer wg.Done(); r.loop(ctx, j.every, j.fn) }()
	}
	wg.Wait()
}
