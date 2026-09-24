package core

import (
	"net"
	"slices"
	"testing"
)

func TestLocalListenAddrsIncludesAvailableLoopbacks(t *testing.T) {
	t.Parallel()
	want := []string{LocalDNSAddr}
	if listener, err := net.Listen("tcp", "[::1]:0"); err == nil {
		_ = listener.Close()
		want = append(want, LocalDNSAddrV6)
	}
	got := LocalListenAddrs()
	if !slices.Equal(got, want) {
		t.Fatalf("LocalListenAddrs() = %v, want %v", got, want)
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

func TestCheckListenAddrsFreeRejectsOccupiedPort(t *testing.T) {
	t.Parallel()
	// Stands in for dnsmasq holding 127.0.0.1:53, without needing port 53.
	busy, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()

	if err := CheckListenAddrsFree(busy.LocalAddr().String()); err == nil {
		t.Fatal("CheckListenAddrsFree() = nil, want error for an occupied address")
	}
}
