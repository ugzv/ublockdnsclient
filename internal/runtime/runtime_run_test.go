package runtime

import (
	"net"
	"slices"
	"testing"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

func TestLocalListenAddrsIncludesAvailableLoopbacks(t *testing.T) {
	t.Parallel()
	want := []string{core.LocalDNSAddr}
	if listener, err := net.Listen("tcp", "[::1]:0"); err == nil {
		_ = listener.Close()
		want = append(want, core.LocalDNSAddrV6)
	}
	got := localListenAddrs()
	if !slices.Equal(got, want) {
		t.Fatalf("localListenAddrs() = %v, want %v", got, want)
	}
	for _, addr := range got {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatal(err)
		}
		// Verify both DNS transports can bind on every advertised address without
		// using port 53 or changing the host's resolver configuration.
		addr = net.JoinHostPort(host, "0")
		tcp, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		_ = tcp.Close()
		udp, err := net.ListenPacket("udp", addr)
		if err != nil {
			t.Fatal(err)
		}
		_ = udp.Close()
	}
}
