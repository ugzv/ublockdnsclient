package main

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/host/service"
)

// install sets up ublockdns as a system service.
func install(profileID, dohServer, apiServer, accountToken string) error {
	if !hasInstallPrivileges() {
		return fmt.Errorf("install requires elevated privileges - %s", installPrivilegeHint())
	}

	svc, err := newService(profileID, dohServer, apiServer)
	if err != nil {
		return err
	}

	// Uninstall any previous version first.
	_ = svc.Stop()
	_ = svc.Uninstall()

	log.Println("Installing service...")
	if err := svc.Install(); err != nil {
		return fmt.Errorf("install service: %w", err)
	}

	log.Println("Starting service...")
	if err := svc.Start(); err != nil {
		// Rollback: remove service if it can't start.
		_ = svc.Uninstall()
		return fmt.Errorf("start service: %w", err)
	}

	if strings.TrimSpace(accountToken) != "" {
		if err := persistToken(profileID, accountToken); err != nil {
			log.Printf("Warning: failed to persist account token for rules stream: %v", err)
		}
	}

	// Best-effort readiness probe only. Do not fail install if upstream DNS is
	// temporarily unavailable (matches NextDNS install behavior).
	if err := checkLocalDNSProxy("example.com"); err != nil {
		log.Printf("Warning: local DNS preflight failed (continuing): %v", err)
	}

	log.Println("Setting system DNS to 127.0.0.1...")
	if err := host.SetDNS("127.0.0.1"); err != nil {
		return fmt.Errorf("set system DNS: %w", err)
	}

	return nil
}

// uninstall removes the system service and restores DNS.
func uninstall() error {
	if err := host.ResetDNS(); err != nil {
		log.Printf("Warning: could not reset DNS: %v", err)
	}

	svc, err := newService("", "", "")
	if err != nil {
		return err
	}

	_ = svc.Stop()

	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}
	_ = clearPersistedTokens()

	return nil
}

func serviceStart() error {
	svc, err := newService("", "", "")
	if err != nil {
		return err
	}
	return svc.Start()
}

func serviceStop() error {
	svc, err := newService("", "", "")
	if err != nil {
		return err
	}
	return svc.Stop()
}

func showStatus() {
	dns := currentSystemDNS()
	localDNS := hasDNS127001(dns)

	svcState := "unknown"
	if svc, err := newService("", "", ""); err == nil {
		if st, err := svc.Status(); err == nil {
			switch st {
			case service.StatusRunning:
				svcState = "running"
			case service.StatusStopped:
				svcState = "stopped"
			case service.StatusNotInstalled:
				svcState = "not-installed"
			default:
				svcState = "unknown"
			}
		}
	}

	active := localDNS
	if svcState == "running" {
		active = localDNS
	}
	if svcState == "stopped" || svcState == "not-installed" {
		active = false
	}

	if active {
		fmt.Println("Status: active")
	} else {
		fmt.Println("Status: inactive")
	}

	if len(dns) == 0 {
		fmt.Println("System DNS: (none)")
	} else if localDNS {
		fmt.Printf("System DNS: %s (includes 127.0.0.1 via uBlock DNS)\n", strings.Join(dns, ", "))
	} else {
		fmt.Printf("System DNS: %s\n", strings.Join(dns, ", "))
	}

	fmt.Printf("Service: %s\n", svcState)

	if svcState == "running" && !localDNS {
		fmt.Println("Warning: service is running but system DNS is not pointing to 127.0.0.1")
	}
	if localDNS && svcState != "running" && svcState != "unknown" {
		fmt.Println("Warning: system DNS includes 127.0.0.1 but service is not running")
	}
}

func hasDNS127001(dns []string) bool {
	for _, d := range dns {
		if d == "127.0.0.1" {
			return true
		}
	}
	return false
}

func currentSystemDNS() []string {
	// On macOS, host.DNS() can report stale/non-primary resolver values.
	// Prefer scutil output when available.
	if runtime.GOOS == "darwin" {
		if dns, err := dnsFromScutil(); err == nil && len(dns) > 0 {
			return dns
		}
	}
	return host.DNS()
}

func dnsFromScutil() ([]string, error) {
	out, err := exec.Command("scutil", "--dns").Output()
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`nameserver\[[0-9]+\]\s*:\s*([^\s]+)`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	var dns []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		ip := strings.TrimSpace(m[1])
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		dns = append(dns, ip)
	}
	return dns, nil
}

func newService(profileID, dohServer, apiServer string) (service.Service, error) {
	args := []string{"run"}
	if profileID != "" {
		args = append(args, "-profile", profileID)
	}
	if dohServer != "" {
		args = append(args, "-server", dohServer)
	}
	if apiServer != "" {
		args = append(args, "-api-server", apiServer)
	}

	return host.NewService(service.Config{
		Name:        serviceName,
		DisplayName: "uBlock DNS",
		Description: "DNS-level ad blocker - routes DNS through ublockdns.com",
		Arguments:   args,
	})
}
