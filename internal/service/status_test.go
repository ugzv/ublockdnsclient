package service

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ugzv/ublockdnsclient/internal/state"
)

func TestCurrentStatusIncludesProbeFailureDetails(t *testing.T) {
	oldServiceState := serviceStateFunc
	oldSystemDNS := systemDNSFunc
	oldProbe := localDNSProbeFunc
	oldLoadInstallState := loadInstallState
	oldHostDNS := hostDNSFunc
	oldCommandOutput := commandOutputFunc
	oldReadFile := readFileFunc
	t.Cleanup(func() {
		serviceStateFunc = oldServiceState
		systemDNSFunc = oldSystemDNS
		localDNSProbeFunc = oldProbe
		loadInstallState = oldLoadInstallState
		hostDNSFunc = oldHostDNS
		commandOutputFunc = oldCommandOutput
		readFileFunc = oldReadFile
	})

	serviceStateFunc = func() (string, error) { return "running", nil }
	systemDNSFunc = func() []string { return []string{"127.0.0.1"} }
	localDNSProbeFunc = func() error { return errors.New("udp timeout") }
	loadInstallState = func() (state.InstallState, error) { return state.InstallState{}, errors.New("missing") }
	hostDNSFunc = func() []string { return []string{"127.0.0.1"} }
	commandOutputFunc = func(name string, args ...string) ([]byte, error) {
		return []byte("Global: 127.0.0.1\n"), nil
	}
	readFileFunc = func(name string) ([]byte, error) {
		return []byte("nameserver 127.0.0.1\n"), nil
	}

	info := CurrentStatus()
	if info.Ready {
		t.Fatalf("expected not ready status, got %+v", info)
	}
	if info.ReadyCode != "local_dns_probe_failed" {
		t.Fatalf("expected ready_code local_dns_probe_failed, got %+v", info)
	}
	if !strings.Contains(info.ProbeError, "udp timeout") {
		t.Fatalf("expected probe error to be preserved, got %+v", info)
	}
	if info.Status != "inactive" {
		t.Fatalf("expected inactive status, got %+v", info)
	}
}

func TestCurrentStatusMarksReadyAfterSuccessfulProbe(t *testing.T) {
	oldServiceState := serviceStateFunc
	oldSystemDNS := systemDNSFunc
	oldProbe := localDNSProbeFunc
	oldLoadInstallState := loadInstallState
	oldHostDNS := hostDNSFunc
	oldCommandOutput := commandOutputFunc
	oldReadFile := readFileFunc
	t.Cleanup(func() {
		serviceStateFunc = oldServiceState
		systemDNSFunc = oldSystemDNS
		localDNSProbeFunc = oldProbe
		loadInstallState = oldLoadInstallState
		hostDNSFunc = oldHostDNS
		commandOutputFunc = oldCommandOutput
		readFileFunc = oldReadFile
	})

	serviceStateFunc = func() (string, error) { return "running", nil }
	systemDNSFunc = func() []string { return []string{"127.0.0.1"} }
	localDNSProbeFunc = func() error { return nil }
	loadInstallState = func() (state.InstallState, error) { return state.InstallState{}, errors.New("missing") }
	hostDNSFunc = func() []string { return []string{"127.0.0.1"} }
	commandOutputFunc = func(name string, args ...string) ([]byte, error) {
		return []byte("Global: 127.0.0.1\n"), nil
	}
	readFileFunc = func(name string) ([]byte, error) {
		return []byte("nameserver 127.0.0.1\n"), nil
	}

	info := CurrentStatus()
	if !info.Ready {
		t.Fatalf("expected ready status, got %+v", info)
	}
	if info.ReadyCode != "ready" {
		t.Fatalf("expected ready_code ready, got %+v", info)
	}
	if info.Status != "active" {
		t.Fatalf("expected active status, got %+v", info)
	}
}

func TestWaitUntilReadyReturnsReadinessFailure(t *testing.T) {
	oldStatus := currentStatusFunc
	oldNow := nowFunc
	oldSleep := sleepFunc
	t.Cleanup(func() {
		currentStatusFunc = oldStatus
		nowFunc = oldNow
		sleepFunc = oldSleep
	})

	now := time.Unix(0, 0)
	currentStatusFunc = func() StatusInfo {
		return StatusInfo{
			Ready:       false,
			Status:      "inactive",
			Service:     "running",
			LocalDNS:    true,
			ReadyCode:   "local_dns_probe_failed",
			ReadyDetail: "System DNS points to 127.0.0.1, but the local proxy did not answer a DNS probe.",
			ProbeError:  "probe failed",
		}
	}
	nowFunc = func() time.Time { return now }
	sleepFunc = func(time.Duration) {
		now = now.Add(30 * time.Millisecond)
	}

	info, err := WaitUntilReady(50 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error when DNS probe keeps failing")
	}
	if !strings.Contains(err.Error(), "did not answer a DNS probe") {
		t.Fatalf("expected probe failure in error, got %v", err)
	}
	if info.Ready {
		t.Fatalf("expected last status to remain not ready, got %+v", info)
	}
}

