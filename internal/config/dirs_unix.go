//go:build !windows

package config

// DefaultDataDir is /var/lib/sismon.
func DefaultDataDir() string { return "/var/lib/sismon" }
