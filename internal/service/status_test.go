package service

import (
	"errors"
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
	t.Cleanup(func() {
		serviceStateFunc = oldServiceState
		systemDNSFunc = oldSystemDNS
		localDNSProbeFunc = oldProbe
		loadInstallState = oldLoadInstallState
	})

	serviceStateFunc = func() (string, error) { return "running", nil }
	systemDNSFunc = func() []string { return []string{"127.0.0.1"} }
	localDNSProbeFunc = func() error { return errors.New("udp timeout") }
	loadInstallState = func() (state.InstallState, error) { return state.InstallState{}, errors.New("missing") }

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
	t.Cleanup(func() {
		serviceStateFunc = oldServiceState
		systemDNSFunc = oldSystemDNS
		localDNSProbeFunc = oldProbe
		loadInstallState = oldLoadInstallState
	})

	serviceStateFunc = func() (string, error) { return "running", nil }
	systemDNSFunc = func() []string { return []string{"127.0.0.1"} }
	localDNSProbeFunc = func() error { return nil }
	loadInstallState = func() (state.InstallState, error) { return state.InstallState{}, errors.New("missing") }

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
