//go:build windows

package app

func hasInstallPrivileges() bool {
	// Windows installer enforces elevation before invoking the binary.
	return true
}

func installPrivilegeHint() string {
	return "run PowerShell as Administrator"
}
