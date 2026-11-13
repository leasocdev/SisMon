package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/export"
	"sismon/internal/stats"
)

var exportTypes = map[string]string{
	"csv":  "text/csv; charset=utf-8",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"pdf":  "application/pdf",
}

func attachment(w http.ResponseWriter, ext, name string, body []byte) {
	w.Header().Set("Content-Type", exportTypes[ext])
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(200)
	w.Write(body)
}

// exportMeta describes the export: identity, period and the filters by name.
func (p *public) exportMeta(r *http.Request, q stats.Query) (export.Meta, error) {
	id, err := LoadIdentity(r.Context(), p.d.Store)
	if err != nil {
		return export.Meta{}, err
	}
	// Reports are headed by the subtitle; the application name is the fallback.
	label := id.Subtitle
	if label == "" {
		label = id.Name
	}
	m := export.Meta{Title: id.ReportTitle, AppName: label, From: q.From, To: q.To, Generated: p.d.Now()}
	locs, err := p.d.Store.ListLocations(r.Context(), true)
	if err != nil {
		return m, err
	}
	locName, devName := "Toate", "Toate"
	for _, l := range locs {
		if q.LocationID != nil && l.ID == *q.LocationID {
			locName = l.Name
		}
		for _, d := range l.Devices {
			if q.DeviceID != nil && d.ID == *q.DeviceID {
				devName = l.Name + " · " + d.Name
			}
		}
	}
	kind := q.Kind
	if kind == "" {
		kind = "Toate"
	}
	inc := map[string]string{"all": "Toate", "active": "În curs", "closed": "Încheiate"}[q.Incidents]
	m.Filters = []export.Filter{{Label: "Locație", Value: locName}, {Label: "Echipament", Value: devName}, {Label: "Tip", Value: kind}, {Label: "Incidente", Value: inc},
		{Label: "Arhivate", Value: map[bool]string{true: "Incluse", false: "Excluse"}[q.Archived]},
		{Label: "Perioade degradate", Value: map[bool]string{true: "Incluse", false: "Excluse"}[q.Degraded]}}
	if p.d.Logo != nil {
		m.Logo = p.d.Logo()
	}
	return m, nil
}

// exportStats serves /api/export/{csv|xlsx|pdf} with the /api/stats parameters.
func (p *public) exportStats(w http.ResponseWriter, r *http.Request) {
	format := r.PathValue("format")
	if _, ok := exportTypes[format]; !ok {
		writeError(w, apperr.Invalid("format", "Formatul de export acceptă: csv, xlsx, pdf."))
		return
	}
	select {
	case p.d.exportSem <- struct{}{}:
	default:
		w.Header().Set("Retry-After", "5")
		writeError(w, &apperr.Error{Status: http.StatusServiceUnavailable, Message: "Un export este deja în curs; reîncercați peste câteva secunde."})
		return
	}
	now := p.d.Now()
	// The slot covers building only: a client that reads slowly must not hold it.
	buf, err := func() (*bytes.Buffer, error) {
		defer func() { <-p.d.exportSem }()
		return p.buildExport(r, format, now)
	}()
	if err != nil {
		writeError(w, err)
		return
	}
	name := fmt.Sprintf("sismon-statistici-%s.%s", now.In(export.Zone).Format("20060102-1504"), format)
	attachment(w, format, name, buf.Bytes())
}

// buildExport renders the statistics export for the request in memory.
func (p *public) buildExport(r *http.Request, format string, now time.Time) (*bytes.Buffer, error) {
	q, err := stats.ParseQuery(r.URL.Query(), now)
	if err != nil {
		return nil, err
	}
	rep, err := stats.Build(r.Context(), p.d.Store, q, now)
	if err != nil {
		return nil, err
	}
	m, err := p.exportMeta(r, q)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	switch format {
	case "csv":
		err = export.StatsCSV(&buf, rep, m)
	case "xlsx":
		err = export.StatsXLSX(&buf, rep, m)
	case "pdf":
		err = export.StatsPDF(&buf, rep, m)
	}
	return &buf, err
}

// exportLocations serves the active locations in the import format.
func (a *admin) exportLocations(w http.ResponseWriter, r *http.Request) {
	format := r.PathValue("format")
	if format != "csv" && format != "xlsx" {
		writeError(w, apperr.NotFound("Formatul cerut nu există."))
		return
	}
	locs, err := a.d.Store.ListLocations(r.Context(), false)
	if err != nil {
		writeError(w, err)
		return
	}
	var buf bytes.Buffer
	if format == "csv" {
		err = export.LocationsCSV(&buf, locs)
	} else {
		err = export.LocationsXLSX(&buf, locs)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	attachment(w, format, fmt.Sprintf("sismon-locatii-%s.%s", a.d.Now().In(export.Zone).Format("20060102"), format), buf.Bytes())
}

// exportLocationsFile maps /api/admin/export/locations.{csv|xlsx}.
func (a *admin) exportLocationsFile(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("file") {
	case "locations.csv":
		r.SetPathValue("format", "csv")
	case "locations.xlsx":
		r.SetPathValue("format", "xlsx")
	default:
		writeError(w, apperr.NotFound("Fișierul cerut nu există."))
		return
	}
	a.exportLocations(w, r)
}
