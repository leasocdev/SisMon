//go:build !windows

package svc

// SetupConsoleUTF8 is a no-op outside Windows.
func SetupConsoleUTF8() {}
