package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"netwatch/internal/live"
)

func writeEvent(w http.ResponseWriter, seq uint64, typ string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", seq, typ, b)
	return err
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
	last, _ := strconv.ParseUint(r.Header.Get("Last-Event-ID"), 10, 64)
	var ch <-chan live.Event
	var cancel func()
	if _, ok := hub.Since(last); last > 0 && ok {
		ch, cancel = hub.Subscribe(last)
		w.WriteHeader(http.StatusOK)
	} else {
		snap := hub.Snapshot()
		ch, cancel = hub.Subscribe(snap.Seq)
		w.WriteHeader(http.StatusOK)
		if err := writeEvent(w, snap.Seq, "snapshot", snap); err != nil {
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
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		case e, ok := <-ch:
			if !ok {
				return // fell behind: the client reconnects and gets a snapshot
			}
			if err := writeEvent(w, e.Seq, e.Type, e.Data); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
