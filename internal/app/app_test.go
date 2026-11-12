package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"sismon/internal/config"
	"sismon/internal/monitor/icmp"
)

func okPinger() icmp.Pinger {
	return &icmp.Fake{Fn: func(netip.Addr) icmp.Result { l := 1.0; return icmp.Result{Sent: 1, Received: 1, LatencyMs: &l} }}
}

type running struct {
	base   string
	cancel context.CancelFunc
	done   chan error
	client *http.Client
}

func start(t *testing.T, cfg config.Config) *running {
	t.Helper()
	ready := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan error, 1), client: &http.Client{Timeout: 5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}}
	go func() {
		r.done <- Run(ctx, cfg, Options{Pinger: okPinger(), Ready: func(a string) { ready <- a }, IntervalScale: 1.0 / 600, FlushEvery: 20 * time.Millisecond})
	}()
	select {
	case a := <-ready:
		scheme := "http"
		if cfg.TLS != "" {
			scheme = "https"
		}
		r.base = scheme + "://" + a
	case err := <-r.done:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("not ready")
	}
	t.Cleanup(func() { r.stop(t) })
	return r
}

func (r *running) stop(t *testing.T) {
	if r.cancel == nil {
		return
	}
	r.cancel()
	r.cancel = nil
	select {
	case err := <-r.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return within 10s")
	}
}

func (r *running) req(t *testing.T, method, path string, body any, hdr map[string]string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, r.base+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", r.base)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	out.ReadFrom(resp.Body)
	return resp, out.Bytes()
}

func TestAppRunIntegration(t *testing.T) {
	r := start(t, config.Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if resp, _ := r.req(t, "GET", "/api/state", nil, nil); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	resp, body := r.req(t, "POST", "/api/admin/setup", map[string]string{"password": "parola-integrare-1"}, nil)
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode, string(body))
	}
	var s struct{ CSRF string }
	json.Unmarshal(body, &s)
	cookie := resp.Cookies()[0]
	hdr := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value, "X-CSRF-Token": s.CSRF}
	loc := map[string]any{"name": "Centru", "devices": []map[string]any{{"name": "Router", "host": "127.0.0.1", "is_primary": true}}}
	if resp, body := r.req(t, "POST", "/api/admin/locations", loc, hdr); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode, string(body))
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, body := r.req(t, "GET", "/api/state", nil, nil)
		if strings.Contains(string(body), `"state":"up"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("device never up: %s", body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	r.stop(t)
}

func TestAppTLSAuto(t *testing.T) {
	r := start(t, config.Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0", TLS: "auto"})
	if resp, _ := r.req(t, "GET", "/api/state", nil, nil); resp.StatusCode != 200 || resp.TLS == nil {
		t.Fatal(resp.StatusCode)
	}
}

func TestAppRefusesSecondInstance(t *testing.T) {
	dir := t.TempDir()
	start(t, config.Config{DataDir: dir, Listen: "127.0.0.1:0"})
	err := Run(context.Background(), config.Config{DataDir: dir, Listen: "127.0.0.1:0"}, Options{Pinger: okPinger()})
	if err == nil || !strings.Contains(err.Error(), "folosit") {
		t.Fatal(err)
	}
}
