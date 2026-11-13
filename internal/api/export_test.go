package api

import (
	"strings"
	"testing"
)

func TestExportRoutes(t *testing.T) {
	s := newServer(t)
	cases := map[string]string{
		"csv":  "text/csv; charset=utf-8",
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"pdf":  "application/pdf",
	}
	for f, ctype := range cases {
		w := s.do("GET", "/api/export/"+f+"?location="+itoa(s.loc), nil, nil)
		if w.Code != 200 || w.Header().Get("Content-Type") != ctype {
			t.Fatal(f, w.Code, w.Header(), w.Body.String())
		}
		cd := w.Header().Get("Content-Disposition")
		if !strings.Contains(cd, `attachment; filename="sismon-statistici-`) || !strings.HasSuffix(cd, "."+f+`"`) {
			t.Fatal(cd)
		}
	}
	csv := s.do("GET", "/api/export/csv?location="+itoa(s.loc), nil, nil).Body.String()
	if !strings.Contains(csv, "Locație;Centru") {
		t.Fatalf("filter shown by name: %s", csv[:300])
	}
	if w := s.do("GET", "/api/export/doc", nil, nil); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := s.do("GET", "/api/export/csv?from=abc", nil, nil); w.Code != 400 || !strings.Contains(w.Body.String(), `"field":"from"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestExportLocationsAdmin(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	if w := s.do("GET", "/api/admin/export/locations.csv", nil, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	a := login(t, s)
	code, body := a.do("GET", "/api/admin/export/locations.csv", nil, false)
	if code != 200 || !strings.Contains(body, "Locație;Adresă;Latitudine;Longitudine;Echipament;Host;Tip;Primar;Observații") || !strings.Contains(body, "Centru;") {
		t.Fatal(code, body)
	}
	if code, _ := a.do("GET", "/api/admin/export/locations.xlsx", nil, false); code != 200 {
		t.Fatal(code)
	}
	if code, _ := a.do("GET", "/api/admin/export/locations.pdf", nil, false); code != 404 && code != 400 {
		t.Fatal(code)
	}
}

func TestExportConcurrencyLimit(t *testing.T) {
	s := newServer(t)
	s.d.exportSem = make(chan struct{}, 1)
	s.d.exportSem <- struct{}{} // an export is already running
	s.h = build(s.d, nil)
	w := s.do("GET", "/api/export/pdf", nil, nil)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "Un export este deja în curs") || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
}

// Review fix: the export slot is released once the file is built, so a
// client that never reads the response cannot block other exports.
func TestExportReleasesSlotBeforeWriting(t *testing.T) {
	s := newServer(t)
	s.d.exportSem = make(chan struct{}, 1)
	s.h = build(s.d, nil)
	for i := 0; i < 3; i++ {
		if w := s.do("GET", "/api/export/csv", nil, nil); w.Code != 200 {
			t.Fatal(i, w.Code, w.Body.String())
		}
	}
	if n := len(s.d.exportSem); n != 0 {
		t.Fatalf("export slot still held: %d", n)
	}
}
