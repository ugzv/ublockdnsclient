package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

func TestManageSystemDNSActivatesAndDeactivates(t *testing.T) {
	var activated, deactivated atomic.Bool
	t.Cleanup(core.SwapPlatformSystemDNSFuncs(
		func() error {
			activated.Store(true)
			return nil
		},
		func() error { return nil },
	))
	t.Cleanup(core.SwapSystemDNSFuncs(
		nil,
		func() error {
			deactivated.Store(true)
			return nil
		},
	))

	oldWatch := watchNetworkDNSChanges
	watchNetworkDNSChanges = func(ctx context.Context, _ func(context.Context)) {
		<-ctx.Done()
	}
	t.Cleanup(func() { watchNetworkDNSChanges = oldWatch })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manageSystemDNS(ctx, nil)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for !activated.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !activated.Load() {
		t.Fatal("expected system DNS activation on start")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manageSystemDNS did not stop after context cancellation")
	}
	if !deactivated.Load() {
		t.Fatal("expected system DNS deactivation on stop")
	}
}

func TestDefaultWatchNetworkDNSChangesReactivatesOnChange(t *testing.T) {
	var activations, changeCallbacks atomic.Int32
	t.Cleanup(core.SwapPlatformSystemDNSFuncs(
		func() error {
			activations.Add(1)
			return nil
		},
		nil,
	))

	oldWatch := watchNetworkChanges
	watchNetworkChanges = func(ctx context.Context, changes chan<- string) {
		select {
		case changes <- "en0 up":
		case <-ctx.Done():
		}
		<-ctx.Done()
	}
	t.Cleanup(func() { watchNetworkChanges = oldWatch })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defaultWatchNetworkDNSChanges(ctx, func(context.Context) { changeCallbacks.Add(1) })
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for activations.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if activations.Load() != 1 {
		t.Fatalf("activation count = %d, want 1 re-activation on network change", activations.Load())
	}
	if changeCallbacks.Load() != 1 {
		t.Fatalf("onChange callback count = %d, want 1", changeCallbacks.Load())
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("defaultWatchNetworkDNSChanges did not stop after context cancellation")
	}
}

func TestWatchNetworkChangesStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchNetworkChanges(ctx, nil)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchNetworkChanges did not stop after context cancellation")
	}
}
