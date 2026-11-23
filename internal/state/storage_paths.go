package state

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EnsureStateDir creates the state directory, readable only by root, or by
// SYSTEM and Administrators on Windows.
func EnsureStateDir() error {
	return secureStateDir(tokenDir())
}

func tokenDir() string {
	if runtime.GOOS == "windows" {
		if programData := strings.TrimSpace(os.Getenv("ProgramData")); programData != "" {
			return filepath.Join(programData, "ublockdns")
		}
		return filepath.Join(`C:\ProgramData`, "ublockdns")
	}
	return "/etc/ublockdns"
}

func DaemonLogPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(tokenDir(), "ublockdns.log")
	case "darwin":
		return "/Library/Logs/ublockdns.log"
	default:
		return "/var/log/ublockdns.log"
	}
}
