package runtime

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/nextdns/nextdns/resolver/endpoint"
)

type probeEndpoint struct {
	probe func(context.Context, []byte, []byte) (int, error)
}

func (e *probeEndpoint) String() string                     { return "test-doh" }
func (e *probeEndpoint) Protocol() endpoint.Protocol        { return endpoint.ProtocolDOH }
func (e *probeEndpoint) Equal(other endpoint.Endpoint) bool { return e == other }
func (e *probeEndpoint) Exchange(ctx context.Context, q, buf []byte) (int, error) {
	return e.probe(ctx, q, buf)
}

func TestEndpointProbeDoesNotBlockQueries(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	ep := &probeEndpoint{probe: func(ctx context.Context, q, buf []byte) (int, error) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		n := copy(buf, q)
		buf[2] |= 0x80
		return n, nil
	}}
	m := newEndpointManager(ep, endpoint.StaticProvider([]endpoint.Endpoint{ep}))
	probeDone := make(chan error, 1)
	go func() { probeDone <- m.Test(context.Background()) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.queries.Do(ctx, func(endpoint.Endpoint) error { return ctx.Err() }) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("query blocked behind probe: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("query blocked behind unrelated endpoint probe")
	}
	close(release)
	<-probeDone
}

func TestEndpointManagerSkipsDeadFallback(t *testing.T) {
	dead, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dead.Close() // Blackhole: deliberately never answer.
	live, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := live.ReadFrom(buf)
			if err != nil {
				return
			}
			buf[2] |= 0x80
			_, _ = live.WriteTo(buf[:n], addr)
		}
	}()
	bad := &probeEndpoint{probe: func(context.Context, []byte, []byte) (int, error) { return 0, errors.New("DoH unreachable") }}
	m := newEndpointManager(bad,
		endpoint.StaticProvider([]endpoint.Endpoint{bad}),
		endpoint.StaticProvider([]endpoint.Endpoint{&endpoint.DNSEndpoint{Addr: dead.LocalAddr().String()}, &endpoint.DNSEndpoint{Addr: live.LocalAddr().String()}}),
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Test(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.queries.Do(ctx, func(ep endpoint.Endpoint) error {
		if ep.String() != live.LocalAddr().String() {
			return errors.New("selected unreachable first fallback instead of responding server")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointProbeRejectsInvalidAndFailedResponses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags byte
		valid bool
	}{
		{"answer", 0x80, true}, {"nxdomain", 0x83, true}, {"servfail", 0x82, false}, {"refused", 0x85, false}, {"query echo", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ep := &probeEndpoint{probe: func(_ context.Context, q, buf []byte) (int, error) {
				n := copy(buf, q)
				buf[2] = tc.flags & 0x80
				buf[3] = tc.flags & 15
				return n, nil
			}}
			err := testEndpointDomain(context.Background(), ep, dohProbeDomain)
			if (err == nil) != tc.valid {
				t.Fatalf("probe err=%v valid=%v", err, tc.valid)
			}
		})
	}
}
