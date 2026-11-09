package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"netwatch/internal/apperr"
	"netwatch/internal/auth"
	"netwatch/internal/domain"
	"netwatch/internal/jobs"
	"netwatch/internal/live"
	"netwatch/internal/monitor/icmp"
	"netwatch/internal/settings"
	"netwatch/internal/store"
)

type fakeMonitor struct {
	reloads int
	result  icmp.Result
}

func (m *fakeMonitor) Reload(context.Context) error { m.reloads++; return nil }
func (m *fakeMonitor) TestHost(_ context.Context, h string) (icmp.Result, error) {
	if !domain.ValidHost(h) {
		return icmp.Result{}, apperr.Invalid("host", "Adresă IP sau nume DNS invalid.")
	}
	return m.result, nil
}
func (m *fakeMonitor) Health() live.Health { return live.Health{ICMPMode: "fake"} }

type fakeBackups struct{ dir string }

func (b *fakeBackups) List() ([]jobs.BackupInfo, error) { return []jobs.BackupInfo{}, nil }
func (b *fakeBackups) Create(context.Context, string) (jobs.BackupInfo, error) {
	return jobs.BackupInfo{Name: "netwatch-x-manual.db", Kind: "manual"}, nil
}
func (b *fakeBackups) Open(name string) (io.ReadSeekCloser, jobs.BackupInfo, error) {
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return nil, jobs.BackupInfo{}, apperr.Invalid("name", "Nume de backup invalid.")
	}
	return nil, jobs.BackupInfo{}, apperr.NotFound("Backup inexistent.")
}

type testServer struct {
	d   Deps
	mon *fakeMonitor
	h   http.Handler
	loc int64
	dev []int64
}

func newServer(t *testing.T, extra ...func(*http.ServeMux, Deps)) *testServer {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "api.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	in := domain.LocationInput{Name: "Centru", Devices: []domain.DeviceInput{{Name: "Router", Host: "10.0.0.1", Primary: true}}}
	in.Normalize(settings.Defaults())
	r, err := st.SaveLocation(ctx, nil, in, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	hub := live.NewHub()
	hub.Load(ctx, st)
	mon := &fakeMonitor{result: icmp.Result{Sent: 3, Received: 3}}
	px, _ := auth.ParseProxies([]string{"10.0.0.5"})
	d := Deps{Store: st, Hub: hub, Settings: settings.NewHolder(settings.Defaults()), Monitor: mon, Backups: &fakeBackups{},
		Sessions: auth.NewSessions(st), Limiter: auth.NewLimiter(5, 5*time.Minute, 100), Proxy: px, Version: "test", Now: time.Now}
	var ex func(*http.ServeMux, Deps)
	if len(extra) > 0 {
		ex = extra[0]
	}
	return &testServer{d: d, mon: mon, h: build(d, ex), loc: r.LocationID, dev: r.Added}
}

func (s *testServer) do(method, path string, body any, hdr map[string]string) *httptest.ResponseRecorder {
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "http://nw.local"+path, rd)
	req.RemoteAddr = "192.0.2.10:1234"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if rd != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.h.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("bad json %q: %v", w.Body.String(), err)
	}
}

func TestStateEndpoint(t *testing.T) {
	s := newServer(t)
	w := s.do("GET", "/api/state", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var body struct {
		Locations []live.LocationView `json:"locations"`
		Identity  Identity            `json:"identity"`
		Version   string              `json:"version"`
		IntervalS int                 `json:"interval_s"`
	}
	decode(t, w, &body)
	if len(body.Locations) != 1 || body.Locations[0].Devices[0].State != "unknown" || body.Identity.Name != "NetWatch" || body.Version != "test" || body.IntervalS != 60 {
		t.Fatalf("%+v", body)
	}
}

func TestCatalogIncludesArchived(t *testing.T) {
	s := newServer(t)
	s.d.Store.ArchiveLocation(context.Background(), s.loc, time.Now())
	var body struct {
		Locations []struct {
			Archived bool `json:"archived"`
			Devices  []struct {
				Archived bool `json:"archived"`
			} `json:"devices"`
		} `json:"locations"`
	}
	decode(t, s.do("GET", "/api/catalog", nil, nil), &body)
	if len(body.Locations) != 1 || !body.Locations[0].Archived || !body.Locations[0].Devices[0].Archived {
		t.Fatalf("%+v", body)
	}
}

func TestStatsInvalidParams(t *testing.T) {
	s := newServer(t)
	w := s.do("GET", "/api/stats?from=abc", nil, nil)
	var e struct{ Error, Field string }
	decode(t, w, &e)
	if w.Code != 400 || e.Field != "from" || e.Error == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := s.do("GET", "/api/stats?from=0&to=99999999999999", nil, nil); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := s.do("GET", "/api/stats", nil, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := s.do("GET", "/api/report24", nil, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestTimelineUnknownDevice(t *testing.T) {
	s := newServer(t)
	for _, p := range []string{"/api/devices/999/timeline", "/api/devices/999/latency", "/api/devices/abc/timeline"} {
		w := s.do("GET", p, nil, nil)
		if w.Code != 404 && w.Code != 400 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
			t.Fatal(p, w.Code, w.Body.String())
		}
	}
	if w := s.do("GET", "/api/devices/"+itoa(s.dev[0])+"/timeline", nil, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := s.do("GET", "/api/devices/"+itoa(s.dev[0])+"/latency", nil, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := newServer(t)
	w := s.do("GET", "/api/state", nil, nil)
	want := "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
	if w.Header().Get("Content-Security-Policy") != want || w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatalf("%v", w.Header())
	}
}

func TestPanicRecovered(t *testing.T) {
	s := newServer(t, func(m *http.ServeMux, _ Deps) {
		m.HandleFunc("GET /api/test-panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	})
	w := s.do("GET", "/api/test-panic", nil, nil)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "error") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := s.do("GET", "/api/state", nil, nil); w.Code != 200 {
		t.Fatal("server broken after panic")
	}
}

func TestDecodeJSONStrict(t *testing.T) {
	type X struct {
		A int `json:"a"`
	}
	run := func(body string) (int, string) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		var x X
		err := decodeJSON(w, r, &x)
		var e *apperr.Error
		if !errors.As(err, &e) {
			if err == nil {
				return 0, ""
			}
			return -1, err.Error()
		}
		return e.Status, e.Field
	}
	if st, _ := run(`{"a":1}`); st != 0 {
		t.Fatal("valid rejected")
	}
	if st, f := run(`{"a":1,"b":2}`); st != 400 || f != "b" {
		t.Fatal("unknown field", st, f)
	}
	if st, f := run(`{"a":"x"}`); st != 400 || f != "a" {
		t.Fatal("type", st, f)
	}
	if st, _ := run(`{"a":`); st != 400 {
		t.Fatal("truncated")
	}
	if st, _ := run(`null`); st != 400 {
		t.Fatal("null")
	}
	if st, _ := run(``); st != 400 {
		t.Fatal("empty")
	}
	if st, _ := run(`{"a":1}{"a":2}`); st != 400 {
		t.Fatal("trailing")
	}
	if st, _ := run(`{"a":"` + strings.Repeat("x", 1<<20+10) + `"}`); st != 413 {
		t.Fatal("too large", st)
	}
}

func TestUnknownAPIRoute(t *testing.T) {
	s := newServer(t)
	w := s.do("GET", "/api/nimic", nil, nil)
	if w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatal(w.Code, w.Header())
	}
	if w := s.do("GET", "/", nil, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
