// Package jobs runs background work: backups, aggregation, retention.
package jobs

// BackupInfo describes one backup file.
type BackupInfo struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Created   int64  `json:"created"`
	SizeBytes int64  `json:"size_bytes"`
}
