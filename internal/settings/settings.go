// Package settings holds the global monitoring settings and their validation.
package settings

import (
	"fmt"
	"math"
	"time"

	"netwatch/internal/apperr"
)

// Settings are the global monitoring settings, stored in meta "settings".
type Settings struct {
	IntervalS          int    `json:"interval_s"`
	Packets            int    `json:"packets"`
	TimeoutS           int    `json:"timeout_s"`
	Failures           int    `json:"failures"`
	Recoveries         int    `json:"recoveries"`
	StaleS             int    `json:"stale_s"`
	IncidentsFor       string `json:"incidents_for"`
	PreferIPv4         bool   `json:"prefer_ipv4"`
	BackupHour         int    `json:"backup_hour"`
	BackupSecondaryDir string `json:"backup_secondary_dir"`
	// DispatchRotationS is the Dispecerat grid/map rotation period; 0 = off.
	DispatchRotationS int `json:"dispatch_rotation_s"`
	// DispatchTheme is the Dispecerat color scheme: "dark", "light" or "system".
	DispatchTheme string `json:"dispatch_theme"`
}

// Overrides are optional per-device thresholds.
type Overrides struct {
	IntervalS  *int `json:"interval_s,omitempty"`
	Failures   *int `json:"failures,omitempty"`
	Recoveries *int `json:"recoveries,omitempty"`
}

// Effective are the thresholds applied to one device.
type Effective struct {
	Interval, Timeout    time.Duration
	Packets              int
	Failures, Recoveries int
	Stale                time.Duration
}

// PacketGap is the pause between ICMP packets of one check.
const PacketGap = 200 * time.Millisecond

// DNSTimeout bounds name resolution.
const DNSTimeout = 2 * time.Second

// Defaults returns the spec defaults.
func Defaults() Settings {
	return Settings{IntervalS: 60, Packets: 3, TimeoutS: 2, Failures: 3, Recoveries: 2, StaleS: 300,
		IncidentsFor: "all", PreferIPv4: true, BackupHour: 3, DispatchTheme: "dark"}
}

type limit struct {
	field  string
	v      int
	lo, hi int
	label  string
}

func checkLimits(ls []limit) error {
	for _, l := range ls {
		if l.v < l.lo || l.v > l.hi {
			return apperr.Invalid(l.field, fmt.Sprintf("%s trebuie să fie între %d și %d.", l.label, l.lo, l.hi))
		}
	}
	return nil
}

// Validate checks limits and cross-field rules.
func (s Settings) Validate() error {
	if err := checkLimits([]limit{
		{"interval_s", s.IntervalS, 10, 3600, "Intervalul între verificări (secunde)"},
		{"packets", s.Packets, 1, 10, "Numărul de pachete"},
		{"timeout_s", s.TimeoutS, 1, 10, "Timeout-ul per pachet (secunde)"},
		{"failures", s.Failures, 1, 20, "Numărul de eșecuri pentru confirmarea căderii"},
		{"recoveries", s.Recoveries, 1, 20, "Numărul de reușite pentru confirmarea revenirii"},
		{"stale_s", s.StaleS, 30, 86400, "Expirarea datelor (secunde)"},
		{"backup_hour", s.BackupHour, 0, 23, "Ora backup-ului"},
	}); err != nil {
		return err
	}
	if s.DispatchRotationS != 0 {
		if err := checkLimits([]limit{{"dispatch_rotation_s", s.DispatchRotationS, 10, 3600, "Rotația din Dispecerat (secunde)"}}); err != nil {
			return err
		}
	}
	switch s.DispatchTheme {
	case "", "dark", "light", "system": // empty (older data) means dark
	default:
		return apperr.Invalid("dispatch_theme", "Schema de culori a Dispeceratului este „întunecată”, „luminoasă” sau „ca sistemul”.")
	}
	if s.IncidentsFor != "all" && s.IncidentsFor != "primary" {
		return apperr.Invalid("incidents_for", "Incidentele se deschid pentru „toate” echipamentele sau „doar primare”.")
	}
	_, err := s.Apply(Overrides{})
	return err
}

// Apply combines the global settings with per-device overrides and validates
// the combination.
func (s Settings) Apply(o Overrides) (Effective, error) {
	interval, failures, recoveries := s.IntervalS, s.Failures, s.Recoveries
	var ls []limit
	if o.IntervalS != nil {
		interval = *o.IntervalS
		ls = append(ls, limit{"interval_s", interval, 10, 3600, "Intervalul între verificări (secunde)"})
	}
	if o.Failures != nil {
		failures = *o.Failures
		ls = append(ls, limit{"failures", failures, 1, 20, "Numărul de eșecuri pentru confirmarea căderii"})
	}
	if o.Recoveries != nil {
		recoveries = *o.Recoveries
		ls = append(ls, limit{"recoveries", recoveries, 1, 20, "Numărul de reușite pentru confirmarea revenirii"})
	}
	if err := checkLimits(ls); err != nil {
		return Effective{}, err
	}
	check := time.Duration(s.Packets)*time.Duration(s.TimeoutS)*time.Second + time.Duration(s.Packets-1)*PacketGap + DNSTimeout
	if check >= time.Duration(interval)*time.Second {
		minInterval := int(math.Floor(check.Seconds())) + 1
		return Effective{}, apperr.Invalid("interval_s", fmt.Sprintf(
			"Intervalul trebuie să fie de cel puțin %d secunde pentru %d pachete cu timeout de %d s.", minInterval, s.Packets, s.TimeoutS))
	}
	need := interval * (max(failures, recoveries) + 1)
	if s.StaleS < need {
		return Effective{}, apperr.Invalid("stale_s", fmt.Sprintf(
			"Expirarea datelor trebuie să fie de cel puțin %d secunde (interval × (prag + 1)).", need))
	}
	return Effective{
		Interval: time.Duration(interval) * time.Second, Timeout: time.Duration(s.TimeoutS) * time.Second,
		Packets: s.Packets, Failures: failures, Recoveries: recoveries, Stale: time.Duration(s.StaleS) * time.Second,
	}, nil
}
