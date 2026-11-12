package monitor

import (
	"time"

	"sismon/internal/monitor/icmp"
)

// Result is one completed check of a device configuration revision.
type Result struct {
	DeviceID   int64
	Rev        int64
	At         time.Time
	Ping       icmp.Result
	DNSWarning bool
}
