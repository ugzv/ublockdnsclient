package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func swapAutoUpdateHooks(t *testing.T, apply func(string, string) (string, error)) (relaunched *atomic.Bool) {
	t.Helper()
	relaunched = &atomic.Bool{}
	oldApply, oldRelaunch, oldCan, oldDelay := applyUpdateFunc, relaunchFunc, canRelaunchFunc, autoUpdateDelay
	applyUpdateFunc = apply
	relaunchFunc = func() { relaunched.Store(true) }
	canRelaunchFunc = func() bool { return true }
	autoUpdateDelay = func(bool) time.Duration { return time.Millisecond }
	t.Cleanup(func() {
		applyUpdateFunc, relaunchFunc, canRelaunchFunc, autoUpdateDelay = oldApply, oldRelaunch, oldCan, oldDelay
	})
	return relaunched
}

func TestWatchUpdatesRelaunchesAfterUpdate(t *testing.T) {
	relaunched := swapAutoUpdateHooks(t, func(current, api string) (string, error) {
		return "2.0.0", nil
	})

	done := make(chan struct{})
	go func() {
		watchUpdates(context.Background(), "1.0.0", "")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchUpdates did not return after applying update")
	}
	if !relaunched.Load() {
		t.Fatal("expected relaunch after update")
	}
}

func TestWatchUpdatesKeepsPollingOnErrorAndNoop(t *testing.T) {
	var calls atomic.Int32
	relaunched := swapAutoUpdateHooks(t, func(current, api string) (string, error) {
		if calls.Add(1) == 1 {
			return "", errors.New("network down")
		}
		return "", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchUpdates(ctx, "1.0.0", "")
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if calls.Load() < 3 {
		t.Fatalf("calls = %d, want polling to continue after error and no-op", calls.Load())
	}
	if relaunched.Load() {
		t.Fatal("must not relaunch without an applied update")
	}
}

func TestWatchUpdatesSkipsDevBuilds(t *testing.T) {
	relaunched := swapAutoUpdateHooks(t, func(string, string) (string, error) {
		t.Error("dev builds must not check for updates")
		return "", nil
	})

	done := make(chan struct{})
	go func() {
		watchUpdates(context.Background(), "dev", "")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchUpdates should return immediately for dev builds")
	}
	if relaunched.Load() {
		t.Fatal("must not relaunch")
	}
}

func TestWatchUpdatesHonorsOptOut(t *testing.T) {
	t.Setenv("UBLOCKDNS_NO_AUTOUPDATE", "1")
	swapAutoUpdateHooks(t, func(string, string) (string, error) {
		t.Error("opted-out daemon must not check for updates")
		return "", nil
	})

	done := make(chan struct{})
	go func() {
		watchUpdates(context.Background(), "1.0.0", "")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchUpdates should return immediately when opted out")
	}
}
