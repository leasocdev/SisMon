// Package demo provides the simulated network used by `netwatch demo`.
package demo

import (
	"context"
	"fmt"
	"math"
	"net/netip"
	"time"

	"netwatch/internal/auth"
	"netwatch/internal/domain"
	"netwatch/internal/monitor/icmp"
	"netwatch/internal/settings"
	"netwatch/internal/store"
)

// Password is the administrator password of the demo (shown in the banner).
const Password = "netwatch-demo"

type device struct{ name, kind string }

type place struct {
	name     string
	lat, lon float64
	devices  []device
}

var places = []place{
	{"Chișinău Centru", 47.0105, 28.8638, []device{{"Router", "Router"}, {"Switch", "Switch"}, {"Server", "Server"}, {"AP", "Radiolink"}}},
	{"Bălți", 47.7617, 27.9289, []device{{"Router", "Router"}, {"Switch", "Switch"}}},
	{"Cahul", 45.9075, 28.1944, []device{{"Router", "Router"}, {"AP", "Radiolink"}}},
	{"Orhei", 47.3849, 28.8245, []device{{"Router", "Router"}, {"Radiolink", "Radiolink"}}},
	{"Ungheni", 47.2108, 27.8001, []device{{"Router", "Router"}}},
	{"Soroca", 48.1558, 28.2975, []device{{"Router", "Router"}, {"Switch", "Switch"}}},
	{"Comrat", 46.2956, 28.6567, []device{{"Router", "Router"}, {"AP", "Radiolink"}}},
	{"Edineț", 48.1681, 27.3036, []device{{"Router", "Router"}, {"AP", "Radiolink"}}},
	{"Hîncești", 46.8281, 28.5908, []device{{"Router", "Router"}}},
	{"Strășeni", 47.1414, 28.6103, []device{{"Router", "Router"}, {"Switch", "Switch"}}},
	{"Căușeni", 46.6436, 29.4111, []device{{"Router", "Router"}}},
	{"Ștefan Vodă", 46.5153, 29.6631, []device{{"Router", "Router"}, {"Radiolink", "Radiolink"}, {"AP", "Radiolink"}}},
}

// HostOf returns the simulated IP of a demo device.
func HostOf(location string, deviceIndex int) string {
	for i, p := range places {
		if p.name == location {
			return fmt.Sprintf("10.%d.0.%d", i+1, deviceIndex+1)
		}
	}
	return ""
}

// Seed fills an empty database with the demo locations, password and identity.
func Seed(ctx context.Context, st *store.Store) error {
	locs, err := st.ListLocations(ctx, true)
	if err != nil || len(locs) > 0 {
		return err
	}
	ins := make([]domain.LocationInput, 0, len(places))
	for i, p := range places {
		lat, lon := p.lat, p.lon
		in := domain.LocationInput{Name: p.name, Address: "Locație demonstrativă", Lat: &lat, Lon: &lon}
		for j, d := range p.devices {
			in.Devices = append(in.Devices, domain.DeviceInput{Name: d.name, Kind: d.kind, Host: fmt.Sprintf("10.%d.0.%d", i+1, j+1), Primary: j == 0})
		}
		if err := in.Normalize(settings.Defaults()); err != nil {
			return err
		}
		ins = append(ins, in)
	}
	if _, err := st.SaveLocations(ctx, ins, time.Now()); err != nil {
		return err
	}
	h, err := auth.HashPassword(Password)
	if err != nil {
		return err
	}
	if err := st.SetMeta(ctx, "password_hash", h); err != nil {
		return err
	}
	return st.SetMeta(ctx, "identity", map[string]any{"name": "NetWatch Demo", "subtitle": "MOD DEMONSTRATIV", "report_title": "Raport de disponibilitate (demo)", "logo_version": 0})
}

// Scenario answers pings deterministically from the time elapsed since Start.
type Scenario struct{ Start time.Time }

func ok(ms float64) icmp.Result { return icmp.Result{Sent: 3, Received: 3, LatencyMs: &ms} }

// Result is the simulated outcome of a check of addr at now.
func (s Scenario) Result(addr netip.Addr, now time.Time) icmp.Result {
	m := now.Sub(s.Start).Minutes()
	if m < 0 {
		m = 0
	}
	b := addr.As4()
	loc, dev := int(b[1])-1, int(b[3])-1
	if loc < 0 || loc >= len(places) {
		return icmp.Result{Sent: 3, Code: icmp.HostUnreachable}
	}
	// normal latency 2–15 ms, a smooth deterministic wave per device
	normal := 2 + 13*(0.5+0.5*math.Sin(float64(loc*7+dev)+m/7))
	switch places[loc].name {
	case "Cahul":
		if dev == 0 && math.Mod(m, 30) < 6 {
			return icmp.Result{Sent: 3, Code: icmp.Timeout}
		}
	case "Orhei":
		if dev == 1 && math.Mod(m, 20) < 10 {
			return icmp.Result{Sent: 3, Code: icmp.Timeout}
		}
	case "Soroca":
		if dev == 0 {
			l := 80 + 40*math.Sin(m/3)
			return icmp.Result{Sent: 3, Received: 2, LatencyMs: &l}
		}
	case "Comrat":
		if dev == 0 && int(now.Sub(s.Start).Seconds()/10)%3 == 0 {
			return icmp.Result{Sent: 3, Code: icmp.Timeout}
		}
	case "Edineț":
		if dev == 1 && math.Mod(m, 60) < 3 {
			return icmp.Result{Sent: 3, Code: icmp.Permission}
		}
	}
	return ok(normal)
}
