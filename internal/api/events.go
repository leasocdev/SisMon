package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"netwatch/internal/live"
)

func writeEvent(w http.ResponseWriter, epoch int64, seq uint64, typ string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d-%d\nevent: %s\ndata: %s\n\n", epoch, seq, typ, b)
	return err
}

// writePing sends a named keep-alive without an id, so the client's
// Last-Event-ID is unchanged but it still sees traffic.
func writePing(w http.ResponseWriter, now time.Time) error {
	_, err := fmt.Fprintf(w, "event: ping\ndata: {\"now\":%d}\n\n", now.UnixMilli())
	return err
}

// parseEventID reads "epoch-seq"; ok is false for a foreign epoch or bad format.
func parseEventID(s string, epoch int64) (uint64, bool) {
	e, q, found := strings.Cut(s, "-")
	if !found {
		return 0, false
	}
	ev, err1 := strconv.ParseInt(e, 10, 64)
	seq, err2 := strconv.ParseUint(q, 10, 64)
	if err1 != nil || err2 != nil || ev != epoch || seq == 0 {
		return 0, false
	}
	return seq, true
}

// events streams live changes as Server-Sent Events.
func (p *public) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming nesuportat", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	h.Set("Connection", "keep-alive")
	// SSE responses are long-lived: lift the server write deadline if any.
	http.NewResponseController(w).SetWriteDeadline(time.Time{})

	hub := p.d.Hub
	epoch := hub.Epoch()
	lastRaw := r.Header.Get("Last-Event-ID")
	if lastRaw == "" {
		lastRaw = r.URL.Query().Get("last")
	}
	last, okID := parseEventID(lastRaw, epoch)
	var ch <-chan live.Event
	var cancel func()
	if past, ok := hub.Since(last); okID && ok && len(past) < live.ReplayLimit {
		ch, cancel = hub.Subscribe(last)
		w.WriteHeader(http.StatusOK)
	} else {
		snap := hub.Snapshot()
		ch, cancel = hub.Subscribe(snap.Seq)
		w.WriteHeader(http.StatusOK)
		if err := writeEvent(w, epoch, snap.Seq, "snapshot", snap); err != nil {
			cancel()
			return
		}
	}
	defer cancel()
	fl.Flush()

	ping := time.NewTicker(p.d.SSEPing)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if err := writePing(w, p.d.Now()); err != nil {
				return
			}
			fl.Flush()
		case e, ok := <-ch:
			if !ok {
				return // fell behind: the client reconnects and gets a snapshot
			}
			if err := writeEvent(w, epoch, e.Seq, e.Type, e.Data); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
