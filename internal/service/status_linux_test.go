//go:build linux

package service

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveSystemDNSPrefersAuthoritativeLocalDNS(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		hostDNS: func() []string { return []string{"8.8.8.8", "8.8.4.4"} },
		commandOutput: func(name string, args ...string) ([]byte, error) {
			if name == "resolvectl" && len(args) == 1 && args[0] == "dns" {
				return []byte("Global: 127.0.0.1\nLink 2 (wlan0): 192.168.2.1\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
		readFile: func(name string) ([]byte, error) {
			if name == "/etc/resolv.conf" {
				return []byte("nameserver 127.0.0.1\n"), nil
			}
			return nil, errors.New("unexpected file")
		},
	})

	assessment := resolveSystemDNS()
	if !assessment.LocalDNS {
		t.Fatalf("expected local DNS assessment, got %+v", assessment)
	}
	if !sameDNSSet(assessment.DNS, []string{"127.0.0.1", "192.168.2.1"}) {
		t.Fatalf("expected resolvectl DNS to win, got %+v", assessment)
	}
	if len(assessment.Warnings) == 0 {
		t.Fatalf("expected disagreement warning, got %+v", assessment)
	}
	if !strings.Contains(assessment.Warnings[0], "upstream DNS") {
		t.Fatalf("expected upstream metadata warning, got %+v", assessment)
	}
}

func TestResolveSystemDNSFallsBackToResolvConf(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		hostDNS: func() []string { return []string{"8.8.8.8"} },
		commandOutput: func(name string, args ...string) ([]byte, error) {
			return nil, errors.New("resolvectl unavailable")
		},
		readFile: func(name string) ([]byte, error) {
			return []byte("# managed\nnameserver 127.0.0.1\n"), nil
		},
	})

	assessment := resolveSystemDNS()
	if !assessment.LocalDNS {
		t.Fatalf("expected resolv.conf local DNS assessment, got %+v", assessment)
	}
	if !sameDNSSet(assessment.DNS, []string{"127.0.0.1"}) {
		t.Fatalf("expected resolv.conf DNS, got %+v", assessment)
	}
	if len(assessment.Warnings) == 0 {
		t.Fatalf("expected resolvectl fallback warning, got %+v", assessment)
	}
	if !strings.Contains(assessment.Warnings[0], "resolvectl unavailable") {
		t.Fatalf("expected resolvectl warning, got %+v", assessment.Warnings)
	}
}

func TestCurrentStatusUsesLinuxResolverAssessment(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		serviceState:     func() (string, error) { return "running", nil },
		localDNSProbe:    func() error { return nil },
		loadInstallState: missingInstallState,
		hostDNS:          func() []string { return []string{"8.8.8.8", "8.8.4.4"} },
		commandOutput: func(name string, args ...string) ([]byte, error) {
			if name == "resolvectl" {
				return []byte("Global: 127.0.0.1\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
		readFile: func(name string) ([]byte, error) {
			return []byte("nameserver 127.0.0.1\n"), nil
		},
	})

	info := CurrentStatus()
	if !info.Ready || info.Status != "active" {
		t.Fatalf("expected linux assessment to mark active, got %+v", info)
	}
	if !info.LocalDNS {
		t.Fatalf("expected local DNS true, got %+v", info)
	}
	if !sameDNSSet(info.SystemDNS, []string{"127.0.0.1"}) {
		t.Fatalf("expected authoritative local DNS, got %+v", info)
	}
}
