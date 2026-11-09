package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"netwatch/internal/live"
	"netwatch/internal/state"
)

type sseMsg struct{ id, event, data string }

type sseClient struct {
	cancel context.CancelFunc
	resp   *http.Response
	lines  chan string
}

func openSSE(t *testing.T, url string, lastID string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/events", nil)
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
	c := openSSE(t, srv.URL, strconv.FormatUint(n, 10))
	m1, m2 := c.next(t), c.next(t)
	if m1.event != "health" || m1.id != strconv.FormatUint(n+1, 10) || m2.id != strconv.FormatUint(n+2, 10) {
		t.Fatalf("%+v %+v", m1, m2)
	}
	stale := openSSE(t, srv.URL, "999999")
	if m := stale.next(t); m.event != "snapshot" {
		t.Fatalf("unknown id should yield snapshot: %+v", m)
	}
}

func TestEventsPing(t *testing.T) {
	s := newServer(t)
	s.d.SSEPing = 50 * time.Millisecond
	srv := httptest.NewServer(build(s.d, nil))
	t.Cleanup(srv.Close)
	c := openSSE(t, srv.URL, "")
	c.next(t)
	if m := c.next(t); m.event != ":ping" {
		t.Fatalf("%+v", m)
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
