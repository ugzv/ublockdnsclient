package service

import (
	"errors"
	"testing"
)

// `ublockdns status` without sudo reported "unknown" and a warning on a
// healthy macOS install, because only root got an answer about the service.
func TestLaunchdStateWithoutRoot(t *testing.T) {
	running := "system/ublockdns = {\n\tactive count = 1\n\tstate = running\n\n\tprogram = /usr/local/bin/ublockdns\n\tpid = 33101\n\tspawn = {\n\t\tstate = active\n\t}\n}\n"
	loadedNotRunning := "system/ublockdns = {\n\tactive count = 0\n\tstate = not running\n}\n"
	notLoaded := errors.New("exit status 113")

	for _, tc := range []struct {
		name        string
		out         string
		err         error
		plistExists bool
		want        string
	}{
		{"running", running, nil, true, "running"},
		{"loaded but not running", loadedNotRunning, nil, true, "stopped"},
		{"unloaded with service file", "", notLoaded, true, "stopped"},
		{"unloaded without service file", "", notLoaded, false, "not-installed"},
		{"unreadable output", "something else\n", nil, true, "unknown"},
	} {
		if got := launchdStateFromPrint(tc.out, tc.err, tc.plistExists); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
