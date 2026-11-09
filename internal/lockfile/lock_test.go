package lockfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLockExclusive(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		release, err := Acquire(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Acquire(dir); !errors.Is(err, ErrLocked) {
			t.Fatalf("second acquire: %v", err)
		}
		release()
	}
	fi, err := os.Stat(filepath.Join(dir, "netwatch.lock"))
	if err != nil || fi.Size() > 1 {
		t.Fatalf("lock file grew: %v %v", fi, err)
	}
}
