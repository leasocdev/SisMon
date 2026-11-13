package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"sismon/internal/live"
	"sismon/internal/state"
)

type sseMsg struct{ id, event, data string }

type sseClient struct {
	cancel context.CancelFunc
	resp   *http.Response
	lines  chan string
}

func sseID(h *live.Hub, seq uint64) string { return fmt.Sprintf("%d-%d", h.Epoch(), seq) }

func openSSE(t *testing.T, url string, lastID string) *sseClient {
	return openSSEPath(t, url+"/api/events", lastID)
}

func openSSEPath(t *testing.T, full string, lastID string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", full, nil)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	c := &sseClient{cancel: cancel, resp: resp, lines: make(chan string, 1000)}
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			c.lines <- sc.Text()
		}
		close(c.lines)
	}()
	t.Cleanup(func() { cancel(); resp.Body.Close() })
	return c
}

// next returns the next complete message (or a ping comment as event ":ping").
func (c *sseClient) next(t *testing.T) sseMsg {
	t.Helper()
	var m sseMsg
	timeout := time.After(2 * time.Second)
	for {
		select {
		case l, ok := <-c.lines:
			if !ok {
				t.Fatal("stream closed")
			}
			switch {
			case strings.HasPrefix(l, ": ping"):
				return sseMsg{event: ":ping"}
			case strings.HasPrefix(l, "id: "):
				m.id = l[4:]
			case strings.HasPrefix(l, "event: "):
				m.event = l[7:]
			case strings.HasPrefix(l, "data: "):
				m.data = l[6:]
			case l == "" && m.event != "":
				return m
			}
		case <-timeout:
			t.Fatal("timeout waiting for SSE message")
		}
	}
}

func TestEventsHeadersAndFlush(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	c := openSSE(t, srv.URL, "")
	h := c.resp.Header
	if h.Get("Content-Type") != "text/event-stream" || h.Get("Cache-Control") != "no-cache" || h.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("%v", h)
	}
	if m := c.next(t); m.event != "snapshot" || !strings.Contains(m.data, `"locations"`) {
		t.Fatalf("%+v", m)
	}
}

func TestEventsReceivesStateChange(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	c := openSSE(t, srv.URL, "")
	c.next(t)
	s.d.Hub.SetDevice(live.DeviceView{ID: s.dev[0], LocationID: s.loc, State: state.Down})
	m := c.next(t)
	if m.event != "state" || !strings.Contains(m.data, `"state":"down"`) || m.id == "" {
		t.Fatalf("%+v", m)
	}
}

func TestEventsResumeLastEventID(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	n := s.d.Hub.Snapshot().Seq
	s.d.Hub.SetHealth(live.Health{AvgCheckMs: 1})
	s.d.Hub.SetHealth(live.Health{AvgCheckMs: 2})
	c := openSSE(t, srv.URL, sseID(s.d.Hub, n))
	m1, m2 := c.next(t), c.next(t)
	if m1.event != "health" || m1.id != sseID(s.d.Hub, n+1) || m2.id != sseID(s.d.Hub, n+2) {
		t.Fatalf("%+v %+v", m1, m2)
	}
	stale := openSSE(t, srv.URL, sseID(s.d.Hub, 999999))
	if m := stale.next(t); m.event != "snapshot" {
		t.Fatalf("unknown id should yield snapshot: %+v", m)
	}
}

func TestSSEPingIsNamedEvent(t *testing.T) {
	s := newServer(t)
	s.d.SSEPing = 20 * time.Millisecond
	srv := httptest.NewServer(build(s.d, nil))
	t.Cleanup(srv.Close)
	c := openSSE(t, srv.URL, "")
	c.next(t)
	m := c.next(t)
	if m.event != "ping" || m.id != "" {
		t.Fatalf("%+v", m)
	}
	var p struct {
		Now int64 `json:"now"`
	}
	if err := json.Unmarshal([]byte(m.data), &p); err != nil || p.Now <= 0 {
		t.Fatalf("%q %v", m.data, err)
	}
}

