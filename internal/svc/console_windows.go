//go:build windows

package svc

import "golang.org/x/sys/windows"

// SetupConsoleUTF8 switches the console code page to UTF-8.
func SetupConsoleUTF8() {
	windows.SetConsoleOutputCP(65001)
	windows.SetConsoleCP(65001)
}
