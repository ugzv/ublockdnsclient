package runtime

import (
	"context"
	"net"
	"time"

	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/resolver/endpoint"
)

const dohProbeDomain = "example.com"

var fallbackDNSServers = []string{
	"1.1.1.1:53",
	"8.8.8.8:53",
	"9.9.9.9:53",
}

func newEndpointManager(dohEndpoint endpoint.Endpoint) *endpoint.Manager {
	m := &endpoint.Manager{
		Providers: []endpoint.Provider{
			endpoint.StaticProvider([]endpoint.Endpoint{dohEndpoint}),
			endpoint.ProviderFunc(func(ctx context.Context) ([]endpoint.Endpoint, error) {
				eps := fallbackDNSEndpoints()
				return eps, nil
			}),
		},
		InitEndpoint: dohEndpoint,
	}

	// Override the upstream library probe domain so startup checks do not emit
	// provider-branded verification lookups on first use.
	m.EndpointTester = func(e endpoint.Endpoint) endpoint.Tester {
		if e.Protocol() == endpoint.ProtocolDOH {
			return func(ctx context.Context, _ string) error {
				return testEndpointDomain(ctx, e, dohProbeDomain)
			}
		}
		if e.Protocol() == endpoint.ProtocolDNS {
			return func(ctx context.Context, testDomain string) error { return nil }
		}
		return nil
	}
	m.GetMinTestInterval = func(e endpoint.Endpoint) time.Duration {
		if e.Protocol() == endpoint.ProtocolDNS {
			return 5 * time.Second
		}
		return 0
	}
	return m
}

func fallbackDNSEndpoints() []endpoint.Endpoint {
	seen := map[string]struct{}{}
	out := make([]endpoint.Endpoint, 0, len(host.DNS())+len(fallbackDNSServers))

	// Prefer currently configured non-loopback system resolvers.
	for _, ip := range host.DNS() {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.IsLoopback() {
			continue
		}
		addr := net.JoinHostPort(ip, "53")
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, &endpoint.DNSEndpoint{Addr: addr})
	}

	// Add known public fallbacks to avoid full outage when local resolver is unreachable.
	for _, addr := range fallbackDNSServers {
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, &endpoint.DNSEndpoint{Addr: addr})
	}

	return out
}
