package runtime

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/nextdns/nextdns/resolver/query"
)

// A real UDP resolver that can become silent without closing its socket.
// Every socket binds an ephemeral loopback port; system DNS is untouched.
type recoveryUDPServer struct {
	ep       *endpoint.DNSEndpoint
	answers  atomic.Bool
	requests atomic.Int32
	received chan struct{}
}

func newRecoveryUDPServer(t *testing.T, answering bool, lastIPByte byte) *recoveryUDPServer {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &recoveryUDPServer{
		ep:       &endpoint.DNSEndpoint{Addr: conn.LocalAddr().String()},
		received: make(chan struct{}, 64),
	}
	s.answers.Store(answering)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			s.requests.Add(1)
			select {
			case s.received <- struct{}{}:
			default:
			}
			if !s.answers.Load() || n < 12 {
				continue
			}
			buf[2], buf[3] = 0x81, 0x80
			binary.BigEndian.PutUint16(buf[6:8], 1)
			answer := append(buf[:n:n], 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, lastIPByte)
			_, _ = conn.WriteTo(answer, addr)
		}
	}()
	t.Cleanup(func() { _ = conn.Close(); <-done })
	return s
}

func recoveryQuery() query.Query {
	return query.Query{
		ID: 0x1234, Class: query.ClassINET, Type: query.TypeA, Name: "example.org.",
		Payload: []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'o', 'r', 'g', 0, 0, 1, 0, 1},
	}
}

func TestRecoveryIntegrationConcurrentInitialization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Initialize HTTP transport without making a network request.
	for range 1000 {
		ep := &endpoint.DOHEndpoint{Hostname: "localhost", Path: "/dns-query", Bootstrap: []string{"127.0.0.1"}}
		m := newEndpointManager(ep, endpoint.StaticProvider([]endpoint.Endpoint{ep}))
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, _ = ep.Exchange(ctx, nil, make([]byte, 512))
		}()
		go func() {
			defer wg.Done()
			<-start
			_ = m.queries.Do(ctx, func(endpoint.Endpoint) error { return nil })
		}()
		close(start)
		wg.Wait()
	}
}

func TestRecoveryIntegrationSilentPrimaryThenReturnToPreferred(t *testing.T) {
	primary := newRecoveryUDPServer(t, false, 1)
	deadFallback := newRecoveryUDPServer(t, false, 2)
	liveFallback := newRecoveryUDPServer(t, true, 3)
	m := newEndpointManager(primary.ep,
		endpoint.StaticProvider([]endpoint.Endpoint{primary.ep}),
		endpoint.StaticProvider([]endpoint.Endpoint{deadFallback.ep, liveFallback.ep}),
	)
	r := retryResolver{inner: &resolver.DNS{Manager: m.queries}, recover: m.Test}
	buf := make([]byte, 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	start := time.Now()
	n, info, err := r.Resolve(ctx, recoveryQuery(), buf)
	if err != nil || n < 16 || buf[n-1] != 3 || info.Transport != "UDP" {
		t.Fatalf("fallback response: n=%d info=%+v err=%v", n, info, err)
	}
	if ctx.Err() != nil {
		t.Fatalf("response exceeded total deadline: %v", ctx.Err())
	}
	if primary.requests.Load() < 2 || liveFallback.requests.Load() < 2 {
		t.Fatalf("expected failed primary query/probe and successful fallback probe/query; primary=%d fallback=%d", primary.requests.Load(), liveFallback.requests.Load())
	}
	t.Logf("silent primary recovered through responsive second fallback in %v", time.Since(start))

	primary.answers.Store(true)
	if err := m.Test(ctx); err != nil {
		t.Fatalf("recovery probe: %v", err)
	}
	n, _, err = r.Resolve(ctx, recoveryQuery(), buf)
	if err != nil || n < 16 || buf[n-1] != 1 {
		t.Fatalf("preferred resolver did not resume answering: n=%d err=%v", n, err)
	}
}

func TestRecoveryIntegrationCacheAndQueryDeadlineDuringProbe(t *testing.T) {
	primary := newRecoveryUDPServer(t, true, 1)
	m := newEndpointManager(primary.ep, endpoint.StaticProvider([]endpoint.Endpoint{primary.ep}))
	cache, err := lru.NewARC(16)
	if err != nil {
		t.Fatal(err)
	}
	r := retryResolver{inner: &resolver.DNS{Manager: m.queries, DNS53: resolver.DNS53{Cache: cache}}, recover: m.Test}
	buf := make([]byte, 2048)
	if _, _, err := r.Resolve(context.Background(), recoveryQuery(), buf); err != nil {
		t.Fatal(err)
	}
	<-primary.received
	primary.answers.Store(false)
	probeDone := make(chan error, 1)
	probeCtx, cancelProbe := context.WithCancel(context.Background())
	defer cancelProbe()
	go func() { probeDone <- m.Test(probeCtx) }()
	select {
	case <-primary.received:
	case <-time.After(time.Second):
		t.Fatal("health probe did not reach upstream")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	n, info, err := r.Resolve(ctx, recoveryQuery(), buf)
	if err != nil || !info.FromCache || n < 16 || ctx.Err() != nil {
		t.Fatalf("probe blocked cached answer: n=%d info=%+v err=%v context=%v", n, info, err, ctx.Err())
	}
	cache.Purge()
	ctx, cancelQuery := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelQuery()
	start := time.Now()
	_, _, err = r.Resolve(ctx, recoveryQuery(), buf)
	if err == nil {
		t.Fatal("silent uncached upstream unexpectedly answered")
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("query exceeded its deadline behind ongoing probe: %v", elapsed)
	}
	select {
	case <-probeDone:
		t.Fatal("short query should return while health probe is still running")
	default:
	}
	cancelProbe()
	<-probeDone
}

func TestRecoveryIntegrationConcurrentRecoveryCoalescesAndWaitersCancel(t *testing.T) {
	live := newRecoveryUDPServer(t, true, 1)
	entered, release := make(chan struct{}), make(chan struct{})
	var providerCalls atomic.Int32
	provider := endpoint.ProviderFunc(func(ctx context.Context) ([]endpoint.Endpoint, error) {
		if providerCalls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
			return []endpoint.Endpoint{live.ep}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	m := newEndpointManager(live.ep, provider)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() { first <- m.Test(ctx) }()
	<-entered
	const waiters = 16
	results := make(chan error, waiters)
	for range waiters {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			results <- m.Test(ctx)
		}()
	}
	for range waiters {
		if err := <-results; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiting recovery ignored cancellation: %v", err)
		}
	}
	if got := providerCalls.Load(); got != 1 {
		t.Fatalf("concurrent recovery duplicated provider work: calls=%d", got)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("cancelled waiters cancelled shared recovery: %v", err)
	}
	if got := live.requests.Load(); got != 1 {
		t.Fatalf("expected one shared network probe, got %d", got)
	}
}
