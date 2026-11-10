//go:build !windows

package fsown

import (
	"os"
	"syscall"
)

// MatchOwner chowns path to dir's owner when running as root.
func MatchOwner(path, dir string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Lchown(path, int(st.Uid), int(st.Gid))
}
