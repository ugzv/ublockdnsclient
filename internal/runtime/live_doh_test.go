package runtime

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nextdns/nextdns/resolver/endpoint"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

// Live network test of the full DoH chain through the dynamic provider and
// manager wiring. Run manually: UBLOCKDNS_LIVE_PROFILE=<id> go test -run Live.
func TestLiveDoHChain(t *testing.T) {
	profile := os.Getenv("UBLOCKDNS_LIVE_PROFILE")
	if profile == "" {
		t.Skip("set UBLOCKDNS_LIVE_PROFILE to run")
	}

	_, hostname, path, err := BuildDoHTarget(core.DefaultDoHServer, profile)
	if err != nil {
		t.Fatal(err)
	}
	ips, err := resolveBootstrapIPs(hostname)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("bootstrap IPs for %s: %v", hostname, ips)

	ep := &endpoint.DOHEndpoint{Hostname: hostname, Path: path, Bootstrap: ips}
	mgr := newEndpointManager(endpoint.StaticProvider([]endpoint.Endpoint{ep}), ep)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := testEndpointDomain(ctx, ep, dohProbeDomain); err != nil {
		t.Fatalf("DoH probe failed: %v", err)
	}
	if err := mgr.Test(ctx); err != nil {
		t.Fatalf("manager test failed: %v", err)
	}
}
