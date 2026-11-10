//go:build windows

package fsown

// MatchOwner is a no-op on Windows (ACLs are inherited from the directory).
func MatchOwner(path, dir string) error { return nil }
