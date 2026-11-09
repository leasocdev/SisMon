package auth

import (
	"context"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"netwatch/internal/store"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("parola-foarte-buna")
	if err != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("%q %v", h, err)
	}
	if !VerifyPassword(h, "parola-foarte-buna") || VerifyPassword(h, "gresit") || VerifyPassword("garbage", "x") {
		t.Fatal("verify")
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("12345678901") == nil || ValidatePassword("ăăăăăăăăăăăă") != nil || ValidatePassword(strings.Repeat("a", 201)) == nil {
		t.Fatal("length rules")
	}
}

func newStore(t *testing.T) *store.Store {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "a.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestSessionLifecycle(t *testing.T) {
	st := newStore(t)
	s := NewSessions(st)
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0)
	tok, csrf, err := s.Create(ctx, t0)
	if err != nil || len(tok) < 40 || csrf == "" {
		t.Fatal(tok, err)
	}
	var stored []byte
	st.DB.QueryRow(`SELECT token_hash FROM sessions`).Scan(&stored)
	if string(stored) == tok || len(stored) != 32 {
		t.Fatal("token stored in clear")
	}
	if c, ok, _ := s.Validate(ctx, tok, t0.Add(time.Hour)); !ok || c != csrf {
		t.Fatal("valid session rejected")
	}
	if _, ok, _ := s.Validate(ctx, tok, t0.Add(time.Hour+8*time.Hour+time.Second)); ok {
		t.Fatal("idle timeout not enforced")
	}
	tok2, _, _ := s.Create(ctx, t0)
	for h := 1; h <= 24; h++ {
		s.Validate(ctx, tok2, t0.Add(time.Duration(h)*time.Hour))
	}
	if _, ok, _ := s.Validate(ctx, tok2, t0.Add(24*time.Hour+time.Second)); ok {
		t.Fatal("absolute timeout not enforced")
	}
	tok3, _, _ := s.Create(ctx, t0)
	s.DeleteAll(ctx)
	if _, ok, _ := s.Validate(ctx, tok3, t0); ok {
		t.Fatal("DeleteAll")
	}
	tok4, _, _ := s.Create(ctx, t0)
	s.Delete(ctx, tok4)
	if _, ok, _ := s.Validate(ctx, tok4, t0); ok {
		t.Fatal("Delete")
	}
	if _, ok, _ := s.Validate(ctx, "nonsense", t0); ok {
		t.Fatal("garbage token")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(5, 5*time.Minute, 2)
	ip := netip.MustParseAddr("10.0.0.1")
	t0 := time.Unix(1000, 0)
	want := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second}
	for i, w := range want {
		if _, ok := l.Check(ip, t0); !ok {
			t.Fatalf("blocked early at %d", i)
		}
		if d := l.Fail(ip, t0); d != w {
			t.Fatalf("delay %d: %v want %v", i, d, w)
		}
	}
	if ra, ok := l.Check(ip, t0); ok || ra <= 0 {
		t.Fatal("not blocked after 5 failures")
	}
	if _, ok := l.Check(ip, t0.Add(5*time.Minute+time.Second)); !ok {
		t.Fatal("window did not expire")
	}
	l.Reset(ip)
	b, c := netip.MustParseAddr("10.0.0.2"), netip.MustParseAddr("10.0.0.3")
	l.Fail(ip, t0)
	l.Fail(b, t0.Add(time.Second))
	l.Fail(c, t0.Add(2*time.Second))
	if l.Len() != 2 || l.has(ip) || !l.has(b) || !l.has(c) {
		t.Fatal("eviction should drop the oldest entry only")
	}
}

func TestProxyUntrustedIgnoresHeaders(t *testing.T) {
	p, _ := ParseProxies([]string{"10.0.0.5"})
	r := httptest.NewRequest("GET", "http://srv:8501/", nil)
	r.RemoteAddr = "192.0.2.9:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Forwarded-Proto", "https")
	c := p.Client(r)
	if c.IP.String() != "192.0.2.9" || c.ViaProxy || c.Scheme != "http" || c.Host != "srv:8501" {
		t.Fatalf("%+v", c)
	}
}

func TestProxyTrusted(t *testing.T) {
	p, err := ParseProxies([]string{"10.0.0.5/32"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://10.0.0.1:8501/", nil)
	r.RemoteAddr = "10.0.0.5:4444"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.7")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "nw.local")
	c := p.Client(r)
	if c.IP.String() != "10.0.0.7" || !c.ViaProxy || c.Scheme != "https" || c.Host != "nw.local" {
		t.Fatalf("%+v", c)
	}
	if _, err := ParseProxies([]string{"not-an-ip"}); err == nil {
		t.Fatal("invalid proxy accepted")
	}
}

func TestSameOriginBehindProxy(t *testing.T) {
	p, _ := ParseProxies([]string{"10.0.0.5"})
	r := httptest.NewRequest("POST", "http://10.0.0.1:8501/api/admin/login", nil)
	r.RemoteAddr = "10.0.0.5:4444"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "nw.local")
	r.Header.Set("Origin", "https://nw.local")
	if !SameOrigin(r, p.Client(r)) {
		t.Fatal("same origin rejected behind proxy")
	}
	r.Header.Set("Origin", "http://evil")
	if SameOrigin(r, p.Client(r)) {
		t.Fatal("cross origin accepted")
	}
	r.Header.Del("Origin")
	r.Header.Set("Referer", "https://nw.local/administrare")
	if !SameOrigin(r, p.Client(r)) {
		t.Fatal("referer fallback")
	}
	r.Header.Del("Referer")
	if SameOrigin(r, p.Client(r)) {
		t.Fatal("missing origin and referer accepted")
	}
}

func TestDirectLoopback(t *testing.T) {
	var p Proxy
	r := httptest.NewRequest("GET", "http://localhost/", nil)
	r.RemoteAddr = "127.0.0.1:1"
	if !p.Client(r).DirectLoopback(r) {
		t.Fatal("loopback")
	}
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if p.Client(r).DirectLoopback(r) {
		t.Fatal("forwarded loopback")
	}
	r2 := httptest.NewRequest("GET", "http://x/", nil)
	r2.RemoteAddr = "[::1]:1"
	if !p.Client(r2).DirectLoopback(r2) {
		t.Fatal("ipv6 loopback")
	}
}