func TestEventsClientDisconnect(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	c := openSSE(t, srv.URL, "")
	c.next(t)
	if s.d.Hub.Subscribers() != 1 {
		t.Fatal(s.d.Hub.Subscribers())
	}
	c.cancel()
	deadline := time.Now().Add(2 * time.Second)
	for s.d.Hub.Subscribers() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("subscriber not released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSSEIDsCarryEpoch(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	m := openSSE(t, srv.URL, "").next(t)
	snap := s.d.Hub.Snapshot()
	if m.event != "snapshot" || m.id != sseID(s.d.Hub, snap.Seq) || !strings.Contains(m.data, fmt.Sprintf(`"epoch":%d`, s.d.Hub.Epoch())) {
		t.Fatalf("%+v", m)
	}
	if s.d.Hub.Epoch() <= 0 {
		t.Fatal("epoch")
	}
}

func TestSSEResumeByQuery(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	s.d.Hub.SetHealth(live.Health{AvgCheckMs: 1})
	seq1 := s.d.Hub.Snapshot().Seq
	s.d.Hub.SetHealth(live.Health{AvgCheckMs: 2})
	c := openSSEPath(t, srv.URL+"/api/events?last="+sseID(s.d.Hub, seq1), "")
	if m := c.next(t); m.event != "health" || m.id != sseID(s.d.Hub, seq1+1) {
		t.Fatalf("%+v", m)
	}
}

func TestSSEEpochMismatchSendsSnapshot(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	s.d.Hub.SetHealth(live.Health{AvgCheckMs: 1})
	if m := openSSE(t, srv.URL, "1-1").next(t); m.event != "snapshot" {
		t.Fatalf("%+v", m)
	}
}

func TestSSEMalformedLastIDSendsSnapshot(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	c := openSSEPath(t, srv.URL+"/api/events?last=abc", "")
	if c.resp.StatusCode != 200 {
		t.Fatal(c.resp.StatusCode)
	}
	if m := c.next(t); m.event != "snapshot" {
		t.Fatalf("%+v", m)
	}
}

func TestSSEResumeBeyondReplayLimitSendsSnapshot(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	first := s.d.Hub.Snapshot().Seq
	for i := 0; i < live.ReplayLimit+20; i++ {
		s.d.Hub.SetHealth(live.Health{AvgCheckMs: float64(i)})
	}
	if m := openSSE(t, srv.URL, sseID(s.d.Hub, first)).next(t); m.event != "snapshot" {
		t.Fatalf("a resume that cannot be replayed fully must get a snapshot: %+v", m)
	}
}

// Review fix: event streams are capped per client IP and in total.
func TestEventsConnectionCap(t *testing.T) {
	s := newServer(t)
	s.d.sse = newSSECap(3, 2)
	s.h = build(s.d, nil)
	ip := netip.MustParseAddr("192.0.2.10")
	s.d.sse.acquire(ip)
	s.d.sse.acquire(ip) // this client already holds 2 streams
	if w := s.do("GET", "/api/events", nil, nil); w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Fatal("per-IP cap:", w.Code, w.Body.String())
	}
	s.d.sse.acquire(netip.MustParseAddr("192.0.2.11")) // total 3
	if w := s.doFrom("192.0.2.12:5000", "GET", "/api/events", nil, nil); w.Code != 503 {
		t.Fatal("total cap:", w.Code)
	}
	s.d.sse.release(ip)
	if !s.d.sse.acquire(ip) {
		t.Fatal("slot not released")
	}
}

// Review fix: open pages (the dispatcher) learn about settings and identity
// changes through a "config" event and refetch /api/state.
func TestConfigChangePublishesEvent(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	last := s.d.Hub.Snapshot().Seq
	if c, b := a.do("PUT", "/api/admin/identity", map[string]any{"name": "Rețea Nord", "subtitle": "", "report_title": "Raport"}, true); c != 200 {
		t.Fatal(c, b)
	}
	set := map[string]any{"interval_s": 60, "packets": 3, "timeout_s": 2, "failures": 3, "recoveries": 2, "stale_s": 300, "incidents_for": "all", "prefer_ipv4": true, "backup_hour": 3, "backup_secondary_dir": ""}
	if c, b := a.do("PUT", "/api/admin/settings", set, true); c != 200 {
		t.Fatal(c, b)
	}
	evs, _ := s.d.Hub.Since(last)
	n := 0
	for _, e := range evs {
		if e.Type == "config" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("config events %d, want 2: %+v", n, evs)
	}
}
