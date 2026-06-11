package service

import (
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

func systemdUnitPath() string {
	return "/etc/systemd/system/" + core.ServiceName + ".service"
}

func hasRestartAlways(unit string) bool {
	return strings.Contains(unit, "\nRestart=always\n")
}

// RelaunchAfterUpdateSupported reports whether the service manager restarts
// the daemon after its process exits, which is how auto-update relaunches the
// new binary: launchd via KeepAlive, Windows SCM via recovery actions, and
// systemd only once the unit carries Restart=always.
func RelaunchAfterUpdateSupported() bool {
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	case "linux":
		b, err := os.ReadFile(systemdUnitPath())
		return err == nil && hasRestartAlways(string(b))
	}
	return false
}

// ensureSystemdRestartPolicy fixes the unit written by the embedded nextdns
// library, which sets RestartSec without Restart= (so the daemon would stay
// down after exiting). Restart=always is also what lets auto-update relaunch
// the new binary by exiting.
func ensureSystemdRestartPolicy() {
	if runtime.GOOS != "linux" {
		return
	}
	path := systemdUnitPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	patched, changed := patchSystemdUnit(string(b))
	if !changed {
		return
	}
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		log.Printf("Warning: failed to patch systemd restart policy: %v", err)
		return
	}
	if err := core.RunCommand("systemctl", "daemon-reload"); err != nil {
		log.Printf("Warning: systemctl daemon-reload failed: %v", err)
	}
}

func patchSystemdUnit(unit string) (string, bool) {
	if strings.Contains(unit, "\nRestart=") {
		return unit, false
	}
	const orig = "\nRestartSec=120\n"
	if !strings.Contains(unit, orig) {
		return unit, false
	}
	return strings.Replace(unit, orig, "\nRestart=always\nRestartSec=2\n", 1), true
}
