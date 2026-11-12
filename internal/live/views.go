// Package live keeps the current state in memory and fans out changes.
package live

import "sismon/internal/state"

// DeviceView is the live state of one device.
type DeviceView struct {
	ID         int64       `json:"id"`
	LocationID int64       `json:"location_id"`
	Name       string      `json:"name"`
	Host       string      `json:"host"`
	Kind       string      `json:"kind"`
	Primary    bool        `json:"is_primary"`
	State      state.State `json:"state"`
	Pending    state.State `json:"pending"`
	Since      int64       `json:"since"`
	LastCheck  *int64      `json:"last_check"`
	LatencyMs  *float64    `json:"latency_ms"`
	LossPct    *float64    `json:"loss_pct"`
	ErrorCode  string      `json:"error_code"`
	DNSWarning bool        `json:"dns_warning"`
}

// LocationView is the live state of one location.
type LocationView struct {
	ID         int64        `json:"id"`
	Name       string       `json:"name"`
	Address    string       `json:"address"`
	Notes      string       `json:"notes"`
	Lat        *float64     `json:"lat"`
	Lon        *float64     `json:"lon"`
	SortOrder  int          `json:"sort_order"`
	State      state.State  `json:"state"`
	Incomplete bool         `json:"incomplete"`
	Since      int64        `json:"since"`
	Devices    []DeviceView `json:"devices"`
}

// Health describes the monitor itself.
type Health struct {
	LastCycle   *int64   `json:"last_cycle"`
	LateChecks  int      `json:"late_checks"`
	AvgCheckMs  float64  `json:"avg_check_ms"`
	ICMPMode    string   `json:"icmp_mode"`
	Errors      []string `json:"errors"`
	DBSizeBytes int64    `json:"db_size_bytes"`
	LastBackup  *int64   `json:"last_backup"`
	BackupError string   `json:"backup_error"`
	Warning     string   `json:"warning"`
}

// Event is one change published to subscribers.
// Type: "state" (LocationView), "removed" (location id), "health" (Health).
type Event struct {
	Seq  uint64 `json:"seq"`
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Snapshot is the full current state.
type Snapshot struct {
	Epoch     int64          `json:"epoch"`
	Seq       uint64         `json:"seq"`
	Locations []LocationView `json:"locations"`
	Health    Health         `json:"health"`
}

func severity(s state.State) int {
	switch s {
	case state.Down:
		return 0
	case state.Degraded:
		return 1
	case state.Error, state.Unknown:
		return 2
	}
	return 3
}

func members(ds []DeviceView) []state.Member {
	out := make([]state.Member, len(ds))
	for i, d := range ds {
		out[i] = state.Member{Primary: d.Primary, State: d.State}
	}
	return out
}
