package api

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
)

func smallPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16)))
	return b.Bytes()
}

func TestLogoRoutes(t *testing.T) {
	s := newServer(t)
	s.d.DataDir = t.TempDir()
	s.h = build(s.d, nil)
	s.setPassword(t)
	a := login(t, s)
	if w := s.do("GET", "/branding/logo", nil, nil); w.Code != 404 {
		t.Fatal("no logo yet", w.Code)
	}
	if code, _ := a.upload(t, "/api/admin/identity/logo", "file", "l.png", smallPNG(t), false); code != 403 {
		t.Fatal("csrf", code)
	}
	code, body := a.upload(t, "/api/admin/identity/logo", "file", "l.png", smallPNG(t), true)
	if code != 200 || strings.Contains(body, `"logo_version":0`) {
		t.Fatal(code, body)
	}
	w := s.do("GET", "/branding/logo", nil, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "public, max-age=86400" {
		t.Fatal(w.Code, w.Header())
	}
	var st struct{ Identity Identity }
	decode(t, s.do("GET", "/api/state", nil, nil), &st)
	if st.Identity.LogoVersion == 0 {
		t.Fatal("state identity")
	}
	if code, body := a.upload(t, "/api/admin/identity/logo", "file", "l.png", []byte("text"), true); code != 400 || !strings.Contains(body, "PNG") {
		t.Fatal(code, body)
	}
	if c, b := a.do("DELETE", "/api/admin/identity/logo", nil, true); c != 200 || !strings.Contains(b, `"logo_version":0`) {
		t.Fatal(c, b)
	}
	if w := s.do("GET", "/branding/logo", nil, nil); w.Code != 404 {
		t.Fatal("deleted", w.Code)
	}
	_, audit := a.do("GET", "/api/admin/audit", nil, false)
	if !strings.Contains(audit, `"logo"`) || !strings.Contains(audit, `"logo_delete"`) {
		t.Fatal(audit)
	}
}

func TestLogoInPDFExport(t *testing.T) {
	s := newServer(t)
	s.d.Logo = func() []byte { return smallPNG(t) }
	s.h = build(s.d, nil)
	w := s.do("GET", "/api/export/pdf", nil, nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("/Subtype /Image")) {
		t.Fatal(w.Code)
	}
}
