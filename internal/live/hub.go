package live

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"netwatch/internal/state"
	"netwatch/internal/store"
)

const (
	historySize = 512
	bufferSize  = 256
)

// Hub holds the live state and publishes changes.
type Hub struct {
	epoch   int64
	mu      sync.Mutex
	seq     uint64
	locs    map[int64]*LocationView
	health  Health
	history []Event
	subs    map[int]chan Event
	nextSub int
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{epoch: time.Now().UnixMilli(), locs: map[int64]*LocationView{}, subs: map[int]chan Event{}}
}

func (h *Hub) publish(typ string, data any) {
	h.seq++
	e := Event{Seq: h.seq, Type: typ, Data: data}
	h.history = append(h.history, e)
	if len(h.history) > historySize {
		h.history = h.history[len(h.history)-historySize:]
	}
	for id, ch := range h.subs {
		select {
		case ch <- e:
		default: // slow subscriber: drop it, never block the hub
			close(ch)
			delete(h.subs, id)
		}
	}
}

func copyLoc(l *LocationView) LocationView {
	c := *l
	c.Devices = append([]DeviceView(nil), l.Devices...)
	return c
}

// Replace swaps the whole state, publishing changed and removed locations.
func (h *Hub) Replace(locs []LocationView) {
	h.mu.Lock()
	defer h.mu.Unlock()
	next := map[int64]*LocationView{}
	for i := range locs {
		l := locs[i]
		next[l.ID] = &l
	}
	for id := range h.locs {
		if _, ok := next[id]; !ok {
			h.publish("removed", id)
		}
	}
	for id, l := range next {
		if old, ok := h.locs[id]; !ok || !reflect.DeepEqual(*old, *l) {
			h.publish("state", copyLoc(l))
		}
	}
	h.locs = next
}

// SetDevice updates one device and recomputes its location.
func (h *Hub) SetDevice(dv DeviceView) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l, ok := h.locs[dv.LocationID]
	if !ok {
		return
	}
	for i := range l.Devices {
		if l.Devices[i].ID == dv.ID {
			if dv.Name == "" { // partial update from the writer keeps configuration fields
				dv.Name, dv.Host, dv.Kind, dv.Primary = l.Devices[i].Name, l.Devices[i].Host, l.Devices[i].Kind, l.Devices[i].Primary
			}
			l.Devices[i] = dv
		}
	}
	r := state.Location(members(l.Devices))
	if r.State != l.State {
		l.Since = time.Now().UnixMilli()
	}
	l.State, l.Incomplete = r.State, r.Incomplete
	h.publish("state", copyLoc(l))
}

// SetLocationState sets the location state and its effective start.
func (h *Hub) SetLocationState(id int64, r state.LocationResult, since int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l, ok := h.locs[id]
	if !ok {
		return
	}
	if l.State == r.State && l.Incomplete == r.Incomplete && l.Since == since {
		return
	}
	l.State, l.Incomplete, l.Since = r.State, r.Incomplete, since
	h.publish("state", copyLoc(l))
}

// SetHealth replaces the monitor health.
func (h *Hub) SetHealth(hl Health) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.health = hl
	h.publish("health", hl)
}

// Epoch identifies this hub instance (ms at creation); SSE ids are "epoch-seq".
func (h *Hub) Epoch() int64 { return h.epoch }

// Health returns the current monitor health.
func (h *Hub) Health() Health {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.health
}

// Snapshot returns the full state ordered by severity, then name.
func (h *Hub) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := Snapshot{Epoch: h.epoch, Seq: h.seq, Health: h.health, Locations: make([]LocationView, 0, len(h.locs))}
	for _, l := range h.locs {
		out.Locations = append(out.Locations, copyLoc(l))
	}
	sort.Slice(out.Locations, func(i, j int) bool {
		a, b := out.Locations[i], out.Locations[j]
		if severity(a.State) != severity(b.State) {
			return severity(a.State) < severity(b.State)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out
}

// Since returns the events after seq, or false if they are no longer kept.
func (h *Hub) Since(seq uint64) ([]Event, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.since(seq)
}

func (h *Hub) since(seq uint64) ([]Event, bool) {
	if seq == h.seq {
		return nil, true
	}
	if seq == 0 || seq > h.seq || len(h.history) == 0 || h.history[0].Seq > seq+1 {
		return nil, false
	}
	var out []Event
	for _, e := range h.history {
		if e.Seq > seq {
			out = append(out, e)
		}
	}
	return out, true
}

// Subscribe returns a channel of events after afterSeq (replayed from
// history when available) and a cancel function. The channel is closed if
// the subscriber falls behind.
func (h *Hub) Subscribe(afterSeq uint64) (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan Event, bufferSize)
	if past, ok := h.since(afterSeq); ok {
		for _, e := range past {
			if len(ch) == cap(ch) {
				break
			}
			ch <- e
		}
	}
	id := h.nextSub
	h.nextSub++
	h.subs[id] = ch
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if c, ok := h.subs[id]; ok {
			close(c)
			delete(h.subs, id)
		}
	}
}

// Subscribers returns the number of active subscribers.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Load rebuilds the state from the database.
func (h *Hub) Load(ctx context.Context, st *store.Store) error {
	locs, err := st.ListLocations(ctx, false)
	if err != nil {
		return err
	}
	stats, err := store.DeviceStatuses(ctx, st.DB)
	if err != nil {
		return err
	}
	views := make([]LocationView, 0, len(locs))
	for _, l := range locs {
		v := LocationView{ID: l.ID, Name: l.Name, Address: l.Address, Notes: l.Notes, Lat: l.Lat, Lon: l.Lon, SortOrder: l.SortOrder, Devices: []DeviceView{}}
		for _, d := range l.Devices {
			v.Devices = append(v.Devices, DeviceViewFrom(d, stats[d.ID]))
		}
		r := state.Location(members(v.Devices))
		v.State, v.Incomplete = r.State, r.Incomplete
		if _, start, ok, err := store.OpenState(ctx, st.DB, store.EntityLocation, l.ID); err != nil {
			return err
		} else if ok {
			v.Since = start.UnixMilli()
		}
		views = append(views, v)
	}
	h.Replace(views)
	return nil
}

// DeviceViewFrom combines configuration and status.
func DeviceViewFrom(d store.Device, s store.Status) DeviceView {
	st := s.State
	if st == "" {
		st = state.Unknown
	}
	return DeviceView{ID: d.ID, LocationID: d.LocationID, Name: d.Name, Host: d.Host, Kind: d.Kind, Primary: d.Primary,
		State: st, Pending: s.Pending, Since: s.Since, LastCheck: s.LastCheck, LatencyMs: s.LatencyMs, LossPct: s.LossPct,
		ErrorCode: s.ErrorCode, DNSWarning: s.DNSWarning}
}
