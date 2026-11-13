package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"sismon/internal/auth"
)

const testPassword = "parola-de-test-123"

func (s *testServer) setPassword(t *testing.T) {
	t.Helper()
	h, _ := auth.HashPassword(testPassword)
	if err := s.d.Store.SetMeta(context.Background(), "password_hash", h); err != nil {
		t.Fatal(err)
	}
}

type adminClient struct {
	s      *testServer
	cookie string
	csrf   string
}

var origin = map[string]string{"Origin": "http://nw.local"}

func login(t *testing.T, s *testServer) *adminClient {
	t.Helper()
	w := s.do("POST", "/api/admin/login", map[string]string{"password": testPassword}, origin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var body struct{ CSRF string }
	decode(t, w, &body)
	var cookie string
	for _, c := range w.Result().Cookies() {
		if c.Name == "nw_session" {
			cookie = c.Name + "=" + c.Value
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Secure {
				t.Fatalf("cookie flags %+v", c)
			}
		}
	}
	if cookie == "" || body.CSRF == "" {
		t.Fatal("no session")
	}
	return &adminClient{s, cookie, body.CSRF}
}

func (a *adminClient) do(method, path string, body any, csrf bool) (int, string) {
	h := map[string]string{"Cookie": a.cookie, "Origin": "http://nw.local"}
	if csrf {
		h["X-CSRF-Token"] = a.csrf
	}
	w := a.s.do(method, path, body, h)
	return w.Code, w.Body.String()
}

var newLoc = map[string]any{"name": "Nord", "devices": []map[string]any{{"name": "R", "host": "10.1.0.1", "is_primary": true}}}

func TestLoginFlow(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	if w := s.do("GET", "/api/admin/locations", nil, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := s.do("POST", "/api/admin/login", map[string]string{"password": "gresit-gresit"}, origin); w.Code != 401 {
		t.Fatal(w.Code)
	}
	a := login(t, s)
	if c, _ := a.do("POST", "/api/admin/locations", newLoc, false); c != 403 {
		t.Fatal("missing CSRF accepted", c)
	}
	if c, b := a.do("POST", "/api/admin/locations", newLoc, true); c != 200 || !strings.Contains(b, `"id"`) {
		t.Fatal(c, b)
	}
	if s.mon.reloads != 1 {
		t.Fatal("Reload not called")
	}
	if c, _ := a.do("GET", "/api/admin/locations", nil, false); c != 200 {
		t.Fatal(c)
	}
	if c, _ := a.do("POST", "/api/admin/logout", map[string]any{}, true); c != 200 {
		t.Fatal(c)
	}
	if c, _ := a.do("GET", "/api/admin/locations", nil, false); c != 401 {
		t.Fatal("session survived logout", c)
	}
}

func TestLoginRequiresSameOrigin(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	if w := s.do("POST", "/api/admin/login", map[string]string{"password": testPassword}, map[string]string{"Origin": "http://evil"}); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := s.do("POST", "/api/admin/login", map[string]string{"password": testPassword}, nil); w.Code != 403 {
		t.Fatal("no origin accepted", w.Code)
	}
}

func TestLoginBehindTrustedProxy(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	req := map[string]string{"Origin": "https://nw.example", "X-Forwarded-Proto": "https", "X-Forwarded-Host": "nw.example", "X-Forwarded-For": "192.0.2.77"}
	w := s.doFrom("10.0.0.5:3333", "POST", "/api/admin/login", map[string]string{"password": testPassword}, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	secure := false
	for _, c := range w.Result().Cookies() {
		secure = secure || (c.Name == "nw_session" && c.Secure)
	}
	if !secure {
		t.Fatal("cookie not Secure behind HTTPS proxy")
	}
}

func TestLoginRateLimit(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	for i := 0; i < 5; i++ {
		if w := s.do("POST", "/api/admin/login", map[string]string{"password": "gresit-gresit"}, origin); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	w := s.do("POST", "/api/admin/login", map[string]string{"password": testPassword}, origin)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Header())
	}
}

func TestSetupOnlyLoopback(t *testing.T) {
	s := newServer(t)
	body := map[string]string{"password": testPassword}
	lo := map[string]string{"Host": "localhost:8501", "Origin": "http://localhost:8501"}
	w := s.doFrom("127.0.0.1:5000", "GET", "/api/admin/session", nil, map[string]string{"Host": "localhost:8501"})
	if !strings.Contains(w.Body.String(), `"setup_allowed":true`) || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatal(w.Body.String())
	}
	if w := s.doFrom("192.0.2.5:5000", "POST", "/api/admin/setup", body, lo); w.Code != 403 {
		t.Fatal("remote setup allowed", w.Code)
	}
	if w := s.doFrom("127.0.0.1:5000", "POST", "/api/admin/setup", map[string]string{"password": "scurta"}, lo); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := s.doFrom("127.0.0.1:5000", "POST", "/api/admin/setup", body, lo); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := s.doFrom("127.0.0.1:5000", "POST", "/api/admin/setup", body, lo); w.Code != 403 {
		t.Fatal("setup twice", w.Code)
	}
	login(t, s)
}

func TestPasswordChangeInvalidatesSessions(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a, b := login(t, s), login(t, s)
	if c, body := a.do("PUT", "/api/admin/password", map[string]string{"old": "gresit-gresit", "new": "parola-noua-12345"}, true); c != 400 {
		t.Fatal(c, body)
	}
	if c, body := a.do("PUT", "/api/admin/password", map[string]string{"old": testPassword, "new": "parola-noua-12345"}, true); c != 200 {
		t.Fatal(c, body)
	}
	if c, _ := a.do("GET", "/api/admin/locations", nil, false); c != 401 {
		t.Fatal(c)
	}
	if c, _ := b.do("GET", "/api/admin/locations", nil, false); c != 401 {
		t.Fatal(c)
	}
}

func TestValidationErrorsAre400(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	for _, body := range []any{`{"name":5}`, `{"devices":"x"}`, ``, `null`, `{"name":"X","devices":[]}`, `{"nume":"x"}`} {
		if c, b := a.do("POST", "/api/admin/locations", body, true); c != 400 {
			t.Errorf("%v → %d %s", body, c, b)
		}
	}
	if c, _ := a.do("PUT", "/api/admin/locations/999", newLoc, true); c != 404 {
		t.Fatal(c)
	}
	if c, _ := a.do("PUT", "/api/admin/locations/abc", newLoc, true); c != 400 {
		t.Fatal(c)
	}
}

func TestEditAndArchiveLocation(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	id := strconv.FormatInt(s.loc, 10)
	dev := s.dev[0]
	body := map[string]any{"name": "Centru nou", "devices": []map[string]any{{"id": dev, "name": "Router", "host": "10.0.0.1", "is_primary": true}}}
	if c, b := a.do("PUT", "/api/admin/locations/"+id, body, true); c != 200 {
		t.Fatal(c, b)
	}
	if c, _ := a.do("DELETE", "/api/admin/locations/"+id, nil, true); c != 200 {
		t.Fatal(c)
	}
	if c, _ := a.do("DELETE", "/api/admin/locations/"+id, nil, true); c != 404 {
		t.Fatal(c)
	}
}

func TestDeleteLocationPermanent(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	id := strconv.FormatInt(s.loc, 10)
	if c, _ := a.do("DELETE", "/api/admin/locations/"+id+"/permanent", nil, false); c != 403 {
		t.Fatal("csrf", c)
	}
	if c, b := a.do("DELETE", "/api/admin/locations/"+id+"/permanent", nil, true); c != 200 {
		t.Fatal(c, b)
	}
	if c, _ := a.do("DELETE", "/api/admin/locations/"+id+"/permanent", nil, true); c != 404 {
		t.Fatal(c)
	}
	if c, _ := a.do("DELETE", "/api/admin/locations/abc/permanent", nil, true); c != 400 {
		t.Fatal(c)
	}
	if _, b := a.do("GET", "/api/catalog", nil, false); strings.Contains(b, `"id":`+id+`,`) {
		t.Fatal("deleted location still in catalog:", b)
	}
}

func TestArchivedLocationsList(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	if c, b := a.do("GET", "/api/admin/locations/archived", nil, false); c != 200 || !strings.Contains(b, `"locations":[]`) {
		t.Fatal(c, b)
	}
	id := strconv.FormatInt(s.loc, 10)
	if c, _ := a.do("DELETE", "/api/admin/locations/"+id, nil, true); c != 200 {
		t.Fatal(c)
	}
	c, b := a.do("GET", "/api/admin/locations/archived", nil, false)
	if c != 200 || !strings.Contains(b, `"id":`+id+`,`) || !strings.Contains(b, `"archived_at":`) || !strings.Contains(b, `"devices":`) {
		t.Fatal(c, b)
	}
	if _, b := a.do("GET", "/api/admin/locations", nil, false); strings.Contains(b, `"id":`+id+`,`) {
		t.Fatal("archived location in the active list:", b)
	}
	if c, _ := a.do("DELETE", "/api/admin/locations/"+id+"/permanent", nil, true); c != 200 {
		t.Fatal(c)
	}
	if _, b := a.do("GET", "/api/admin/locations/archived", nil, false); !strings.Contains(b, `"locations":[]`) {
		t.Fatal(b)
	}
}

func TestDeviceTestEndpoint(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	l := 2.5
	s.mon.result.LatencyMs = &l
	if c, b := a.do("POST", "/api/admin/devices/test", map[string]string{"host": "10.0.0.9"}, true); c != 200 || !strings.Contains(b, `"ok":true`) || !strings.Contains(b, `"latency_ms":2.5`) || !strings.Contains(b, `"message"`) {
		t.Fatal(c, b)
	}
	if c, _ := a.do("POST", "/api/admin/devices/test", map[string]string{"host": "a b"}, true); c != 400 {
		t.Fatal(c)
	}
}

func TestSettingsPutValidation(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	c, body := a.do("GET", "/api/admin/settings", nil, false)
	if c != 200 || !strings.Contains(body, `"interval_s":60`) {
		t.Fatal(c, body)
	}
	bad := map[string]any{"interval_s": 60, "packets": 3, "timeout_s": 2, "failures": 3, "recoveries": 2, "stale_s": 50, "incidents_for": "all", "prefer_ipv4": true, "backup_hour": 3, "backup_secondary_dir": ""}
	if c, b := a.do("PUT", "/api/admin/settings", bad, true); c != 400 || !strings.Contains(b, `"field":"stale_s"`) {
		t.Fatal(c, b)
	}
	bad["stale_s"] = 600
	if c, b := a.do("PUT", "/api/admin/settings", bad, true); c != 200 || s.d.Settings.Get().StaleS != 600 {
		t.Fatal(c, b)
	}
}

func TestIdentity(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	if c, b := a.do("PUT", "/api/admin/identity", map[string]any{"name": "", "subtitle": "", "report_title": "R"}, true); c != 400 || !strings.Contains(b, `"field":"name"`) {
		t.Fatal(c, b)
	}
	if c, b := a.do("PUT", "/api/admin/identity", map[string]any{"name": "Rețea Nord", "subtitle": "", "report_title": "Raport"}, true); c != 200 {
		t.Fatal(c, b)
	}
	if w := s.do("GET", "/api/state", nil, nil); !strings.Contains(w.Body.String(), "Rețea Nord") {
		t.Fatal(w.Body.String())
	}
}

func TestBackupDownloadRejectsTraversal(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	if c, _ := a.do("GET", "/api/admin/backups/..%5Csismon.db", nil, false); c != 400 {
		t.Fatal(c)
	}
	if c, b := a.do("POST", "/api/admin/backups", map[string]any{}, true); c != 200 || !strings.Contains(b, "manual") {
		t.Fatal(c, b)
	}
	if c, _ := a.do("GET", "/api/admin/backups", nil, false); c != 200 {
		t.Fatal(c)
	}
}

func TestAuditPaginated(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	for i := 0; i < 3; i++ {
		a.do("PUT", "/api/admin/identity", map[string]any{"name": "N" + strconv.Itoa(i), "subtitle": "", "report_title": "R"}, true)
	}
	var page struct {
		Entries []struct {
			ID     int64  `json:"id"`
			Action string `json:"action"`
		} `json:"entries"`
	}
	_, body := a.do("GET", "/api/admin/audit?limit=2", nil, false)
	decodeString(t, body, &page)
	if len(page.Entries) != 2 {
		t.Fatal(body)
	}
	_, body = a.do("GET", "/api/admin/audit?limit=50&before="+strconv.FormatInt(page.Entries[1].ID, 10), nil, false)
	var rest struct {
		Entries []struct{ ID int64 } `json:"entries"`
	}
	decodeString(t, body, &rest)
	if len(rest.Entries) < 2 { // remaining identity change + initial save_location
		t.Fatal(body)
	}
	if c, _ := a.do("GET", "/api/admin/audit?limit=abc", nil, false); c != 400 {
		t.Fatal(c)
	}
	if c, b := a.do("GET", "/api/admin/diagnostic", nil, false); c != 200 || !strings.Contains(b, `"icmp_mode":"fake"`) {
		t.Fatal(c, b)
	}
}

func (s *testServer) doFrom(remote, method, path string, body any, hdr map[string]string) *httptest.ResponseRecorder {
	old := testRemote
	testRemote = remote
	defer func() { testRemote = old }()
	return s.do(method, path, body, hdr)
}

func decodeString(t *testing.T, body string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("bad json %q: %v", body, err)
	}
}

