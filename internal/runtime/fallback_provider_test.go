package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nextdns/nextdns/resolver/endpoint"
)

func hasFallbackAddress(endpoints []endpoint.Endpoint, address string) bool {
	for _, ep := range endpoints {
		if ep.String() == address {
			return true
		}
	}
	return false
}

func TestWindowsFallbackCachesSlowDiscoveryAndRefreshesWithoutBlocking(t *testing.T) {
	var calls atomic.Int32
	refreshStarted := make(chan struct{})
	release := make(chan struct{})
	provider := newFallbackDNSProvider("windows", func(ctx context.Context) []string {
		if calls.Add(1) == 1 {
			select {
			case <-time.After(300 * time.Millisecond):
				return []string{"192.0.2.53"}
			case <-ctx.Done():
				return nil
			}
		}
		close(refreshStarted)
		select {
		case <-release:
			return []string{"192.0.2.54"}
		case <-ctx.Done():
			return nil
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	for range 10 {
		endpoints, err := provider.GetEndpoints(ctx)
		if err != nil || !hasFallbackAddress(endpoints, "192.0.2.53:53") {
			t.Fatalf("cached local resolver missing: endpoints=%v err=%v", endpoints, err)
		}
	}
	<-refreshStarted
	if calls.Load() != 2 {
		t.Fatalf("discover calls=%d, want one startup and one coalesced refresh", calls.Load())
	}
	close(release)
	waitFallbackRefresh(t, provider)
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.servers) != 1 || provider.servers[0] != "192.0.2.54" {
		t.Fatalf("cached servers=%v, want refreshed resolver", provider.servers)
	}
}

func TestWindowsFallbackPreservesCacheWhenRefreshFails(t *testing.T) {
	var calls atomic.Int32
	provider := newFallbackDNSProvider("windows", func(context.Context) []string {
		if calls.Add(1) == 1 {
			return []string{"192.0.2.53"}
		}
		return nil
	})
	if _, err := provider.GetEndpoints(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFallbackRefresh(t, provider)
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.servers) != 1 || provider.servers[0] != "192.0.2.53" {
		t.Fatalf("failed discovery erased cached resolver: %v", provider.servers)
	}
}

func TestNonWindowsFallbackDiscoveryRetainsShortBudget(t *testing.T) {
	var calls int
	provider := newFallbackDNSProvider("darwin", func(ctx context.Context) []string {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 250*time.Millisecond {
			t.Error("discovery exceeded recovery budget")
		}
		return []string{"192.0.2.53"}
	})
	if calls != 0 {
		t.Fatal("non-Windows provider should not discover during construction")
	}
	endpoints, err := provider.GetEndpoints(context.Background())
	if err != nil || calls != 1 || !hasFallbackAddress(endpoints, "192.0.2.53:53") {
		t.Fatalf("fallback result=%v err=%v calls=%d", endpoints, err, calls)
	}
}

func waitFallbackRefresh(t *testing.T, provider *fallbackDNSProvider) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		provider.mu.Lock()
		busy := provider.refreshing
		provider.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("fallback refresh did not finish")
}
