package core

import (
	"fmt"
	"net"
)

// LocalListenAddrs is where the proxy binds: IPv4 loopback and, when the stack
// has it, IPv6 loopback too, since Windows points resolvers at ::1 alongside
// 127.0.0.1.
func LocalListenAddrs() []string {
	addrs := []string{LocalDNSAddr}
	if l, err := net.Listen("tcp", "[::1]:0"); err == nil {
		_ = l.Close()
		addrs = append(addrs, LocalDNSAddrV6)
	}
	return addrs
}

// CheckListenAddrsFree fails when another program already holds one of addrs,
// e.g. dnsmasq, Pi-hole or AdGuard Home on port 53. Pointing system DNS at
// loopback would then hand every query to that program instead of the proxy.
func CheckListenAddrsFree(addrs ...string) error {
	for _, addr := range addrs {
		udp, err := net.ListenPacket("udp", addr)
		if err == nil {
			_ = udp.Close()
			var tcp net.Listener
			if tcp, err = net.Listen("tcp", addr); err == nil {
				_ = tcp.Close()
			}
		}
		if err != nil {
			return fmt.Errorf("%s is already in use by another program (a local DNS server such as dnsmasq?); stop it and install again: %w", addr, err)
		}
	}
	return nil
}
