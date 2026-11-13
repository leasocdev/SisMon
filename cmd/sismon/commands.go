package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"sismon/internal/app"
	"sismon/internal/apperr"
	"sismon/internal/auth"
	"sismon/internal/config"
	"sismon/internal/fsown"
	"sismon/internal/jobs"
	"sismon/internal/lockfile"
	"sismon/internal/restore"
	"sismon/internal/settings"
	"sismon/internal/svc"
)

const lockedMsg = "Directorul de date este folosit de o instanță activă. Opriți serviciul (sismon stop) sau folosiți pagina de administrare."

func loadConfig(name string, args []string, stderr io.Writer) (config.Config, bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg, err := config.Load(fs, args)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stderr, "Eroare:", err)
		}
		return cfg, false
	}
	return cfg, true
}

func fail(stderr io.Writer, err error) int {
	var ae *apperr.Error
	switch {
	case errors.Is(err, lockfile.ErrLocked):
		fmt.Fprintln(stderr, lockedMsg)
	case errors.As(err, &ae):
		fmt.Fprintln(stderr, ae.Message)
	default:
		fmt.Fprintln(stderr, "Eroare:", err)
	}
	return 1
}

func cmdRun(args []string, stdout, stderr io.Writer) int {
	cfg, ok := loadConfig("run", args, stderr)
	if !ok {
		return 2
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fail(stderr, err)
	}
	_, closeLog := app.SetupLogging(cfg.DataDir, slog.LevelInfo)
	defer closeLog()
	if !svc.Interactive() {
		if err := svc.RunAsService(cfg, app.Options{}); err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg, app.Options{}); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func cmdService(action string, args []string, stdout, stderr io.Writer) int {
	cfg, ok := loadConfig(action, args, stderr)
	if !ok {
		return 2
	}
	if action == "status" {
		st, err := svc.Status(cfg)
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintln(stdout, "Serviciul SisMon:", st)
		return 0
	}
	if action == "install" {
		if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
			return fail(stderr, err)
		}
		if err := fsown.Protect(cfg.DataDir); err != nil {
			fmt.Fprintln(stderr, "Avertisment:", err)
		}
	}
	if err := svc.Control(action, cfg); err != nil {
		return fail(stderr, fmt.Errorf("%s: %w (rulați ca administrator/root?)", action, err))
	}
	msgs := map[string]string{"install": "Serviciul a fost instalat. Porniți-l cu: sismon start",
		"uninstall": "Serviciul a fost eliminat.", "start": "Serviciul a pornit.", "stop": "Serviciul a fost oprit."}
	fmt.Fprintln(stdout, msgs[action])
	return 0
}

func readPasswords(stdin io.Reader, stdout io.Writer) (string, string, error) {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(stdout, "Parola administratorului (12–200 caractere): ")
		a, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stdout)
		if err != nil {
			return "", "", err
		}
		fmt.Fprint(stdout, "Repetați parola: ")
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stdout)
		return string(a), string(b), err
	}
	r := bufio.NewReader(stdin)
	line := func() (string, error) {
		s, err := r.ReadString('\n')
		if err != nil && !(errors.Is(err, io.EOF) && s != "") {
			return "", errors.New("parola nu a putut fi citită")
		}
		return strings.TrimRight(s, "\r\n"), nil
	}
	a, err := line()
	if err != nil {
		return "", "", err
	}
	b, err := line()
	return a, b, err
}

func cmdSetPassword(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, ok := loadConfig("set-password", args, stderr)
	if !ok {
		return 2
	}
	release, err := lockfile.Acquire(cfg.DataDir)
	if err != nil {
		return fail(stderr, err)
	}
	defer release()
	defer fsown.MatchTree(cfg.DataDir) // runs after the store is closed
	a, b, err := readPasswords(stdin, stdout)
	if err != nil {
		return fail(stderr, err)
	}
	if a != b {
		fmt.Fprintln(stderr, "Parolele nu coincid.")
		return 1
	}
	if err := auth.ValidatePassword(a); err != nil {
		return fail(stderr, err)
	}
	ctx := context.Background()
	st, err := app.OpenStore(ctx, cfg.DataDir)
	if err != nil {
		return fail(stderr, err)
	}
	defer st.Close()
	h, err := auth.HashPassword(a)
	if err == nil {
		err = st.SetMeta(ctx, "password_hash", h)
	}
	if err == nil {
		err = auth.NewSessions(st).DeleteAll(ctx)
	}
	if err == nil {
		err = st.Audit(ctx, st.DB, "set_password_cli", "admin", 0, map[string]any{}, time.Now())
	}
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, "Parola a fost setată. Sesiunile existente au fost închise.")
	return 0
}

func cmdBackup(args []string, stdout, stderr io.Writer) int {
	cfg, ok := loadConfig("backup", args, stderr)
	if !ok {
		return 2
	}
	release, err := lockfile.Acquire(cfg.DataDir)
	if err != nil {
		return fail(stderr, err)
	}
	defer release()
	defer fsown.MatchTree(cfg.DataDir)
	ctx := context.Background()
	st, err := app.OpenStore(ctx, cfg.DataDir)
	if err != nil {
		return fail(stderr, err)
	}
	defer st.Close()
	set, err := settings.Load(ctx, st)
	if err != nil {
		return fail(stderr, err)
	}
	b := jobs.NewBackupService(st, cfg.DataDir, set)
	info, err := b.Create(ctx, "manual")
	if err != nil {
		return fail(stderr, err)
	}
	if w := b.Warning(); w != "" {
		fmt.Fprintln(stderr, w)
	}
	fmt.Fprintln(stdout, filepath.Join(b.Dir, info.Name))
	return 0
}

func cmdRestore(args []string, stdout, stderr io.Writer) int {
	var file string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		file, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg, err := config.Load(fs, args)
	if err != nil {
		return 2
	}
	if file == "" {
		file = fs.Arg(0)
	}
	if file == "" {
		fmt.Fprintln(stderr, "Indicați fișierul de backup: sismon restore FIȘIER")
		return 2
	}
	if err := restore.Verify(file); err != nil {
		return fail(stderr, err)
	}
	wasRunning := svc.Running(cfg)
	if wasRunning {
		fmt.Fprintln(stdout, "Opresc serviciul…")
		if err := svc.Control("stop", cfg); err != nil {
			return fail(stderr, err)
		}
		time.Sleep(2 * time.Second)
	}
	release, err := lockfile.Acquire(cfg.DataDir)
	if err != nil {
		return fail(stderr, err)
	}
	pre, err := restore.Restore(context.Background(), cfg.DataDir, file)
	fsown.MatchTree(cfg.DataDir)
	release()
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, "Restaurare finalizată. Copia bazei anterioare:", pre.Name)
	if wasRunning {
		if err := svc.Control("start", cfg); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintln(stdout, "Serviciul a fost repornit.")
	}
	return 0
}
