package runtime

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nextdns/nextdns/proxy"
	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/query"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

// flakyUpstream fails the next n resolves, then answers NOERROR.
type flakyUpstream struct {
	failures atomic.Int32
}

func (f *flakyUpstream) Resolve(_ context.Context, q query.Query, buf []byte) (int, resolver.ResolveInfo, error) {
	if f.failures.Add(-1) >= 0 {
		return 0, resolver.ResolveInfo{}, errors.New("transport broken")
	}
	n := copy(buf, q.Payload)
	buf[2] |= 0x80 // QR: response
	return n, resolver.ResolveInfo{Transport: "test"}, nil
}

func queryRcode(t *testing.T, addr string) uint16 {
	t.Helper()
	conn, err := net.DialTimeout("udp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))

	resp, err := core.ExchangeDNSQuery(0x1234, "example.com", func(payload, buf []byte) (int, error) {
		if _, err := conn.Write(payload); err != nil {
			return 0, err
		}
		return conn.Read(buf)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) < 4 {
		t.Fatalf("short response: %d bytes", len(resp))
	}
	return binary.BigEndian.Uint16(resp[2:4]) & 0x000F
}

// TestProxyEndToEndRetryMasksTransientFailure exercises the real proxy stack
// over UDP: a persistent upstream failure still surfaces as SERVFAIL, while a
// single transient failure (dead connection after wake) is masked by the
// retry wrapper and resolves NOERROR.
func TestProxyEndToEndRetryMasksTransientFailure(t *testing.T) {
	upstream := &flakyUpstream{}
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.LocalAddr().String()
	_ = l.Close()

	p := proxy.Proxy{
		Addrs:               []string{addr},
		Upstream:            retryResolver{inner: upstream},
		Timeout:             2 * time.Second,
		MaxInflightRequests: 16,
	}
	// No shutdown: cancelling ListenAndServe races inside the nextdns proxy
	// library; the goroutine exits with the test process.
	go func() { _ = p.ListenAndServe(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	upstream.failures.Store(2) // first attempt + retry fail, no stale cache
	if rcode := queryRcode(t, addr); rcode != 2 {
		t.Fatalf("persistent failure: rcode = %d, want 2 (SERVFAIL)", rcode)
	}

	upstream.failures.Store(1) // transient blip: retry succeeds
	if rcode := queryRcode(t, addr); rcode != 0 {
		t.Fatalf("transient failure: rcode = %d, want 0 (NOERROR via retry)", rcode)
	}
}