func TestWaitUntilReadySucceedsAfterProbePasses(t *testing.T) {
	oldStatus := currentStatusFunc
	oldNow := nowFunc
	oldSleep := sleepFunc
	t.Cleanup(func() {
		currentStatusFunc = oldStatus
		nowFunc = oldNow
		sleepFunc = oldSleep
	})

	now := time.Unix(0, 0)
	statusCalls := 0
	currentStatusFunc = func() StatusInfo {
		statusCalls++
		if statusCalls < 2 {
			return StatusInfo{
				Ready:       false,
				Status:      "inactive",
				Service:     "running",
				LocalDNS:    true,
				ReadyCode:   "local_dns_probe_failed",
				ReadyDetail: "System DNS points to 127.0.0.1, but the local proxy did not answer a DNS probe.",
				ProbeError:  "not yet",
			}
		}
		return StatusInfo{Ready: true, Status: "active", Service: "running", LocalDNS: true, ReadyCode: "ready"}
	}
	nowFunc = func() time.Time { return now }
	sleepFunc = func(time.Duration) {
		now = now.Add(10 * time.Millisecond)
	}

	info, err := WaitUntilReady(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Ready {
		t.Fatalf("expected ready status, got %+v", info)
	}
	if statusCalls != 2 {
		t.Fatalf("expected 2 readiness checks, got %d", statusCalls)
	}
}

func TestAssessLinuxResolverDNSPrefersAuthoritativeLocalDNS(t *testing.T) {
	oldHostDNS := hostDNSFunc
	oldCommandOutput := commandOutputFunc
	oldReadFile := readFileFunc
	t.Cleanup(func() {
		hostDNSFunc = oldHostDNS
		commandOutputFunc = oldCommandOutput
		readFileFunc = oldReadFile
	})

	hostDNSFunc = func() []string { return []string{"8.8.8.8", "8.8.4.4"} }
	commandOutputFunc = func(name string, args ...string) ([]byte, error) {
		if name == "resolvectl" && len(args) == 1 && args[0] == "dns" {
			return []byte("Global: 127.0.0.1\nLink 2 (wlan0): 192.168.2.1\n"), nil
		}
		return nil, errors.New("unexpected command")
	}
	readFileFunc = func(name string) ([]byte, error) {
		if name == "/etc/resolv.conf" {
			return []byte("nameserver 127.0.0.1\n"), nil
		}
		return nil, errors.New("unexpected file")
	}

	assessment := assessLinuxResolverDNS()
	if !assessment.LocalDNS {
		t.Fatalf("expected local DNS assessment, got %+v", assessment)
	}
	if !sameDNSSet(assessment.DNS, []string{"127.0.0.1", "192.168.2.1"}) {
		t.Fatalf("expected resolvectl DNS to win, got %+v", assessment)
	}
	if len(assessment.Warnings) == 0 {
		t.Fatalf("expected disagreement warning, got %+v", assessment)
	}
	if !strings.Contains(assessment.Warnings[0], "upstream DNS") {
		t.Fatalf("expected upstream metadata warning, got %+v", assessment)
	}
}

func TestAssessLinuxResolverDNSFallsBackToResolvConf(t *testing.T) {
	oldHostDNS := hostDNSFunc
	oldCommandOutput := commandOutputFunc
	oldReadFile := readFileFunc
	t.Cleanup(func() {
		hostDNSFunc = oldHostDNS
		commandOutputFunc = oldCommandOutput
		readFileFunc = oldReadFile
	})

	hostDNSFunc = func() []string { return []string{"8.8.8.8"} }
	commandOutputFunc = func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("resolvectl unavailable")
	}
	readFileFunc = func(name string) ([]byte, error) {
		return []byte("# managed\nnameserver 127.0.0.1\n"), nil
	}

	assessment := assessLinuxResolverDNS()
	if !assessment.LocalDNS {
		t.Fatalf("expected resolv.conf local DNS assessment, got %+v", assessment)
	}
	if !sameDNSSet(assessment.DNS, []string{"127.0.0.1"}) {
		t.Fatalf("expected resolv.conf DNS, got %+v", assessment)
	}
}

func TestCurrentStatusUsesLinuxResolverAssessment(t *testing.T) {
	oldServiceState := serviceStateFunc
	oldSystemDNS := systemDNSFunc
	oldProbe := localDNSProbeFunc
	oldLoadInstallState := loadInstallState
	oldHostDNS := hostDNSFunc
	oldCommandOutput := commandOutputFunc
	oldReadFile := readFileFunc
	t.Cleanup(func() {
		serviceStateFunc = oldServiceState
		systemDNSFunc = oldSystemDNS
		localDNSProbeFunc = oldProbe
		loadInstallState = oldLoadInstallState
		hostDNSFunc = oldHostDNS
		commandOutputFunc = oldCommandOutput
		readFileFunc = oldReadFile
	})

	serviceStateFunc = func() (string, error) { return "running", nil }
	systemDNSFunc = func() []string { return []string{"8.8.8.8", "8.8.4.4"} }
	localDNSProbeFunc = func() error { return nil }
	loadInstallState = func() (state.InstallState, error) { return state.InstallState{}, errors.New("missing") }
	hostDNSFunc = func() []string { return []string{"8.8.8.8", "8.8.4.4"} }
	commandOutputFunc = func(name string, args ...string) ([]byte, error) {
		if name == "resolvectl" {
			return []byte("Global: 127.0.0.1\n"), nil
		}
		return nil, errors.New("unexpected command")
	}
	readFileFunc = func(name string) ([]byte, error) {
		return []byte("nameserver 127.0.0.1\n"), nil
	}

	info := CurrentStatus()
	if runtime.GOOS == "linux" {
		if !info.Ready || info.Status != "active" {
			t.Fatalf("expected linux assessment to mark active, got %+v", info)
		}
		if !info.LocalDNS {
			t.Fatalf("expected local DNS true, got %+v", info)
		}
		if !sameDNSSet(info.SystemDNS, []string{"127.0.0.1"}) {
			t.Fatalf("expected authoritative local DNS, got %+v", info)
		}
	}
}
