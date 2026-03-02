//go:build windows

package main

func hasInstallPrivileges() bool {
	// Windows installer enforces elevation before invoking the binary.
	return true
}

func installPrivilegeHint() string {
	return "run PowerShell as Administrator"
}
