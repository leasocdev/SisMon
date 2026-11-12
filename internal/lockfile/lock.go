// Package lockfile prevents two instances from using one data directory.
package lockfile

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrLocked means another process holds the lock.
var ErrLocked = errors.New("directorul de date este folosit de o instanță activă")

// Acquire takes an exclusive lock on <dir>/sismon.lock. The file is never
// written, so it does not grow.
func Acquire(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "sismon.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lock(f); err != nil {
		f.Close()
		return nil, err
	}
	return func() { unlock(f); f.Close() }, nil
}
