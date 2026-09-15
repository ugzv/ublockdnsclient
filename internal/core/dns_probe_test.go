package core

import (
	"encoding/binary"
	"net"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLocalDNSAddresses(t *testing.T) {
	dns := []string{"8.8.8.8", "127.0.0.53", "invalid", "127.0.0.1", "::1", "0:0:0:0:0:0:0:1", "127.0.0.1"}
	if got, want := LocalDNSAddresses(dns), []string{LocalDNSAddr, LocalDNSAddrV6}; !slices.Equal(got, want) {
		t.Fatalf("addresses = %v, want %v", got, want)
	}
}

func TestCheckLocalDNSProxyRequiresEveryAddress(t *testing.T) {
	var servers []string
	for _, addr := range []string{"127.0.0.1:0", "[::1]:0"} {
		conn, err := net.ListenPacket("udp", addr)
		if err != nil {
			if strings.HasPrefix(addr, "[") {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		servers = append(servers, conn.LocalAddr().String())
		// IPv4 answers normally; IPv6 returns SERVFAIL.
		failed := strings.HasPrefix(addr, "[")
		go func() {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			buf := make([]byte, 512)
			n, peer, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			flags := uint16(0x8180)
			if failed {
				flags |= 2
			}
			binary.BigEndian.PutUint16(buf[2:4], flags)
			_, _ = conn.WriteTo(buf[:n], peer)
		}()
	}
	if err := CheckLocalDNSProxy("example.com", servers...); err == nil || !strings.Contains(err.Error(), servers[1]) {
		t.Fatalf("probe error = %v, want failing IPv6 address", err)
	}
}
