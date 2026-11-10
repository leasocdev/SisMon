package webui

import (
	"io/fs"
	"testing"
)

func TestWebuiFSNilWithoutIndex(t *testing.T) {
	f := FS()
	if _, err := fs.Stat(dist, "dist/index.html"); err != nil {
		if f != nil {
			t.Fatal("FS must be nil without index.html")
		}
		return
	}
	if f == nil {
		t.Fatal("FS must be non-nil when index.html is embedded")
	}
	if _, err := fs.ReadFile(f, "index.html"); err != nil {
		t.Fatal(err)
	}
}
