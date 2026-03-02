//go:build !windows

package main

import "os"

func hasInstallPrivileges() bool {
	return os.Geteuid() == 0
}

func installPrivilegeHint() string {
	return "run with sudo"
}
