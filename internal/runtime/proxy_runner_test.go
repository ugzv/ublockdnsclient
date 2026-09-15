package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nextdns/nextdns/proxy"
)

func TestProxyRunnerStopWithoutStart(t *testing.T) {
	t.Parallel()

	runner := &proxyRunner{}
	if err := runner.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestProxyRunnerStartReturnsImmediateError(t *testing.T) {
	t.Parallel()

	runner := &proxyRunner{
		proxy: proxy.Proxy{
			Addrs: []string{"not-a-valid-listen-address"},
		},
	}

	err := runner.Start()
	if err == nil {
		t.Fatal("expected Start() to fail for invalid listen address")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProxyRunnerStartFailureSkipsOnReady(t *testing.T) {
	t.Parallel()

	var readyCalled atomic.Bool
	runner := &proxyRunner{
		proxy: proxy.Proxy{
			Addrs: []string{"not-a-valid-listen-address"},
		},
		onReady: []func(context.Context){
			func(context.Context) { readyCalled.Store(true) },
		},
	}

	if err := runner.Start(); err == nil {
		t.Fatal("expected Start() to fail for invalid listen address")
	}
	if readyCalled.Load() {
		t.Fatal("onReady hooks must not run when startup fails immediately")
	}
}

// Delay the actual bind until after Start returns. This exercises a serving
// failure after the startup grace period without external DNS or privileged ports.
func TestProxyRunnerLateFailureCancelsReadyAndWaitsForCleanup(t *testing.T) {
	t.Parallel()
	releaseBind := make(chan struct{})
	ready := make(chan struct{})
	cancelled := make(chan struct{})
	cleanup := make(chan struct{})
	runner := &proxyRunner{
		proxy: proxy.Proxy{Addrs: []string{"invalid-address"}, InfoLog: func(string) { <-releaseBind }},
		onReady: []func(context.Context){func(ctx context.Context) {
			close(ready)
			<-ctx.Done()
			close(cancelled)
			<-cleanup
		}},
	}
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	<-ready
	close(releaseBind)
	var cleanupOnce sync.Once
	releaseCleanup := func() { cleanupOnce.Do(func() { close(cleanup) }) }
	defer releaseCleanup()
	defer runner.cancel()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("serving failure did not cancel ready hooks")
	}
	select {
	case <-runner.stopped:
		t.Fatal("runner reported stopped before DNS cleanup completed")
	default:
	}
	// Release cleanup before Stop; the stopped channel must include hook cleanup.
	releaseCleanup()
	if err := runner.Stop(); err == nil {
		t.Fatal("Stop must surface the serving failure")
	}
}

func TestProxyRunnerStopWaitsForReadyCleanup(t *testing.T) {
	t.Parallel()
	ready := make(chan struct{})
	cancelled := make(chan struct{})
	cleanup := make(chan struct{})
	runner := &proxyRunner{
		proxy:   proxyServeFunc(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }),
		onReady: []func(context.Context){func(ctx context.Context) { close(ready); <-ctx.Done(); close(cancelled); <-cleanup }},
	}
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	<-ready
	stopped := make(chan error, 1)
	go func() { stopped <- runner.Stop() }()
	<-cancelled
	select {
	case err := <-stopped:
		close(cleanup)
		t.Fatalf("Stop returned before cleanup: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(cleanup)
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("normal Stop returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not finish after cleanup")
	}
}

type proxyServeFunc func(context.Context) error

func (f proxyServeFunc) ListenAndServe(ctx context.Context) error { return f(ctx) }
