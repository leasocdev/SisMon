package api

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/settings"
)

func (a *admin) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.d.Settings.Get())
}

func (a *admin) putSettings(w http.ResponseWriter, r *http.Request) {
	var s settings.Settings
	if err := decodeJSON(w, r, &s); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Settings.Save(r.Context(), a.d.Store, s); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Store.Audit(r.Context(), a.d.Store.DB, "settings", "settings", 0, s, a.d.Now()); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Monitor.Reload(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, s)
}

func (a *admin) getIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := LoadIdentity(r.Context(), a.d.Store)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, id)
}

type identityBody struct {
	Name        string `json:"name"`
	Subtitle    string `json:"subtitle"`
	ReportTitle string `json:"report_title"`
}

func (a *admin) putIdentity(w http.ResponseWriter, r *http.Request) {
	var b identityBody
	if err := decodeJSON(w, r, &b); err != nil {
		writeError(w, err)
		return
	}
	cur, err := LoadIdentity(r.Context(), a.d.Store)
	if err != nil {
		writeError(w, err)
		return
	}
	cur.Name, cur.Subtitle, cur.ReportTitle = b.Name, b.Subtitle, b.ReportTitle
	if err := cur.Normalize(); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Store.SetMeta(r.Context(), "identity", cur); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Store.Audit(r.Context(), a.d.Store.DB, "identity", "identity", 0, cur, a.d.Now()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, cur)
}

type auditEntry struct {
	ID       int64  `json:"id"`
	TS       int64  `json:"ts"`
	Action   string `json:"action"`
	Entity   string `json:"entity"`
	EntityID *int64 `json:"entity_id"`
	Details  string `json:"details"`
}

func (a *admin) audit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, before := 50, int64(1<<62)
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, apperr.Invalid("limit", "Limita trebuie să fie între 1 și 200."))
			return
		}
		limit = n
	}
	if v := q.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			writeError(w, apperr.Invalid("before", "Identificator invalid."))
			return
		}
		before = n
	}
	rows, err := a.d.Store.DB.QueryContext(r.Context(), `SELECT id,ts,action,entity,entity_id,details_json FROM audit WHERE id<? ORDER BY id DESC LIMIT ?`, before, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rows.Close()
	out := []auditEntry{}
	for rows.Next() {
		var e auditEntry
		if err := rows.Scan(&e.ID, &e.TS, &e.Action, &e.Entity, &e.EntityID, &e.Details); err != nil {
			writeError(w, err)
			return
		}
		out = append(out, e)
	}
	writeJSON(w, 200, map[string]any{"entries": out})
}

func (a *admin) diagnostic(w http.ResponseWriter, r *http.Request) {
	h := a.d.Monitor.Health()
	var size int64
	if fi, err := os.Stat(a.d.Store.Path); err == nil {
		size = fi.Size()
	}
	writeJSON(w, 200, map[string]any{"health": h, "version": a.d.Version, "data_dir": a.d.DataDir, "db_size_bytes": size,
		"icmp_mode": h.ICMPMode, "now": a.d.Now().UnixMilli()})
}

func (a *admin) listBackups(w http.ResponseWriter, r *http.Request) {
	list, err := a.d.Backups.List()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"backups": list})
}

func (a *admin) createBackup(w http.ResponseWriter, r *http.Request) {
	info, err := a.d.Backups.Create(r.Context(), "manual")
	if err != nil {
		writeError(w, err)
		return
	}
	a.d.Store.Audit(r.Context(), a.d.Store.DB, "backup", "backup", 0, info, a.d.Now())
	writeJSON(w, 200, info)
}

func (a *admin) downloadBackup(w http.ResponseWriter, r *http.Request) {
	f, info, err := a.d.Backups.Open(r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Name))
	http.ServeContent(w, r, info.Name, time.UnixMilli(info.Created), f)
}
