// Package tiles is a caching proxy for OpenStreetMap tiles: disk cache with
// LRU eviction, bounded concurrent downloads, request de-duplication, and
// stale tiles served when the upstream is unavailable.
package tiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	minZoom      = 2
	maxZoom      = 18
	maxTileBytes = 1 << 20
	concurrency  = 4
)

type entry struct {
	size    int64
	access  time.Time
	fetched time.Time
}

type call struct {
	done chan struct{}
	data []byte
	err  error
}

// Proxy serves /tiles/{z}/{x}/{y}.png.
type Proxy struct {
	Upstream string
	MaxBytes int64
	TTL      time.Duration
	Timeout  time.Duration
	Client   *http.Client
	Now      func() time.Time

	dir string
	ua  string
	sem chan struct{}

	mu       sync.Mutex
	index    map[string]*entry
	total    int64
	inflight map[string]*call
}

var errTimeout = errors.New("timeout")

// New builds a proxy caching under dir and indexes the existing cache.
func New(dir, version string) (*Proxy, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	p := &Proxy{
		Upstream: "https://tile.openstreetmap.org", MaxBytes: 500 << 20, TTL: 7 * 24 * time.Hour, Timeout: 5 * time.Second,
		Client: &http.Client{}, Now: time.Now, dir: dir, ua: "NetWatch/" + version + " (monitorizare rețea, instalare locală)",
		sem: make(chan struct{}, concurrency), index: map[string]*entry{}, inflight: map[string]*call{},
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".tile-") {
			os.Remove(path) // left by a crash during a download
			return nil
		}
		if !strings.HasSuffix(path, ".png") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(strings.TrimSuffix(rel, ".png"))
		p.index[key] = &entry{size: fi.Size(), access: fi.ModTime(), fetched: fi.ModTime()}
		p.total += fi.Size()
		return nil
	})
	return p, err
}

// parse validates /tiles/{z}/{x}/{y}.png.
func parse(urlPath string) (z, x, y int, ok bool) {
	parts := strings.Split(strings.TrimPrefix(urlPath, "/tiles/"), "/")
	if len(parts) != 3 || !strings.HasSuffix(parts[2], ".png") {
		return 0, 0, 0, false
	}
	var err error
	if z, err = strconv.Atoi(parts[0]); err != nil || z < minZoom || z > maxZoom {
		return 0, 0, 0, false
	}
	n := 1 << z
	if x, err = strconv.Atoi(parts[1]); err != nil || x < 0 || x >= n {
		return 0, 0, 0, false
	}
	if y, err = strconv.Atoi(strings.TrimSuffix(parts[2], ".png")); err != nil || y < 0 || y >= n {
		return 0, 0, 0, false
	}
	return z, x, y, true
}

func (p *Proxy) file(key string) string { return filepath.Join(p.dir, filepath.FromSlash(key)+".png") }

func writeTile(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// cached returns the cached tile and whether it is still fresh.
func (p *Proxy) cached(key string) ([]byte, bool, bool) {
	p.mu.Lock()
	e, ok := p.index[key]
	p.mu.Unlock()
	if !ok {
		return nil, false, false
	}
	data, err := os.ReadFile(p.file(key))
	if err != nil {
		p.mu.Lock()
		if p.index[key] == e {
			delete(p.index, key)
			p.total -= e.size
		}
		p.mu.Unlock()
		return nil, false, false
	}
	now := p.Now()
	p.mu.Lock()
	e.access = now
	fresh := now.Sub(e.fetched) < p.TTL
	p.mu.Unlock()
	return data, fresh, true
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	z, x, y, ok := parse(r.URL.Path)
	if !ok {
		http.Error(w, "Dală invalidă.", http.StatusBadRequest)
		return
	}
	key := fmt.Sprintf("%d/%d/%d", z, x, y)
	stale, fresh, have := p.cached(key)
	if have && fresh {
		writeTile(w, stale)
		return
	}
	p.mu.Lock()
	c, joined := p.inflight[key]
	if !joined {
		select {
		case p.sem <- struct{}{}:
		default:
			p.mu.Unlock()
			if have {
				writeTile(w, stale)
				return
			}
			http.Error(w, "Prea multe descărcări de dale în curs.", http.StatusServiceUnavailable)
			return
		}
		c = &call{done: make(chan struct{})}
		p.inflight[key] = c
		p.mu.Unlock()
		go p.fetch(key, c)
	} else {
		p.mu.Unlock()
	}
	select {
	case <-c.done:
	case <-r.Context().Done():
		return
	}
	switch {
	case c.err == nil:
		writeTile(w, c.data)
	case have:
		writeTile(w, stale)
	case errors.Is(c.err, errTimeout):
		http.Error(w, "OpenStreetMap nu răspunde.", http.StatusGatewayTimeout)
	default:
		http.Error(w, "Dala nu a putut fi descărcată.", http.StatusBadGateway)
	}
}

// fetch downloads one tile (independently of any single client request).
func (p *Proxy) fetch(key string, c *call) {
	defer func() {
		<-p.sem
		p.mu.Lock()
		delete(p.inflight, key)
		p.mu.Unlock()
		close(c.done)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout)
	defer cancel()
	c.data, c.err = p.download(ctx, key)
	if c.err == nil {
		if err := p.store(key, c.data); err != nil {
			slog.Warn("cache dale", "err", err)
		}
	} else {
		slog.Debug("dală", "key", key, "err", c.err)
	}
}

func (p *Proxy) download(ctx context.Context, key string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(p.Upstream, "/")+"/"+key+".png", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", p.ua)
	resp, err := p.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errTimeout
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/png") {
		return nil, fmt.Errorf("upstream content-type %q", resp.Header.Get("Content-Type"))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTileBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, errTimeout
		}
		return nil, err
	}
	if len(data) > maxTileBytes {
		return nil, errors.New("tile too large")
	}
	return data, nil
}

// store writes the tile atomically and evicts least recently used tiles.
func (p *Proxy) store(key string, data []byte) error {
	path := p.file(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tile-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	now := p.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if old, ok := p.index[key]; ok {
		p.total -= old.size
	}
	p.index[key] = &entry{size: int64(len(data)), access: now, fetched: now}
	p.total += int64(len(data))
	if p.total <= p.MaxBytes {
		return nil
	}
	keys := make([]string, 0, len(p.index))
	for k := range p.index {
		if k != key {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return p.index[keys[i]].access.Before(p.index[keys[j]].access) })
	for _, k := range keys {
		if p.total <= p.MaxBytes {
			break
		}
		if err := os.Remove(p.file(k)); err != nil && !os.IsNotExist(err) {
			continue
		}
		p.total -= p.index[k].size
		delete(p.index, k)
	}
	return nil
}
