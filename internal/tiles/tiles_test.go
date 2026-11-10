package tiles

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var pngBody = []byte("\x89PNG\r\n\x1a\nfake-tile-data")

type upstream struct {
	srv   *httptest.Server
	calls atomic.Int64
	mu    sync.Mutex
	fail  bool
	ctype string
	block chan struct{}
	ua    atomic.Value
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{ctype: "image/png"}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		u.ua.Store(r.Header.Get("User-Agent"))
		u.mu.Lock()
		fail, ctype, block := u.fail, u.ctype, u.block
		u.mu.Unlock()
		if block != nil {
			select {
			case <-block:
			case <-r.Context().Done():
				return
			}
		}
		if fail {
			http.Error(w, "down", 500)
			return
		}
		w.Header().Set("Content-Type", ctype)
		w.Write(pngBody)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newProxy(t *testing.T, u *upstream) (*Proxy, string, *clock) {
	dir := t.TempDir()
	p, err := New(dir, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	c := &clock{t: time.Now()}
	p.Upstream = u.srv.URL
	p.Now = c.now
	return p, dir, c
}

func get(p *Proxy, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

func TestTileValidation(t *testing.T) {
	u := newUpstream(t)
	p, _, _ := newProxy(t, u)
	for _, path := range []string{"/tiles/1/0/0.png", "/tiles/19/0/0.png", "/tiles/2/4/0.png", "/tiles/2/0/-1.png", "/tiles/2/1/1", "/tiles/2/1/1.jpg", "/tiles/a/1/1.png", "/tiles/2/1"} {
		if w := get(p, path); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	if w := get(p, "/tiles/2/3/3.png"); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "public, max-age=86400" {
		t.Fatal(w.Code, w.Header())
	}
}

func TestTileCachedOnDisk(t *testing.T) {
	u := newUpstream(t)
	p, dir, _ := newProxy(t, u)
	get(p, "/tiles/2/1/1.png")
	if w := get(p, "/tiles/2/1/1.png"); w.Code != 200 || w.Body.String() != string(pngBody) {
		t.Fatal(w.Code)
	}
	if u.calls.Load() != 1 {
		t.Fatal("calls", u.calls.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "2", "1", "1.png")); err != nil {
		t.Fatal(err)
	}
	// a new proxy on the same directory reuses the cache
	p2, _ := New(dir, "1.2.3")
	p2.Upstream = u.srv.URL
	if w := get(p2, "/tiles/2/1/1.png"); w.Code != 200 || u.calls.Load() != 1 {
		t.Fatal("reuse", w.Code, u.calls.Load())
	}
}

func TestTileStaleServedWhenUpstreamDown(t *testing.T) {
	u := newUpstream(t)
	p, _, c := newProxy(t, u)
	get(p, "/tiles/2/1/1.png")
	c.add(8 * 24 * time.Hour)
	u.mu.Lock()
	u.fail = true
	u.mu.Unlock()
	if w := get(p, "/tiles/2/1/1.png"); w.Code != 200 || w.Body.String() != string(pngBody) {
		t.Fatal(w.Code)
	}
}

func TestTileExpiredRefetched(t *testing.T) {
	u := newUpstream(t)
	p, _, c := newProxy(t, u)
	get(p, "/tiles/2/1/1.png")
	c.add(8 * 24 * time.Hour)
	get(p, "/tiles/2/1/1.png")
	if u.calls.Load() != 2 {
		t.Fatal(u.calls.Load())
	}
	get(p, "/tiles/2/1/1.png")
	if u.calls.Load() != 2 {
		t.Fatal("refreshed tile must be fresh again", u.calls.Load())
	}
}

func TestTileDedup(t *testing.T) {
	u := newUpstream(t)
	u.block = make(chan struct{})
	p, _, _ := newProxy(t, u)
	var wg sync.WaitGroup
	codes := make([]int, 10)
	for i := range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); codes[i] = get(p, "/tiles/3/1/1.png").Code }()
	}
	time.Sleep(200 * time.Millisecond)
	close(u.block)
	wg.Wait()
	for _, c := range codes {
		if c != 200 {
			t.Fatal(codes)
		}
	}
	if u.calls.Load() != 1 {
		t.Fatal("calls", u.calls.Load())
	}
}

func TestTilePoolFull503(t *testing.T) {
	u := newUpstream(t)
	u.block = make(chan struct{})
	p, _, _ := newProxy(t, u)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); get(p, "/tiles/4/"+string(rune('0'+i))+"/0.png") }()
	}
	deadline := time.Now().Add(2 * time.Second)
	for u.calls.Load() < 4 {
		if time.Now().After(deadline) {
			t.Fatal("upstream not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
	start := time.Now()
	w := get(p, "/tiles/4/9/9.png")
	if w.Code != 503 || time.Since(start) > 100*time.Millisecond {
		t.Fatal(w.Code, time.Since(start))
	}
	close(u.block)
	wg.Wait()
}

func TestTileTimeout(t *testing.T) {
	u := newUpstream(t)
	u.block = make(chan struct{})
	defer close(u.block)
	p, _, _ := newProxy(t, u)
	p.Timeout = 300 * time.Millisecond
	start := time.Now()
	w := get(p, "/tiles/2/1/1.png")
	if (w.Code != 502 && w.Code != 504) || time.Since(start) > 2*time.Second {
		t.Fatal(w.Code, time.Since(start))
	}
}

func TestTileDefaults(t *testing.T) {
	p, err := New(t.TempDir(), "1.0")
	if err != nil {
		t.Fatal(err)
	}
	if p.Upstream != "https://tile.openstreetmap.org" || p.MaxBytes != 500<<20 || p.TTL != 7*24*time.Hour || p.Timeout != 5*time.Second {
		t.Fatalf("%+v", p)
	}
}

func TestTileEvictsLRU(t *testing.T) {
	u := newUpstream(t)
	p, dir, c := newProxy(t, u)
	size := int64(len(pngBody))
	p.MaxBytes = 2 * size
	get(p, "/tiles/2/0/0.png")
	c.add(time.Second)
	get(p, "/tiles/2/0/1.png")
	c.add(time.Second)
	get(p, "/tiles/2/0/0.png") // touch: 0/1 is now the least recently used
	c.add(time.Second)
	get(p, "/tiles/2/0/2.png")
	if _, err := os.Stat(filepath.Join(dir, "2", "0", "1.png")); !os.IsNotExist(err) {
		t.Fatal("LRU tile not evicted", err)
	}
	for _, f := range []string{"0.png", "2.png"} {
		if _, err := os.Stat(filepath.Join(dir, "2", "0", f)); err != nil {
			t.Fatal(f, err)
		}
	}
}

func TestTileUserAgent(t *testing.T) {
	u := newUpstream(t)
	p, _, _ := newProxy(t, u)
	get(p, "/tiles/2/1/1.png")
	if ua, _ := u.ua.Load().(string); !strings.HasPrefix(ua, "NetWatch/1.2.3") {
		t.Fatal(ua)
	}
}

func TestTileRejectsNonPNG(t *testing.T) {
	u := newUpstream(t)
	u.mu.Lock()
	u.ctype = "text/html"
	u.mu.Unlock()
	p, dir, _ := newProxy(t, u)
	if w := get(p, "/tiles/2/1/1.png"); w.Code != 502 {
		t.Fatal(w.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "2", "1", "1.png")); !os.IsNotExist(err) {
		t.Fatal("non-PNG saved")
	}
}
