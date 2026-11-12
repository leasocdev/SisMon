// Package svc installs and runs SisMon as a system service.
package svc

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/kardianos/service"

	"sismon/internal/app"
	"sismon/internal/config"
)

const systemdScript = `[Unit]
Description={{.Description}}
After=network-online.target
Wants=network-online.target

[Service]
ExecStart={{.Path|cmdEscape}}{{range .Arguments}} {{.|cmd}}{{end}}
{{if .UserName}}User={{.UserName}}{{end}}
Restart=on-failure
RestartSec=10
AmbientCapabilities=CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_RAW
NoNewPrivileges=true
UMask=0077

[Install]
WantedBy=multi-user.target
`

// Program runs app.Run under the service manager.
type Program struct {
	cfg    config.Config
	opt    app.Options
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Start implements service.Interface.
func (p *Program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := app.Run(ctx, p.cfg, p.opt); err != nil {
			slog.Error("SisMon s-a oprit cu eroare", "err", err)
			os.Exit(1)
		}
	}()
	return nil
}

// Stop implements service.Interface.
func (p *Program) Stop(service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	return nil
}

func serviceConfig(cfg config.Config) *service.Config {
	args := []string{"run", "--data", cfg.DataDir, "--listen", cfg.Listen}
	if cfg.TLS != "" {
		args = append(args, "--tls", cfg.TLS)
		if cfg.TLS == "files" {
			args = append(args, "--tls-cert", cfg.TLSCert, "--tls-key", cfg.TLSKey)
		}
	}
	for _, p := range cfg.TrustedProxies {
		args = append(args, "--trusted-proxy", p)
	}
	c := &service.Config{Name: "SisMon", DisplayName: "SisMon", Description: "SisMon — monitorizarea rețelei (ICMP)",
		Arguments: args, Option: service.KeyValue{"SystemdScript": systemdScript, "StartType": "automatic", "OnFailure": "restart"}}
	if service.Platform() == "linux-systemd" {
		c.UserName = "sismon"
	}
	return c
}

func newService(cfg config.Config, opt app.Options) (service.Service, *Program, error) {
	p := &Program{cfg: cfg, opt: opt}
	s, err := service.New(p, serviceConfig(cfg))
	return s, p, err
}

// Interactive reports whether the process runs from a console.
func Interactive() bool { return service.Interactive() }

// RunAsService hands control to the service manager.
func RunAsService(cfg config.Config, opt app.Options) error {
	s, _, err := newService(cfg, opt)
	if err != nil {
		return err
	}
	return s.Run()
}

// Control performs install|uninstall|start|stop.
func Control(action string, cfg config.Config) error {
	s, _, err := newService(cfg, app.Options{})
	if err != nil {
		return err
	}
	return service.Control(s, action)
}

// Status returns "rulează", "oprit" or "neinstalat".
func Status(cfg config.Config) (string, error) {
	s, _, err := newService(cfg, app.Options{})
	if err != nil {
		return "", err
	}
	st, err := s.Status()
	switch {
	case err == service.ErrNotInstalled:
		return "neinstalat", nil
	case err != nil:
		return "", err
	case st == service.StatusRunning:
		return "rulează", nil
	}
	return "oprit", nil
}

// Running reports whether the service is installed and running.
func Running(cfg config.Config) bool {
	st, err := Status(cfg)
	return err == nil && st == "rulează"
}
