package app

import (
	"fmt"
	"log"
	"regexp"
	"runtime"
	"strings"

	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/host/service"
)

type InstallOutcome string

const (
	InstallOutcomeFresh    InstallOutcome = "fresh"
	InstallOutcomeUpdated  InstallOutcome = "updated"
	InstallOutcomeSwitched InstallOutcome = "switched"
)

// install sets up ublockdns as a system service.
func Install(profileID, dohServer, apiServer, accountToken string) error {
	_, err := InstallDetailed(profileID, dohServer, apiServer, accountToken)
	return err
}

// InstallDetailed installs (or reinstalls) the service and returns a UX-friendly outcome.
func InstallDetailed(profileID, dohServer, apiServer, accountToken string) (InstallOutcome, error) {
	if !hasInstallPrivileges() {
		return InstallOutcomeFresh, fmt.Errorf("install requires elevated privileges - %s", installPrivilegeHint())
	}

	prevDNSLocal := hasDNS127001(currentSystemDNS())
	prevInstalled := serviceCurrentlyInstalled()
	prevState, prevStateErr := loadInstallState()
	hasPrevState := prevStateErr == nil && strings.TrimSpace(prevState.ProfileID) != ""

	outcome := InstallOutcomeFresh
	if prevInstalled {
		outcome = InstallOutcomeUpdated
		if hasPrevState && prevState.ProfileID != profileID {
			outcome = InstallOutcomeSwitched
		}
	}

	svc, err := newService(profileID, dohServer, apiServer)
	if err != nil {
		return outcome, err
	}

	// Uninstall any previous version first.
	_ = svc.Stop()
	_ = svc.Uninstall()

	log.Println("Installing service...")
	if err := svc.Install(); err != nil {
		rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState, prevState)
		return outcome, fmt.Errorf("install service: %w", err)
	}

	log.Println("Starting service...")
	if err := svc.Start(); err != nil {
		// Rollback: remove service if it can't start.
		_ = svc.Uninstall()
		rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState, prevState)
		return outcome, fmt.Errorf("start service: %w", err)
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
		rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState, prevState)
		return outcome, fmt.Errorf("set system DNS: %w", err)
	}

	if err := persistInstallState(profileID, dohServer, apiServer); err != nil {
		log.Printf("Warning: failed to persist install state: %v", err)
	}

	return outcome, nil
}

// uninstall removes the system service and restores DNS.
func Uninstall() error {
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
	_ = clearInstallState()

	return nil
}

func serviceCurrentlyInstalled() bool {
	if runtime.GOOS == "windows" {
		if st, ok := windowsServiceState(); ok {
			return st != "not-installed"
		}
		return false
	}
	svc, err := newService("", "", "")
	if err != nil {
		return false
	}
	st, err := svc.Status()
	if err != nil {
		return false
	}
	return st != service.StatusNotInstalled
}

func rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState bool, prevState installState) {
	log.Printf("Install failed, attempting rollback...")

	if svc, err := newService("", "", ""); err == nil {
		_ = svc.Stop()
		_ = svc.Uninstall()
	}

	if prevInstalled && hasPrevState {
		if oldSvc, err := newService(prevState.ProfileID, prevState.DoHServer, prevState.APIServer); err == nil {
			if err := oldSvc.Install(); err != nil {
				log.Printf("Rollback warning: failed to reinstall previous service config: %v", err)
			}
			if err := oldSvc.Start(); err != nil {
				log.Printf("Rollback warning: failed to start previous service config: %v", err)
			}
		} else {
			log.Printf("Rollback warning: could not create previous service config: %v", err)
		}
	} else if prevInstalled {
		log.Printf("Rollback warning: previous service config unknown; manual reinstall may be required.")
	}

	if prevDNSLocal {
		if err := host.SetDNS("127.0.0.1"); err != nil {
			log.Printf("Rollback warning: failed to restore local DNS setting: %v", err)
		}
	} else {
		if err := host.ResetDNS(); err != nil {
			log.Printf("Rollback warning: failed to restore DNS defaults: %v", err)
		}
	}
}

func ServiceStart() error {
	svc, err := newService("", "", "")
	if err != nil {
		return err
	}
	return svc.Start()
}

func ServiceStop() error {
	svc, err := newService("", "", "")
	if err != nil {
		return err
	}
	return svc.Stop()
}

func ShowStatus() {
	dns := currentSystemDNS()
	localDNS := hasDNS127001(dns)

	svcState := "unknown"
	if runtime.GOOS == "windows" {
		if st, ok := windowsServiceState(); ok {
			svcState = st
		}
	} else if svc, err := newService("", "", ""); err == nil {
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
		fmt.Printf("System DNS: %s (includes 127.0.0.1 via uBlockDNS)\n", strings.Join(dns, ", "))
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

func currentSystemDNS() []string {
	// On macOS, host.DNS() can report stale/non-primary resolver values.
	// Prefer scutil output when available.
	if runtime.GOOS == "darwin" {
		if dns, err := dnsFromScutil(); err == nil && len(dns) > 0 {
			return dns
		}
	}
	// On Windows, host.DNS() can return empty on some adapter setups.
	// Prefer native adapter DNS query output when available.
	if runtime.GOOS == "windows" {
		if dns, err := dnsFromWindowsPowerShell(); err == nil && len(dns) > 0 {
			return dns
		}
	}
	return host.DNS()
}

func dnsFromScutil() ([]string, error) {
	out, err := commandOutput("scutil", "--dns")
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`nameserver\[[0-9]+\]\s*:\s*([^\s]+)`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return nil, nil
	}
	raw := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		raw = append(raw, m[1])
	}
	return collectUniqueNonEmpty(raw), nil
}

func dnsFromWindowsPowerShell() ([]string, error) {
	out, err := commandOutput(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		`Get-DnsClientServerAddress -AddressFamily IPv4 | ForEach-Object { $_.ServerAddresses } | Where-Object { $_ }`,
	)
	if err != nil {
		return nil, err
	}
	return collectUniqueNonEmpty(strings.Split(string(out), "\n")), nil
}

func windowsServiceState() (string, bool) {
	out, err := commandCombinedOutput("sc.exe", "query", serviceName)
	text := string(out)
	if err != nil {
		if strings.Contains(text, "FAILED 1060") {
			return "not-installed", true
		}
		return "unknown", false
	}

	m := regexp.MustCompile(`STATE\s*:\s*(\d+)`).FindStringSubmatch(text)
	if len(m) < 2 {
		return "unknown", false
	}
	switch m[1] {
	case "1":
		return "stopped", true
	case "4":
		return "running", true
	default:
		return "unknown", true
	}
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
		DisplayName: "uBlockDNS",
		Description: "DNS-level ad blocker - routes DNS through ublockdns.com",
		Arguments:   args,
	})
}
