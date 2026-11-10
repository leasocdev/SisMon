package api

import (
	"bytes"
	"mime/multipart"
	"strings"
	"testing"
)

func multipartBody(t *testing.T, field, name string, data []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if field != "" {
		fw, err := mw.CreateFormFile(field, name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(data)
	} else {
		mw.WriteField("other", "x")
	}
	mw.Close()
	return buf.Bytes(), mw.FormDataContentType()
}

func (a *adminClient) upload(t *testing.T, path, field, name string, data []byte, csrf bool) (int, string) {
	t.Helper()
	body, ctype := multipartBody(t, field, name, data)
	h := map[string]string{"Cookie": a.cookie, "Origin": "http://nw.local", "Content-Type": ctype}
	if csrf {
		h["X-CSRF-Token"] = a.csrf
	}
	w := a.s.do("POST", path, body, h)
	return w.Code, w.Body.String()
}

const goodCSV = "Locație;Echipament;Host;Primar\nNord;Router;10.5.0.1;da\nNord;AP;10.5.0.2;nu\nSud;Router;10.6.0.1;da\n"

func TestImportPreviewAndApply(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	code, body := a.upload(t, "/api/admin/import/preview", "file", "l.csv", []byte(goodCSV), true)
	if code != 200 || !strings.Contains(body, `"valid":true`) || !strings.Contains(body, `"locations":2`) {
		t.Fatal(code, body)
	}
	reloads := s.mon.reloads
	code, body = a.upload(t, "/api/admin/import/apply", "file", "l.csv", []byte(goodCSV), true)
	if code != 200 || !strings.Contains(body, `"locations":2`) || !strings.Contains(body, `"devices":3`) {
		t.Fatal(code, body)
	}
	if s.mon.reloads != reloads+1 {
		t.Fatal("monitor not reloaded")
	}
	_, list := a.do("GET", "/api/admin/locations", nil, false)
	if !strings.Contains(list, `"Nord"`) || !strings.Contains(list, `"Sud"`) {
		t.Fatal(list)
	}
	_, audit := a.do("GET", "/api/admin/audit", nil, false)
	if !strings.Contains(audit, `"import"`) {
		t.Fatal(audit)
	}
}

func TestImportApplyRejectsInvalid(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	bad := goodCSV + "Vest;R;999.1.1.1;da\n"
	code, body := a.upload(t, "/api/admin/import/apply", "file", "l.csv", []byte(bad), true)
	if code != 400 || !strings.Contains(body, "Fișierul conține erori") || !strings.Contains(body, `"preview"`) {
		t.Fatal(code, body)
	}
	_, list := a.do("GET", "/api/admin/locations", nil, false)
	if strings.Contains(list, `"Nord"`) {
		t.Fatal("partial import")
	}
}

func TestImportApplyRechecksExisting(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	if code, _ := a.upload(t, "/api/admin/import/preview", "file", "l.csv", []byte(goodCSV), true); code != 200 {
		t.Fatal(code)
	}
	nord := map[string]any{"name": "Nord", "devices": []map[string]any{{"name": "R", "host": "10.9.0.1", "is_primary": true}}}
	if c, b := a.do("POST", "/api/admin/locations", nord, true); c != 200 {
		t.Fatal(c, b)
	}
	code, body := a.upload(t, "/api/admin/import/apply", "file", "l.csv", []byte(goodCSV), true)
	if code != 400 || !strings.Contains(body, "există deja") {
		t.Fatal(code, body)
	}
}

func TestImportLimits(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	if code, body := a.upload(t, "/api/admin/import/preview", "file", "big.csv", bytes.Repeat([]byte("a"), 6<<20), true); code != 413 || !strings.Contains(body, "5 MB") {
		t.Fatal(code, body)
	}
	if code, _ := a.upload(t, "/api/admin/import/preview", "file", "l.csv", []byte(goodCSV), false); code != 403 {
		t.Fatal("csrf", code)
	}
	body, ctype := multipartBody(t, "file", "l.csv", []byte(goodCSV))
	if w := s.do("POST", "/api/admin/import/preview", body, map[string]string{"Content-Type": ctype, "Origin": "http://nw.local"}); w.Code != 401 {
		t.Fatal("no session", w.Code)
	}
	if code, body := a.upload(t, "/api/admin/import/preview", "", "", nil, true); code != 400 || !strings.Contains(body, "Alegeți un fișier.") {
		t.Fatal(code, body)
	}
	if code, body := a.upload(t, "/api/admin/import/preview", "file", "x.xlsx", []byte("PK\x03\x04garbage"), true); code != 400 || !strings.Contains(body, `"field":"file"`) {
		t.Fatal(code, body)
	}
}
