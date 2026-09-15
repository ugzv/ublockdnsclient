package service

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nextdns/nextdns/host/service"
	"github.com/ugzv/ublockdnsclient/internal/state"
)

func TestWaitForLocalDNSProxyRetriesUntilProxyBinds(t *testing.T) {
	var probes atomic.Int32
	now := time.Now()
	withStatusTestEnv(t, statusTestEnv{
		localDNSProbe: func(...string) error {
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
		localDNSProbe: func(...string) error { return want },
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

type rollbackService struct {
	fakeService
	installErr error
	starts     int
}

func (s *rollbackService) Install() error { return s.installErr }
func (s *rollbackService) Start() error   { s.starts++; return s.startErr }

func TestRollbackRestoresOnlyAWorkingPreviousService(t *testing.T) {
	for _, tt := range []struct {
		name                           string
		running, known                 bool
		installErr, startErr, probeErr error
		wantStarts                     int
		wantLocal                      bool
	}{
		{name: "previously stopped", known: true},
		{name: "missing saved configuration", running: true},
		{name: "reinstall failed", running: true, known: true, installErr: errors.New("install failed")},
		{name: "restart failed", running: true, known: true, startErr: errors.New("start failed"), wantStarts: 1},
		{name: "proxy never answered", running: true, known: true, probeErr: errors.New("no listener"), wantStarts: 1},
		{name: "recovered", running: true, known: true, wantStarts: 1, wantLocal: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			previous := &rollbackService{fakeService: fakeService{startErr: tt.startErr}, installErr: tt.installErr}
			oldFactory, oldRestore := newHostService, restoreSystemDNSStrictFunc
			t.Cleanup(func() { newHostService = oldFactory; restoreSystemDNSStrictFunc = oldRestore })
			newHostService = func(config service.Config) (service.Service, error) { return previous, nil }
			restores, activations := 0, 0
			restoreSystemDNSStrictFunc = func() error { restores++; return nil }
			withControlTestEnv(t, controlTestEnv{
				baseService:                func() (service.Service, error) { return fakeService{}, nil },
				restoreSystemDNSBestEffort: func() {},
				activatePlatformSystemDNS:  func() error { activations++; return nil },
			})
			now := time.Now()
			withStatusTestEnv(t, statusTestEnv{
				localDNSProbe: func(...string) error { return tt.probeErr },
				now:           func() time.Time { return now },
				sleep:         func(d time.Duration) { now = now.Add(d) },
			})
			err := rollbackInstall(installPreconditions{installed: true, running: tt.running}, []string{"127.0.0.1:53", "[::1]:53"}, tt.known, state.InstallState{ProfileID: "previous"})
			wantErr := !tt.known || tt.installErr != nil || tt.startErr != nil || tt.probeErr != nil
			if (err != nil) != wantErr {
				t.Errorf("rollback error = %v, want error %v", err, wantErr)
			}
			if previous.starts != tt.wantStarts {
				t.Errorf("starts = %d, want %d", previous.starts, tt.wantStarts)
			}
			if (activations > 0) != tt.wantLocal {
				t.Errorf("DNS activations = %d, want local %v", activations, tt.wantLocal)
			}
			if !tt.wantLocal && restores == 0 {
				t.Error("failed recovery did not restore upstream DNS")
			}
		})
	}
}
