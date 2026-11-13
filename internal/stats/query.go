// Package stats computes durations, availability, incidents and chart series.
package stats

import (
	"math"
	"net/url"
	"strconv"
	"time"

	"sismon/internal/apperr"
)

// MaxPeriod is the longest period a report may cover.
const MaxPeriod = 400 * 24 * time.Hour

// Query selects a report.
type Query struct {
	From, To   time.Time
	LocationID *int64
	DeviceID   *int64
	Kind       string
	Incidents  string // all | active | closed
	// Archived includes archived locations and devices (parameter archived=1).
	Archived bool
	// Degraded adds the degraded periods to the location outages (degraded=1).
	Degraded bool
}

func parseMs(v url.Values, key string) (*time.Time, error) {
	s := v.Get(key)
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, apperr.Invalid(key, "Dată invalidă (milisecunde Unix așteptate).")
	}
	t := time.UnixMilli(int64(f))
	return &t, nil
}

func parseID(v url.Values, key string) (*int64, error) {
	s := v.Get(key)
	if s == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return nil, apperr.Invalid(key, "Identificator invalid.")
	}
	return &id, nil
}

// ParseQuery reads a Query from URL parameters. Defaults: the last 24 hours.
func ParseQuery(v url.Values, now time.Time) (Query, error) {
	q := Query{To: now, Kind: v.Get("kind"), Incidents: v.Get("incidents")}
	to, err := parseMs(v, "to")
	if err != nil {
		return q, err
	}
	if to != nil && to.Before(now) {
		q.To = *to
	}
	q.From = q.To.Add(-24 * time.Hour)
	from, err := parseMs(v, "from")
	if err != nil {
		return q, err
	}
	if from != nil {
		q.From = *from
	}
	if !q.To.After(q.From) {
		return q, apperr.Invalid("to", "Sfârșitul perioadei trebuie să fie după început.")
	}
	if q.To.Sub(q.From) > MaxPeriod {
		return q, apperr.Invalid("from", "Perioada poate avea cel mult 400 de zile.")
	}
	if q.LocationID, err = parseID(v, "location"); err != nil {
		return q, err
	}
	if q.DeviceID, err = parseID(v, "device"); err != nil {
		return q, err
	}
	switch v.Get("archived") {
	case "", "0":
	case "1":
		q.Archived = true
	default:
		return q, apperr.Invalid("archived", "Parametrul archived acceptă: 0, 1.")
	}
	switch v.Get("degraded") {
	case "", "0":
	case "1":
		q.Degraded = true
	default:
		return q, apperr.Invalid("degraded", "Parametrul degraded acceptă: 0, 1.")
	}
	switch q.Incidents {
	case "":
		q.Incidents = "all"
	case "all", "active", "closed":
	default:
		return q, apperr.Invalid("incidents", "Filtrul de incidente acceptă: all, active, closed.")
	}
	return q, nil
}

// ReasonText translates an incident end reason.
func ReasonText(code string) string {
	switch code {
	case "recovered":
		return "Revenire"
	case "monitoring_gap":
		return "Lipsă monitorizare"
	case "monitor_error":
		return "Eroare monitor"
	case "configuration":
		return "Configurație modificată"
	case "archived":
		return "Arhivat"
	case "disabled":
		return "Echipament dezactivat"
	case "":
		return "În curs"
	}
	return code
}
