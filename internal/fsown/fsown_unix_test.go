//go:build !windows

package fsown

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Review fix: files created by a root CLI get the data directory's owner.
func TestMatchOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	dir := t.TempDir()
	os.Chown(dir, 65534, 65534)
	f := filepath.Join(dir, "x.db")
	os.WriteFile(f, []byte("x"), 0o600)
	if err := MatchOwner(f, dir); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(f)
	if st := fi.Sys().(*syscall.Stat_t); st.Uid != 65534 || st.Gid != 65534 {
		t.Fatalf("owner %d:%d", st.Uid, st.Gid)
	}
}
