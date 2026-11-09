//go:build windows

package config

import (
	"os"
	"path/filepath"
)

// DefaultDataDir is %ProgramData%\NetWatch.
func DefaultDataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "NetWatch")
}
