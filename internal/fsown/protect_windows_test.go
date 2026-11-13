//go:build windows

package fsown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// Review fix: the data directory must not inherit "Users: read / create
// files" from C:\ProgramData; existing files are covered too.
func TestProtectRemovesUsersAccess(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sismon.db")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Protect(dir); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dir, f} {
		sd, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		s := sd.String()
		if strings.Contains(s, ";BU)") || strings.Contains(s, ";S-1-5-32-545)") || strings.Contains(s, ";WD)") || strings.Contains(s, ";AU)") {
			t.Fatalf("%s still grants access to all users: %s", p, s)
		}
	}
	sd, _ := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if !strings.HasPrefix(sd.String(), "D:P") {
		t.Fatalf("inheritance not blocked: %s", sd)
	}
	if b, err := os.ReadFile(f); err != nil || string(b) != "x" {
		t.Fatalf("current user lost access: %v", err)
	}
	u, _ := windows.GetCurrentProcessToken().GetTokenUser()
	want, _ := windows.SecurityDescriptorFromString(protectedSDDL(u.User.Sid.String()))
	if sd.String() != want.String() {
		t.Fatalf("access list %s, want %s (Protect would rewrite it at every start)", sd, want)
	}
	if err := Protect(dir); err != nil { // idempotent
		t.Fatal(err)
	}
}
