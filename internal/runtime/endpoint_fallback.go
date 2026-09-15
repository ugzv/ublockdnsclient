package runtime

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/ugzv/ublockdnsclient/internal/core"
)

const dohProbeDomain = core.ProbeDomain
const endpointProbeTimeout = time.Second

var fallbackDNSServers = []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}

type endpointSelection struct{ endpoint.Endpoint }
type endpointTest struct {
	done chan struct{}
	err  error
}

// The library manager holds its query lock while testing endpoints. Keep its
// tests entirely local; network probes publish a selection only after finishing.
type endpointManager struct {
	queries   *endpoint.Manager
	providers []endpoint.Provider
	selected  atomic.Pointer[endpointSelection]
	mu        sync.Mutex
	testing   *endpointTest
}

func newEndpointManager(initial endpoint.Endpoint, providers ...endpoint.Provider) *endpointManager {
	m := &endpointManager{providers: providers}
	m.selected.Store(&endpointSelection{initial})
	m.queries = &endpoint.Manager{
		InitEndpoint: initial,
		Providers: []endpoint.Provider{endpoint.ProviderFunc(func(context.Context) ([]endpoint.Endpoint, error) {
			return []endpoint.Endpoint{m.selected.Load().Endpoint}, nil
		})},
		EndpointTester: func(endpoint.Endpoint) endpoint.Tester {
			return func(context.Context, string) error { return nil }
		},
	}
	// Initialize the dependency's endpoint metadata before the first external
	// probe can initialize its transport concurrently with a query.
	_ = m.queries.Test(context.Background())
	return m
}

// Test coalesces concurrent recovery requests and never holds the query lock
// during I/O. Unavailable candidates leave the last selected endpoint intact.
func (m *endpointManager) Test(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	if run := m.testing; run != nil {
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-run.done:
			return run.err
		}
	}
	run := &endpointTest{done: make(chan struct{})}
	m.testing = run
	m.mu.Unlock()
	defer func() { m.mu.Lock(); close(run.done); m.testing = nil; m.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(ctx, 2*endpointProbeTimeout)
	defer cancel()
	for _, provider := range m.providers {
		candidates, err := provider.GetEndpoints(ctx)
		if err != nil {
			run.err = err
			continue
		}
		chosen, err := m.probe(ctx, candidates)
		if err != nil {
			run.err = err
			continue
		}
		if err := ctx.Err(); err != nil {
			run.err = err
			return err
		}
		previous := m.selected.Swap(&endpointSelection{chosen})
		if previous.Protocol() != chosen.Protocol() {
			if chosen.Protocol() == endpoint.ProtocolDNS {
				log.Printf("DNS recovery: using plaintext fallback; profile filtering is bypassed")
			} else {
				log.Printf("DNS recovery: restored encrypted profile resolver")
			}
		}
		run.err = m.queries.Test(ctx) // No network operations in this manager.
		return run.err
	}
	if run.err == nil {
		run.err = errors.New("no reachable DNS endpoints")
	}
	return run.err
}

func (m *endpointManager) probe(ctx context.Context, candidates []endpoint.Endpoint) (endpoint.Endpoint, error) {
	ctx, cancel := context.WithTimeout(ctx, endpointProbeTimeout)
	defer cancel()
	type result struct {
		ep  endpoint.Endpoint
		err error
	}
	results := make(chan result, len(candidates))
	for _, ep := range candidates {
		go func() {
			results <- result{ep, testEndpointDomain(ctx, ep, dohProbeDomain)}
		}()
	}
	var lastErr error
	for range candidates {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-results:
			if r.err == nil {
				return r.ep, nil
			}
			lastErr = r.err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no DNS endpoints")
	}
	return nil, lastErr
}

// While using a fallback, periodically try the preferred encrypted endpoint.
func (m *endpointManager) watch(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.selected.Load().Protocol() == endpoint.ProtocolDNS {
				_ = m.Test(ctx)
			}
		}
	}
}

type fallbackDNSProvider struct {
	windows    bool
	discover   func(context.Context) []string
	mu         sync.Mutex
	servers    []string
	refreshing bool
}

func newFallbackDNSProvider(platform string, discover func(context.Context) []string) *fallbackDNSProvider {
	p := &fallbackDNSProvider{windows: platform == "windows", discover: discover}
	if p.windows {
		// PowerShell startup can exceed the recovery budget. Prime its cache
		// before serving DNS, then keep discovery off the recovery path.
		p.refresh()
	}
	return p
}

func (p *fallbackDNSProvider) String() string { return "fallback DNS" }

func (p *fallbackDNSProvider) GetEndpoints(ctx context.Context) ([]endpoint.Endpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !p.windows {
		discoveryCtx, cancel := context.WithTimeout(ctx, endpointProbeTimeout/4)
		defer cancel()
		return fallbackEndpointsFor(p.discover(discoveryCtx)), nil
	}
	p.mu.Lock()
	servers := p.servers
	if !p.refreshing {
		p.refreshing = true
		go p.refresh()
	}
	p.mu.Unlock()
	return fallbackEndpointsFor(servers), nil
}

func (p *fallbackDNSProvider) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	servers := p.discover(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(servers) > 0 {
		p.servers = servers
	}
	p.refreshing = false
}

func fallbackEndpointsFor(servers []string) []endpoint.Endpoint {
	seen := map[string]struct{}{}
	out := make([]endpoint.Endpoint, 0, len(servers)+len(fallbackDNSServers))

	// Include the network's own resolvers as well as the public fallbacks.
	for _, ip := range servers {
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

func testEndpointDomain(ctx context.Context, e endpoint.Endpoint, hostname string) error {
	const queryID = uint16(0x4D21)
	response, err := core.ExchangeDNSQuery(queryID, hostname, func(payload, buf []byte) (int, error) {
		if dns, ok := e.(*endpoint.DNSEndpoint); ok {
			// DNSEndpoint.Exchange in the pinned dependency mutates the transaction ID
			// and compares its low byte against the response buffer before reading it.
			// Use a connected UDP socket so the probe actually verifies this resolver.
			conn, err := (&net.Dialer{}).DialContext(ctx, "udp", dns.Addr)
			if err != nil {
				return 0, err
			}
			defer conn.Close()
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			defer stop()
			if deadline, ok := ctx.Deadline(); ok {
				_ = conn.SetDeadline(deadline)
			}
			if _, err = conn.Write(payload); err != nil {
				return 0, err
			}
			return conn.Read(buf)
		}
		return e.Exchange(ctx, payload, buf)
	})
	if err != nil {
		return fmt.Errorf("endpoint probe failed: %w", err)
	}
	if len(response) < 12 || response[2]&0x80 == 0 {
		return errors.New("endpoint probe returned an invalid DNS response")
	}
	rcode := binary.BigEndian.Uint16(response[2:4]) & 15
	if rcode != 0 && rcode != 3 {
		return fmt.Errorf("endpoint probe returned DNS rcode=%d", rcode)
	}
	return nil
}
