//go:build !windows

package service

import (
	"os"
	"regexp"
	"runtime"

	"github.com/nextdns/nextdns/host/service"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

func platformServiceState() (string, error) {
	svc, err := baseService()
	if err != nil {
		return "unknown", err
	}
	st, err := svc.Status()
	if err != nil {
		// The launchd backend only answers root, but any user may ask
		// launchd about a system service.
		if runtime.GOOS == "darwin" {
			out, printErr := commandCombinedOutputFunc("launchctl", "print", "system/"+core.ServiceName)
			_, statErr := os.Stat(launchdPlistPath)
			if state := launchdStateFromPrint(string(out), printErr, statErr == nil); state != "unknown" {
				return state, nil
			}
		}
		return "unknown", err
	}
	return mapServiceStatus(st), nil
}

const launchdPlistPath = "/Library/LaunchDaemons/" + core.ServiceName + ".plist"

var launchdStateRE = regexp.MustCompile(`(?m)^\tstate = (.+)$`)

// launchdStateFromPrint reads the service state from `launchctl print`. A
// failed print means launchd has not loaded the service; the service file
// tells an installed but unloaded service from one that was never installed.
func launchdStateFromPrint(out string, printErr error, plistExists bool) string {
	if printErr != nil {
		if plistExists {
			return "stopped"
		}
		return "not-installed"
	}
	m := launchdStateRE.FindStringSubmatch(out)
	switch {
	case m == nil:
		return "unknown"
	case m[1] == "running":
		return "running"
	default:
		return "stopped"
	}
}

func mapServiceStatus(st service.Status) string {
	switch st {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	case service.StatusNotInstalled:
		return "not-installed"
	default:
		return "unknown"
	}
}
