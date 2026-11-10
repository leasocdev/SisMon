// Command e2eserver runs a real NetWatch server with a scripted pinger and a
// control port, for the Playwright end-to-end tests. Not shipped.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"netwatch/internal/app"
	"netwatch/internal/auth"
	"netwatch/internal/config"
	"netwatch/internal/domain"
	"netwatch/internal/monitor/icmp"
	"netwatch/internal/settings"
)

// Password is the administrator password of the seeded server.
const Password = "parola-e2e-12345"

type hostResult struct {
	Result    string  `json:"result"` // ok | timeout | permission
	LatencyMs float64 `json:"latency_ms"`
}

type control struct {
	mu      sync.Mutex
	hosts   map[string]hostResult
	sseOn   bool
	tilesOK bool
	streams map[int]context.CancelFunc
	nextID  int
	tile    []byte
}

func (c *control) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hosts = map[string]hostResult{}
	c.sseOn, c.tilesOK = true, true
}

func (c *control) ping(addr netip.Addr) icmp.Result {
	c.mu.Lock()
	h, ok := c.hosts[addr.String()]
	c.mu.Unlock()
	if !ok {
		h = hostResult{Result: "ok", LatencyMs: 5}
	}
	switch h.Result {
	case "timeout":
		return icmp.Result{Sent: 3, Code: icmp.Timeout}
	case "permission":
		return icmp.Result{Sent: 3, Code: icmp.Permission}
	}
	l := h.LatencyMs
	if l == 0 {
		l = 5
	}
	return icmp.Result{Sent: 3, Received: 3, LatencyMs: &l}
}

// wrap lets the control port cut SSE streams and fail the tile proxy.
func (c *control) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/events":
			c.mu.Lock()
			if !c.sseOn {
				c.mu.Unlock()
				http.Error(w, "SSE oprit (test)", http.StatusServiceUnavailable)
				return
			}
			ctx, cancel := context.WithCancel(r.Context())
			id := c.nextID
			c.nextID++
			c.streams[id] = cancel
			c.mu.Unlock()
			defer func() {
				c.mu.Lock()
				delete(c.streams, id)
				c.mu.Unlock()
				cancel()
			}()
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		case strings.HasPrefix(r.URL.Path, "/tiles/"):
			c.mu.Lock()
			ok := c.tilesOK
			c.mu.Unlock()
			if !ok {
				http.Error(w, "dale indisponibile (test)", http.StatusBadGateway)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, err.Error(), 400)
		return false
	}
	return true
}

func (c *control) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /host", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Host string `json:"host"`
			hostResult
		}
		if !decode(w, r, &b) {
			return
		}
		c.mu.Lock()
		c.hosts[b.Host] = b.hostResult
		c.mu.Unlock()
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /reset", func(w http.ResponseWriter, r *http.Request) {
		c.reset()
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /sse", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			On bool `json:"on"`
		}
		if !decode(w, r, &b) {
			return
		}
		c.mu.Lock()
		c.sseOn = b.On
		if !b.On {
			for id, cancel := range c.streams {
				cancel()
				delete(c.streams, id)
			}
		}
		c.mu.Unlock()
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /tiles", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			OK bool `json:"ok"`
		}
		if !decode(w, r, &b) {
			return
		}
		c.mu.Lock()
		c.tilesOK = b.OK
		c.mu.Unlock()
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("GET /osm/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(c.tile)
	})
	return mux
}

func grayTile() []byte {
	img := image.NewGray(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			v := uint8(225)
			if x%64 == 0 || y%64 == 0 {
				v = 200
			}
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func f(v float64) *float64 { return &v }

func seed(dataDir string, password bool) error {
	ctx := context.Background()
	st, err := app.OpenStore(ctx, dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	if password {
		h, err := auth.HashPassword(Password)
		if err != nil {
			return err
		}
		if err := st.SetMeta(ctx, "password_hash", h); err != nil {
			return err
		}
	}
	locs := []domain.LocationInput{
		{Name: "Chișinău Centru", Address: "bd. Ștefan cel Mare 1", Lat: f(47.0105), Lon: f(28.8638), Devices: []domain.DeviceInput{
			{Name: "Router", Host: "10.0.0.1", Kind: "Router", Primary: true}, {Name: "Switch", Host: "10.0.0.2", Kind: "Switch"}}},
		{Name: "Cahul", Lat: f(45.9075), Lon: f(28.1944), Devices: []domain.DeviceInput{
			{Name: "Router", Host: "10.0.1.1", Kind: "Router", Primary: true}}},
		{Name: "Bălți", Lat: f(47.7617), Lon: f(27.9289), Devices: []domain.DeviceInput{
			{Name: "Router", Host: "10.0.2.1", Kind: "Router", Primary: true}, {Name: "AP", Host: "10.0.2.2", Kind: "Radiolink"}}},
	}
	created := time.Now().Add(-48 * time.Hour)
	for _, in := range locs {
		if err := in.Normalize(settings.Defaults()); err != nil {
			return err
		}
		if _, err := st.SaveLocation(ctx, nil, in, created); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	listen := flag.String("listen", "127.0.0.1:18501", "adresa serverului NetWatch")
	ctrlAddr := flag.String("control", "127.0.0.1:18502", "adresa portului de control")
	noPassword := flag.Bool("no-password", false, "pornește fără parolă de administrator")
	demoFlag := flag.Bool("demo-flag", false, "marchează serverul ca mod demonstrativ (banner)")
	web := flag.String("web", "internal/webui/dist", "directorul interfeței compilate")
	flag.Parse()

	cleanOld()
	dataDir, err := os.MkdirTemp("", "netwatch-e2e-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dataDir)
	if err := seed(dataDir, !*noPassword); err != nil {
		log.Fatal(err)
	}
	c := &control{streams: map[int]context.CancelFunc{}, tile: grayTile()}
	c.reset()

	ln, err := net.Listen("tcp", *ctrlAddr)
	if err != nil {
		log.Fatal(err)
	}
	go http.Serve(ln, c.routes())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cfg := config.Config{DataDir: dataDir, Listen: *listen}
	err = app.Run(ctx, cfg, app.Options{
		Pinger: &icmp.Fake{Fn: c.ping},
		Lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
			if a, err := netip.ParseAddr(host); err == nil {
				return []netip.Addr{a}, nil
			}
			return nil, fmt.Errorf("nume necunoscut: %s", host)
		},
		Web:           os.DirFS(*web),
		Wrap:          c.wrap,
		TilesUpstream: "http://" + *ctrlAddr + "/osm",
		Demo:          *demoFlag,
		IntervalScale: 1.0 / 60,
		FlushEvery:    50 * time.Millisecond,
		Ready:         func(a string) { log.Printf("e2eserver gata pe %s (control %s, date %s)", a, *ctrlAddr, dataDir) },
	})
	if err != nil {
		log.Fatal(err)
	}
}

// cleanOld removes data directories left by killed previous runs.
func cleanOld() {
	old, _ := filepath.Glob(filepath.Join(os.TempDir(), "netwatch-e2e-*"))
	for _, d := range old {
		if fi, err := os.Stat(d); err == nil && time.Since(fi.ModTime()) > time.Hour {
			os.RemoveAll(d)
		}
	}
}
