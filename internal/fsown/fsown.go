// Package fsown gives files created by a root CLI command the owner of the
// data directory, so the service account can still write them.
package fsown

import (
	"io/fs"
	"path/filepath"
)

// MatchTree applies MatchOwner to every entry under dir.
func MatchTree(dir string) error {
	return filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return err
		}
		return MatchOwner(p, dir)
	})
}