// Review fix: concurrent password hashing is capped (argon2id uses 64 MiB each).
func TestLoginConcurrencyCap(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	s.d.loginSem = make(chan struct{}, 1)
	s.d.loginSem <- struct{}{} // the only slot is busy
	s.h = build(s.d, nil)
	w := s.do("POST", "/api/admin/login", map[string]string{"password": testPassword}, origin)
	if w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	<-s.d.loginSem
	login(t, s)
}

// Review fix: initial setup must not be reachable through DNS rebinding.
func TestSetupRejectsForeignHost(t *testing.T) {
	s := newServer(t)
	w := s.doFrom("127.0.0.1:5000", "POST", "/api/admin/setup", map[string]string{"password": testPassword},
		map[string]string{"Host": "rebind.example:8501", "Origin": "http://rebind.example:8501"})
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = s.doFrom("127.0.0.1:5000", "GET", "/api/admin/session", nil, map[string]string{"Host": "rebind.example:8501"})
	if !strings.Contains(w.Body.String(), `"setup_allowed":false`) {
		t.Fatal(w.Body.String())
	}
}

func TestAdminLocationsOrder(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	a := login(t, s)
	c, b := a.do("POST", "/api/admin/locations", newLoc, true)
	if c != 200 {
		t.Fatal(c, b)
	}
	var created struct{ ID int64 }
	decodeString(t, b, &created)
	body := map[string]any{"ids": []int64{created.ID, s.loc}}
	if c, _ := a.do("PUT", "/api/admin/locations/order", body, false); c != 403 {
		t.Fatal("without csrf", c)
	}
	if c, b := a.do("PUT", "/api/admin/locations/order", body, true); c != 200 {
		t.Fatal(c, b)
	}
	s.d.Hub.Load(context.Background(), s.d.Store)
	var st struct {
		Locations []struct {
			ID        int64 `json:"id"`
			SortOrder int   `json:"sort_order"`
		}
	}
	decode(t, s.do("GET", "/api/state", nil, nil), &st)
	got := map[int64]int{}
	for _, l := range st.Locations {
		got[l.ID] = l.SortOrder
	}
	if got[created.ID] != 0 || got[s.loc] != 1 {
		t.Fatalf("%+v", got)
	}
}

// Review fix: the failure delay runs after the login slot is released, so
// wrong guesses cannot keep the administrator out with 503 responses.
func TestLoginFailureDelayDoesNotHoldSlot(t *testing.T) {
	s := newServer(t)
	s.setPassword(t)
	s.d.loginSem = make(chan struct{}, 1)
	var held int
	s.d.Sleep = func(context.Context, time.Duration) { held = len(s.d.loginSem) }
	s.h = build(s.d, nil)
	w := s.do("POST", "/api/admin/login", map[string]string{"password": "parola-gresita-123"}, origin)
	if w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	if held != 0 {
		t.Fatal("login slot held during the failure delay")
	}
}
