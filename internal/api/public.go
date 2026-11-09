package api

import (
	"net/http"
	"strconv"

	"netwatch/internal/apperr"
	"netwatch/internal/live"
	"netwatch/internal/stats"
	"netwatch/internal/store"
)

type public struct{ d Deps }

type stateResponse struct {
	live.Snapshot
	Identity  Identity `json:"identity"`
	IntervalS int      `json:"interval_s"`
	Version   string   `json:"version"`
	Now       int64    `json:"now"`
}

func (p *public) state(w http.ResponseWriter, r *http.Request) {
	id, err := LoadIdentity(r.Context(), p.d.Store)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, stateResponse{Snapshot: p.d.Hub.Snapshot(), Identity: id, IntervalS: p.d.Settings.Get().IntervalS,
		Version: p.d.Version, Now: p.d.Now().UnixMilli()})
}

type catalogDevice struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Kind     string `json:"kind"`
	Primary  bool   `json:"is_primary"`
	Archived bool   `json:"archived"`
}

type catalogLocation struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Archived bool            `json:"archived"`
	Devices  []catalogDevice `json:"devices"`
}

func (p *public) catalog(w http.ResponseWriter, r *http.Request) {
	locs, err := p.d.Store.ListLocations(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]catalogLocation, 0, len(locs))
	for _, l := range locs {
		c := catalogLocation{ID: l.ID, Name: l.Name, Archived: l.ArchivedAt != nil, Devices: []catalogDevice{}}
		for _, d := range l.Devices {
			c.Devices = append(c.Devices, catalogDevice{d.ID, d.Name, d.Host, d.Kind, d.Primary, d.ArchivedAt != nil})
		}
		out = append(out, c)
	}
	writeJSON(w, 200, map[string]any{"locations": out})
}

func (p *public) stats(w http.ResponseWriter, r *http.Request) {
	now := p.d.Now()
	q, err := stats.ParseQuery(r.URL.Query(), now)
	if err != nil {
		writeError(w, err)
		return
	}
	rep, err := stats.Build(r.Context(), p.d.Store, q, now)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, rep)
}

func (p *public) deviceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, apperr.Invalid("id", "Identificator de echipament invalid."))
		return 0, false
	}
	var n int
	if err := p.d.Store.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM devices WHERE id=?`, id).Scan(&n); err != nil {
		writeError(w, err)
		return 0, false
	}
	if n == 0 {
		writeError(w, apperr.NotFound("Echipamentul nu există."))
		return 0, false
	}
	return id, true
}

func (p *public) timeline(w http.ResponseWriter, r *http.Request) {
	id, ok := p.deviceID(w, r)
	if !ok {
		return
	}
	now := p.d.Now()
	q, err := stats.ParseQuery(r.URL.Query(), now)
	if err != nil {
		writeError(w, err)
		return
	}
	segs, err := stats.Timeline(r.Context(), p.d.Store, id, q.From, q.To, now)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"from": store.Ms(q.From), "to": store.Ms(q.To), "segments": segs})
}

func (p *public) latency(w http.ResponseWriter, r *http.Request) {
	id, ok := p.deviceID(w, r)
	if !ok {
		return
	}
	q, err := stats.ParseQuery(r.URL.Query(), p.d.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	pts, err := stats.Latency(r.Context(), p.d.Store, id, q.From, q.To)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"from": store.Ms(q.From), "to": store.Ms(q.To), "points": pts})
}

func (p *public) report24(w http.ResponseWriter, r *http.Request) {
	rep, err := stats.BuildReport24(r.Context(), p.d.Store, p.d.Hub.Snapshot(), p.d.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, rep)
}
