//go:build windows

package service

import (
	"slices"
	"strings"
	"testing"
)

func TestWindowsDNSDiscoveryIncludesIPv6(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		commandOutput: func(name string, args ...string) ([]byte, error) {
			if strings.Contains(strings.Join(args, " "), "-AddressFamily IPv4") {
				t.Fatal("DNS discovery must include IPv6 adapters")
			}
			return []byte("127.0.0.1\r\n::1\r\n127.0.0.1\r\n"), nil
		},
	})
	got, err := dnsFromWindowsPowerShell()
	if err != nil || !slices.Equal(got, []string{"127.0.0.1", "::1"}) {
		t.Fatalf("DNS = %v, error = %v", got, err)
	}
}
