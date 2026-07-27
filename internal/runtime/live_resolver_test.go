package runtime

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/nextdns/nextdns/resolver/query"
)

// Live network test of the full upstream chain a running daemon uses:
// bootstrap resolution, endpoint manager, DoH transport, the response cache,
// and the purge a rules update triggers. Unit tests cannot cover this -- they
// stub the resolver, so a broken cache or purge still looks like success.
//
// Run manually against any DoH endpoint, e.g.:
//
//	UBLOCKDNS_LIVE_DOH=https://cloudflare-dns.com/dns-query go test -run LiveResolver ./internal/runtime/
func TestLiveResolverServesAndPurges(t *testing.T) {
	base := os.Getenv("UBLOCKDNS_LIVE_DOH")
	if base == "" {
		t.Skip("set UBLOCKDNS_LIVE_DOH to a DoH endpoint URL to run")
	}

	hostname, path := splitDoHBase(base)
	ips, err := resolveBootstrapIPs(hostname)
	if err != nil {
		t.Fatalf("bootstrap resolution failed: %v", err)
	}
	t.Logf("bootstrap IPs for %s: %v", hostname, ips)

	ep := &endpoint.DOHEndpoint{Hostname: hostname, Path: path, Bootstrap: ips}
	mgr := newEndpointManager(endpoint.StaticProvider([]endpoint.Endpoint{ep}), ep)

	cache, err := lru.NewARC(dnsCacheEntries)
	if err != nil {
		t.Fatal(err)
	}

	// Wired exactly as the proxy wires it.
	res := retryResolver{inner: &resolver.DNS{
		DOH:     resolver.DOH{URL: base, Cache: cache},
		Manager: mgr,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if first := mustResolve(ctx, t, res, "example.com"); first.fromCache {
		t.Fatal("first query was served from cache; the cache should have been empty")
	}
	t.Log("first query went upstream, as expected")

	if second := mustResolve(ctx, t, res, "example.com"); !second.fromCache {
		t.Fatal("repeat query was not served from cache; caching is not working")
	}

	// A rules update purges: the next identical query must go upstream again.
	cache.Purge()

	if third := mustResolve(ctx, t, res, "example.com"); third.fromCache {
		t.Fatal("query after purge was still served from cache; rule updates would not take effect")
	}
	t.Log("purge cleared the cache; the next query went upstream")
}

type resolveResult struct {
	n         int
	fromCache bool
}

func mustResolve(ctx context.Context, t *testing.T, r resolver.Resolver, name string) resolveResult {
	t.Helper()

	q, err := query.New(dnsQueryPayload(0x1234, name), net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 1))
	if err != nil {
		t.Fatalf("query.New(%q) error = %v", name, err)
	}

	buf := make([]byte, 4096)
	n, info, err := r.Resolve(ctx, q, buf)
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", name, err)
	}
	if n <= 0 {
		t.Fatalf("Resolve(%q) returned %d bytes", name, n)
	}
	if rcode := binary.BigEndian.Uint16(buf[2:4]) & 0x000F; rcode != 0 {
		t.Fatalf("Resolve(%q) rcode = %d, want 0", name, rcode)
	}
	t.Logf("resolved %s: %d bytes, fromCache=%v, transport=%s", name, n, info.FromCache, info.Transport)
	return resolveResult{n: n, fromCache: info.FromCache}
}

func dnsQueryPayload(id uint16, hostname string) []byte {
	q := make([]byte, 12)
	binary.BigEndian.PutUint16(q[0:2], id)
	binary.BigEndian.PutUint16(q[2:4], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(q[4:6], 1)      // QDCOUNT
	for _, label := range strings.Split(hostname, ".") {
		if label == "" {
			continue
		}
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	q = append(q, 0x00)
	q = append(q, 0x00, 0x01, 0x00, 0x01) // QTYPE=A, QCLASS=IN
	return q
}

func splitDoHBase(base string) (hostname, path string) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
	host, rest, found := strings.Cut(trimmed, "/")
	if !found {
		return host, "/"
	}
	return host, "/" + rest
}
