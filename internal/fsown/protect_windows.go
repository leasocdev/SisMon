//go:build windows

package fsown

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// protectedSDDL grants full control to SYSTEM, the Administrators group and
// the given account, inherited by everything below, and blocks inheritance
// from the parent (C:\ProgramData lets every local user read and create files).
func protectedSDDL(sid string) string {
	s := "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	if sid != "S-1-5-18" && sid != "S-1-5-32-544" {
		s += "(A;OICI;FA;;;" + sid + ")"
	}
	return s
}

// Protect restricts dir (and, through inheritance, everything in it) to
// SYSTEM, Administrators and the current account. It does nothing when the
// directory already has exactly that access list.
func Protect(dir string) error {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return err
	}
	want, err := windows.SecurityDescriptorFromString(protectedSDDL(u.User.Sid.String()))
	if err != nil {
		return err
	}
	cur, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err == nil && cur.String() == want.String() {
		return nil
	}
	dacl, _, err := want.DACL()
	if err != nil {
		return err
	}
	// SetNamedSecurityInfo also propagates the inheritable entries to the
	// files and folders already in dir.
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("drepturile directorului de date %s: %w", dir, err)
	}
	return nil
}
