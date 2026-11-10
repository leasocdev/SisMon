package api

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAFallback(t *testing.T) {
	s := newServer(t)
	s.d.Web = fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>NetWatch</title>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	}
	s.h = build(s.d, nil)
	for _, p := range []string{"/", "/dispecerat", "/locatie/7", "/administrare/setari"} {
		w := s.do("GET", p, nil, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<title>NetWatch</title>") || w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal(p, w.Code, w.Header(), w.Body.String())
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("csp missing", p)
		}
	}
	w := s.do("GET", "/assets/app-1.js", nil, nil)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatal(w.Code, w.Header())
	}
	if w := s.do("GET", "/assets/lipsa.js", nil, nil); w.Code != 404 {
		t.Fatal("missing asset", w.Code)
	}
	if w := s.do("GET", "/assets/", nil, nil); w.Code != 404 && !strings.Contains(w.Body.String(), "<title>NetWatch") {
		t.Fatal("directory listing", w.Code, w.Body.String())
	}
	if w := s.do("GET", "/api/nimic", nil, nil); w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatal(w.Code, w.Header())
	}
}
