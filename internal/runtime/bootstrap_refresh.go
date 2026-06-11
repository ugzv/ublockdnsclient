package runtime

import (
	"context"
	"log"
	"slices"
	"sync/atomic"

	"github.com/nextdns/nextdns/resolver/endpoint"
)

var resolveBootstrapIPsFunc = resolveBootstrapIPs

// bootstrapRefresher re-resolves the DoH hostname in the background and swaps
// in a new endpoint when its IPs change, so a server IP migration does not
// require a daemon restart. At most one refresh runs at a time.
type bootstrapRefresher struct {
	hostname, path string
	ep             *atomic.Pointer[endpoint.DOHEndpoint]
	mgr            *endpoint.Manager
	busy           atomic.Bool
}

func (b *bootstrapRefresher) refresh(ctx context.Context) {
	if !b.busy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer b.busy.Store(false)
		ips, err := resolveBootstrapIPsFunc(b.hostname)
		if err != nil || slices.Equal(ips, b.ep.Load().Bootstrap) {
			return
		}
		log.Printf("Bootstrap IPs changed for %s: %v", b.hostname, ips)
		b.ep.Store(&endpoint.DOHEndpoint{
			Hostname:  b.hostname,
			Path:      b.path,
			Bootstrap: ips,
		})
		testEndpoints(ctx, b.mgr, "bootstrap refresh")
	}()
}

func testEndpoints(ctx context.Context, mgr *endpoint.Manager, reason string) {
	if err := mgr.Test(ctx); err != nil {
		log.Printf("Endpoint test after %s failed: %v", reason, err)
	}
}
