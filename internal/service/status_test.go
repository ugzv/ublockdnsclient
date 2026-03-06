package service

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitUntilReadyRequiresLocalDNSProbe(t *testing.T) {
	oldStatus := currentStatusFunc
	oldProbe := localDNSProbeFunc
	oldNow := nowFunc
	oldSleep := sleepFunc
	t.Cleanup(func() {
		currentStatusFunc = oldStatus
		localDNSProbeFunc = oldProbe
		nowFunc = oldNow
		sleepFunc = oldSleep
	})

	now := time.Unix(0, 0)
	currentStatusFunc = func() StatusInfo {
		return StatusInfo{Ready: true, Status: "active", Service: "running", LocalDNS: true}
	}
	localDNSProbeFunc = func() error { return errors.New("probe failed") }
	nowFunc = func() time.Time { return now }
	sleepFunc = func(time.Duration) {
		now = now.Add(30 * time.Millisecond)
	}

	info, err := WaitUntilReady(50 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error when DNS probe keeps failing")
	}
	if !strings.Contains(err.Error(), "local DNS probe failed") {
		t.Fatalf("expected probe failure in error, got %v", err)
	}
	if !info.Ready {
		t.Fatalf("expected last status to remain ready, got %+v", info)
	}
}

func TestWaitUntilReadySucceedsAfterProbePasses(t *testing.T) {
	oldStatus := currentStatusFunc
	oldProbe := localDNSProbeFunc
	oldNow := nowFunc
	oldSleep := sleepFunc
	t.Cleanup(func() {
		currentStatusFunc = oldStatus
		localDNSProbeFunc = oldProbe
		nowFunc = oldNow
		sleepFunc = oldSleep
	})

	now := time.Unix(0, 0)
	currentStatusFunc = func() StatusInfo {
		return StatusInfo{Ready: true, Status: "active", Service: "running", LocalDNS: true}
	}
	probeCalls := 0
	localDNSProbeFunc = func() error {
		probeCalls++
		if probeCalls < 2 {
			return errors.New("not yet")
		}
		return nil
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
	if probeCalls != 2 {
		t.Fatalf("expected 2 probe attempts, got %d", probeCalls)
	}
}
