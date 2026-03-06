package service

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/host/service"
	"github.com/ugzv/ublockdnsclient/internal/core"
	"github.com/ugzv/ublockdnsclient/internal/state"
)

type StatusInfo struct {
	Status    string   `json:"status"`
	Ready     bool     `json:"ready"`
	SystemDNS []string `json:"system_dns"`
	LocalDNS  bool     `json:"local_dns"`
	Service   string   `json:"service_state"`
	ProfileID string   `json:"profile_id,omitempty"`
	DoHServer string   `json:"doh_server,omitempty"`
	APIServer string   `json:"api_server,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

var (
	currentStatusFunc = CurrentStatus
	localDNSProbeFunc = func() error {
		return core.CheckLocalDNSProxy("example.com")
	}
	nowFunc   = time.Now
	sleepFunc = time.Sleep
)

func serviceCurrentlyInstalled() bool {
	st, err := serviceState()
	if err != nil {
		return false
	}
	return st != "not-installed"
}

func CurrentStatus() StatusInfo {
	dns := currentSystemDNS()
	localDNS := core.HasDNS127001(dns)

	svcState := "unknown"
	if st, err := serviceState(); err == nil {
		svcState = st
	}

	ready := localDNS
	if svcState == "stopped" || svcState == "not-installed" {
		ready = false
	}

	status := "inactive"
	if ready {
		status = "active"
	}

	info := StatusInfo{
		Status:    status,
		Ready:     ready,
		SystemDNS: dns,
		LocalDNS:  localDNS,
		Service:   svcState,
	}
	if st, err := state.LoadInstallState(); err == nil {
		info.ProfileID = st.ProfileID
		info.DoHServer = st.DoHServer
		info.APIServer = st.APIServer
	}

	if svcState == "running" && !localDNS {
		info.Warnings = append(info.Warnings, "service is running but system DNS is not pointing to 127.0.0.1")
	}
	if localDNS && svcState != "running" && svcState != "unknown" {
		info.Warnings = append(info.Warnings, "system DNS includes 127.0.0.1 but service is not running")
	}

	return info
}

func ShowStatus() {
	showStatus(CurrentStatus())
}

func showStatus(info StatusInfo) {
	fmt.Printf("Status: %s\n", info.Status)

	if len(info.SystemDNS) == 0 {
		fmt.Println("System DNS: (none)")
	} else if info.LocalDNS {
		fmt.Printf("System DNS: %s (includes 127.0.0.1 via uBlockDNS)\n", strings.Join(info.SystemDNS, ", "))
	} else {
		fmt.Printf("System DNS: %s\n", strings.Join(info.SystemDNS, ", "))
	}

	fmt.Printf("Service: %s\n", info.Service)
	for _, warning := range info.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func ShowStatusJSON() error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(CurrentStatus())
}

func WaitUntilReady(timeout time.Duration) (StatusInfo, error) {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	deadline := nowFunc().Add(timeout)
	var last StatusInfo
	var lastProbeErr error
	for {
		last = currentStatusFunc()
		if last.Ready {
			if err := localDNSProbeFunc(); err == nil {
				return last, nil
			} else {
				lastProbeErr = err
			}
		}
		if nowFunc().After(deadline) {
			if lastProbeErr != nil {
				return last, fmt.Errorf("uBlockDNS did not become ready within %v: local DNS probe failed: %w", timeout, lastProbeErr)
			}
			return last, fmt.Errorf("uBlockDNS did not become ready within %v", timeout)
		}
		sleepFunc(time.Second)
	}
}

func serviceState() (string, error) {
	if runtime.GOOS == "windows" {
		if st, ok := windowsServiceState(); ok {
			return st, nil
		}
		return "unknown", fmt.Errorf("could not determine windows service state")
	}

	svc, err := baseService()
	if err != nil {
		return "unknown", err
	}
	st, err := svc.Status()
	if err != nil {
		return "unknown", err
	}
	return mapServiceStatus(st), nil
}

func mapServiceStatus(st service.Status) string {
	switch st {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	case service.StatusNotInstalled:
		return "not-installed"
	default:
		return "unknown"
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
	out, err := core.CommandOutput("scutil", "--dns")
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
	return core.CollectUniqueNonEmpty(raw), nil
}

func dnsFromWindowsPowerShell() ([]string, error) {
	out, err := core.CommandOutput(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		`Get-DnsClientServerAddress -AddressFamily IPv4 | ForEach-Object { $_.ServerAddresses } | Where-Object { $_ }`,
	)
	if err != nil {
		return nil, err
	}
	return core.CollectUniqueNonEmpty(strings.Split(string(out), "\n")), nil
}

func windowsServiceState() (string, bool) {
	out, err := core.CommandCombinedOutput("sc.exe", "query", core.ServiceName)
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
