package runtime

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nextdns/nextdns/resolver/endpoint"
)

func TestBootstrapRefresherDedupesAndSwapsEndpoint(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	old := resolveBootstrapIPsFunc
	resolveBootstrapIPsFunc = func(context.Context, string) ([]string, error) {
		calls.Add(1)
		<-release
		return []string{"9.9.9.9"}, nil
	}
	t.Cleanup(func() { resolveBootstrapIPsFunc = old })

	var ptr atomic.Pointer[endpoint.DOHEndpoint]
	ptr.Store(&endpoint.DOHEndpoint{Hostname: "h", Path: "/p", Bootstrap: []string{"1.1.1.1"}})
	rec := &recordingEndpoint{}
	mgr := newEndpointManager(rec, endpoint.StaticProvider([]endpoint.Endpoint{rec}))
	r := &bootstrapRefresher{hostname: "h", path: "/p", ep: &ptr, mgr: mgr}

	r.refresh(context.Background())
	r.refresh(context.Background()) // coalesced while the first is in flight
	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for r.busy.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("resolve calls = %d, want 1 (overlapping refresh must coalesce)", calls.Load())
	}
	if got := ptr.Load().Bootstrap; !slices.Equal(got, []string{"9.9.9.9"}) {
		t.Fatalf("bootstrap = %v, want swapped to [9.9.9.9]", got)
	}
}

func TestBootstrapRefresherCancellationPreservesEndpoint(t *testing.T) {
	started := make(chan struct{})
	old := resolveBootstrapIPsFunc
	resolveBootstrapIPsFunc = func(ctx context.Context, _ string) ([]string, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() { resolveBootstrapIPsFunc = old })
	var ptr atomic.Pointer[endpoint.DOHEndpoint]
	original := &endpoint.DOHEndpoint{Hostname: "h", Bootstrap: []string{"1.1.1.1"}}
	ptr.Store(original)
	r := &bootstrapRefresher{hostname: "h", ep: &ptr}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.refresh(ctx)
	<-started
	cancel()
	deadline := time.Now().Add(time.Second)
	for r.busy.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if r.busy.Load() {
		t.Fatal("canceled refresh is still running")
	}
	if ptr.Load() != original {
		t.Fatal("canceled refresh replaced the current endpoint")
	}
}
