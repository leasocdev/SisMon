package api

import (
	"net/http"
	"strconv"

	"netwatch/internal/apperr"
	"netwatch/internal/domain"
	"netwatch/internal/monitor/icmp"
)

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, apperr.Invalid("id", "Identificator invalid."))
		return 0, false
	}
	return id, true
}

func (a *admin) listLocations(w http.ResponseWriter, r *http.Request) {
	locs, err := a.d.Store.ListLocations(r.Context(), false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"locations": locs})
}

func (a *admin) saveLocation(w http.ResponseWriter, r *http.Request, id *int64) {
	var in domain.LocationInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	if err := in.Normalize(a.d.Settings.Get()); err != nil {
		writeError(w, err)
		return
	}
	res, err := a.d.Store.SaveLocation(r.Context(), id, in, a.d.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Monitor.Reload(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]int64{"id": res.LocationID})
}

func (a *admin) createLocation(w http.ResponseWriter, r *http.Request) { a.saveLocation(w, r, nil) }

func (a *admin) updateLocation(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		a.saveLocation(w, r, &id)
	}
}

func (a *admin) archiveLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := a.d.Store.ArchiveLocation(r.Context(), id, a.d.Now()); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Monitor.Reload(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

var codeMessages = map[icmp.Code]string{
	icmp.OK:              "Echipamentul răspunde.",
	icmp.Timeout:         "Fără răspuns în timpul stabilit.",
	icmp.HostUnreachable: "Echipamentul este inaccesibil.",
	icmp.NetUnreachable:  "Rețeaua sau ruta către echipament nu este accesibilă.",
	icmp.DNS:             "Numele DNS nu a putut fi rezolvat.",
	icmp.Permission:      "Monitorul nu are permisiunea de a trimite pachete ICMP.",
	icmp.Internal:        "Eroare internă a monitorului.",
}

type hostBody struct {
	Host string `json:"host"`
}

func (a *admin) testDevice(w http.ResponseWriter, r *http.Request) {
	var b hostBody
	if err := decodeJSON(w, r, &b); err != nil {
		writeError(w, err)
		return
	}
	res, err := a.d.Monitor.TestHost(r.Context(), b.Host)
	if err != nil {
		writeError(w, err)
		return
	}
	code := res.Code
	if res.OK() {
		code = icmp.OK
	}
	writeJSON(w, 200, map[string]any{"ok": res.OK(), "sent": res.Sent, "received": res.Received, "latency_ms": res.LatencyMs,
		"code": code, "message": codeMessages[code]})
}

type orderBody struct {
	IDs []int64 `json:"ids"`
}

func (a *admin) orderLocations(w http.ResponseWriter, r *http.Request) {
	var b orderBody
	if err := decodeJSON(w, r, &b); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Store.SetLocationOrder(r.Context(), b.IDs, a.d.Now()); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Monitor.Reload(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
