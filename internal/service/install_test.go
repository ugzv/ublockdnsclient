package service

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForLocalDNSProxyRetriesUntilProxyBinds(t *testing.T) {
	var probes atomic.Int32
	now := time.Now()
	withStatusTestEnv(t, statusTestEnv{
		localDNSProbe: func() error {
			// The proxy binds asynchronously; the first probes hit a closed port.
			if probes.Add(1) < 4 {
				return errors.New("connection refused")
			}
			return nil
		},
		now:   func() time.Time { return now },
		sleep: func(d time.Duration) { now = now.Add(d) },
	})

	if err := waitForLocalDNSProxy(localDNSPreflightTimeout); err != nil {
		t.Fatalf("waitForLocalDNSProxy() error = %v, want success once the proxy binds", err)
	}
	if got := probes.Load(); got != 4 {
		t.Fatalf("probed %d times, want 4", got)
	}
}

func TestWaitForLocalDNSProxyGivesUpAfterTimeout(t *testing.T) {
	want := errors.New("connection refused")
	now := time.Now()
	withStatusTestEnv(t, statusTestEnv{
		localDNSProbe: func() error { return want },
		now:           func() time.Time { return now },
		sleep:         func(d time.Duration) { now = now.Add(d) },
	})

	err := waitForLocalDNSProxy(50 * time.Millisecond)
	if !errors.Is(err, want) {
		t.Fatalf("waitForLocalDNSProxy() error = %v, want %v", err, want)
	}
}

// Every service manager errors when asked to stop something that is not
// running, and those errors reached users as warnings that read like install
// failures: systemd's "Unit ublockdns.service not loaded" on a fresh install,
// launchd's "Unload failed: 5: Input/output error" on an upgrade, because
// install.sh stops the service before handing over.
func TestAssessInstallPreconditions(t *testing.T) {
	tests := []struct {
		state         string
		err           error
		wantInstalled bool
		wantRunning   bool
		why           string
	}{
		{state: "running", wantInstalled: true, wantRunning: true, why: "an upgrade over a live service"},
		{state: "stopped", wantInstalled: true, wantRunning: false, why: "install.sh already stopped it"},
		{state: "not-installed", wantInstalled: false, wantRunning: false, why: "fresh install"},
		{state: "unknown", wantInstalled: true, wantRunning: false, why: "registered but state unreadable; do not guess it is running"},
		{state: "running", err: errors.New("launchctl unavailable"), why: "an error means we know nothing"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := assessInstallPreconditions(tt.state, tt.err)
			if got.installed != tt.wantInstalled {
				t.Errorf("installed = %v, want %v (%s)", got.installed, tt.wantInstalled, tt.why)
			}
			if got.running != tt.wantRunning {
				t.Errorf("running = %v, want %v (%s)", got.running, tt.wantRunning, tt.why)
			}
		})
	}
}
