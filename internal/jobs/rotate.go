package jobs

import (
	"fmt"
	"sort"
	"time"
)

// Rotate returns the automatic backups to delete, keeping the newest one for
// each of the last 7 days, 4 ISO weeks and 12 months. Other kinds are kept.
func Rotate(all []BackupInfo, now time.Time) []BackupInfo {
	var autos []BackupInfo
	for _, b := range all {
		if b.Kind == "auto" {
			autos = append(autos, b)
		}
	}
	sort.Slice(autos, func(i, j int) bool { return autos[i].Created > autos[j].Created })
	keep := map[string]bool{}
	tiers := []struct {
		n   int
		key func(time.Time) string
	}{
		{7, func(t time.Time) string { return t.Format("2006-01-02") }},
		{4, func(t time.Time) string { y, w := t.ISOWeek(); return fmt.Sprintf("%d-W%02d", y, w) }},
		{12, func(t time.Time) string { return t.Format("2006-01") }},
	}
	for _, tier := range tiers {
		seen := map[string]bool{}
		for _, b := range autos {
			k := tier.key(time.UnixMilli(b.Created).In(Zone))
			if seen[k] {
				continue
			}
			if len(seen) >= tier.n {
				break
			}
			seen[k] = true
			keep[b.Name] = true
		}
	}
	var remove []BackupInfo
	for _, b := range autos {
		if !keep[b.Name] {
			remove = append(remove, b)
		}
	}
	return remove
}
