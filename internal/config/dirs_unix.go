//go:build !windows

package config

// DefaultDataDir is /var/lib/netwatch.
func DefaultDataDir() string { return "/var/lib/netwatch" }
