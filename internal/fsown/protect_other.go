//go:build !windows

package fsown

// Protect is a no-op outside Windows: the data directory is created with
// mode 0750 and the systemd unit runs with UMask=0077.
func Protect(dir string) error { return nil }
