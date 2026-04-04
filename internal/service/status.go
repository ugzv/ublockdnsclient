package service

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/host/service"
	"github.com/ugzv/ublockdnsclient/internal/core"
	"github.com/ugzv/ublockdnsclient/internal/state"
)

type StatusInfo struct {
	Status      string   `json:"status"`
	Ready       bool     `json:"ready"`
	ReadyCode   string   `json:"ready_code,omitempty"`
	ReadyDetail string   `json:"ready_detail,omitempty"`
	ProbeError  string   `json:"probe_error,omitempty"`
	SystemDNS   []string `json:"system_dns"`
	LocalDNS    bool     `json:"local_dns"`
	Service     string   `json:"service_state"`
	ProfileID   string   `json:"profile_id,omitempty"`
	DoHServer   string   `json:"doh_server,omitempty"`
	APIServer   string   `json:"api_server,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

const readinessProbeHost = "example.com"

var (
	currentStatusFunc = CurrentStatus
	localDNSProbeFunc = func() error {
		return core.CheckLocalDNSProxy(readinessProbeHost)
	}
	serviceStateFunc  = serviceState
	systemDNSFunc     = currentSystemDNS
	loadInstallState  = state.LoadInstallState
	nowFunc           = time.Now
	sleepFunc         = time.Sleep
	hostDNSFunc       = host.DNS
	commandOutputFunc = core.CommandOutput
	readFileFunc      = os.ReadFile
)

func serviceCurrentlyInstalled() bool {
	st, err := serviceStateFunc()
	if err != nil {
		return false
	}
	return st != "not-installed"
}

func CurrentStatus() StatusInfo {
	dns := systemDNSFunc()
	localDNS := core.HasDNS127001(dns)
	var warnings []string
	if runtime.GOOS == "linux" {
		assessment := assessLinuxResolverDNS()
		if len(assessment.DNS) > 0 {
			dns = assessment.DNS
			localDNS = assessment.LocalDNS
		}
		warnings = append(warnings, assessment.Warnings...)
	}

	svcState := "unknown"
	if st, err := serviceStateFunc(); err == nil {
		svcState = st
	}

	info := StatusInfo{
		Status:    "inactive",
		Ready:     false,
		SystemDNS: dns,
		LocalDNS:  localDNS,
		Service:   svcState,
	}
	if st, err := loadInstallState(); err == nil {
		info.ProfileID = st.ProfileID
		info.DoHServer = st.DoHServer
		info.APIServer = st.APIServer
	}
	info.Warnings = append(info.Warnings, warnings...)

	switch {
	case svcState == "not-installed":
		info.ReadyCode = "service_not_installed"
		info.ReadyDetail = "uBlockDNS service is not installed."
	case svcState == "stopped":
		info.ReadyCode = "service_stopped"
		info.ReadyDetail = "uBlockDNS service is installed but not running."
	case !localDNS:
		info.ReadyCode = "dns_not_local"
		info.ReadyDetail = "System DNS does not point to 127.0.0.1."
	default:
		if err := localDNSProbeFunc(); err != nil {
			info.ReadyCode = "local_dns_probe_failed"
			info.ReadyDetail = "System DNS points to 127.0.0.1, but the local proxy did not answer a DNS probe."
			info.ProbeError = err.Error()
			info.Warnings = append(info.Warnings, fmt.Sprintf("local DNS probe failed: %v", err))
		} else {
			info.Status = "active"
			info.Ready = true
			info.ReadyCode = "ready"
			info.ReadyDetail = "uBlockDNS is responding on 127.0.0.1:53."
		}
	}

	if svcState == "running" && !localDNS {
		info.Warnings = append(info.Warnings, "service is running but system DNS is not pointing to 127.0.0.1")
	}
	if localDNS && svcState != "running" && svcState != "unknown" {
		info.Warnings = append(info.Warnings, "system DNS includes 127.0.0.1 but service is not running")
	}
	if svcState == "unknown" {
		info.Warnings = append(info.Warnings, "service state could not be determined; readiness was inferred from DNS settings and probe results")
	}

	return info
}

func WaitUntilReady(timeout time.Duration) (StatusInfo, error) {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	deadline := nowFunc().Add(timeout)
	var last StatusInfo
	for {
		last = currentStatusFunc()
		if last.Ready {
			return last, nil
		}
		if nowFunc().After(deadline) {
			return last, waitReadyError(last, timeout)
		}
		sleepFunc(time.Second)
	}
}

func waitReadyError(info StatusInfo, timeout time.Duration) error {
	prefix := fmt.Sprintf("uBlockDNS did not become ready within %v", timeout)
	if info.ReadyDetail == "" {
		return fmt.Errorf("%s", prefix)
	}
	if info.ProbeError != "" {
		return fmt.Errorf("%s: %s (%s)", prefix, info.ReadyDetail, info.ProbeError)
	}
	return fmt.Errorf("%s: %s", prefix, info.ReadyDetail)
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
