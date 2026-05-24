package service

import (
	"fmt"
	"os"
	"time"

	"github.com/nextdns/nextdns/host"
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
	serviceStateFunc          = platformServiceState
	resolveSystemDNSFunc      = resolveSystemDNS
	loadInstallState          = state.LoadInstallState
	nowFunc                   = time.Now
	sleepFunc                 = time.Sleep
	hostDNSFunc               = host.DNS
	commandOutputFunc         = core.CommandOutput
	commandCombinedOutputFunc = core.CommandCombinedOutput
	readFileFunc              = os.ReadFile
)

type readinessVerdict struct {
	status   string
	ready    bool
	code     string
	detail   string
	probeErr string
	warnings []string
}

func serviceCurrentlyInstalled() bool {
	st, err := serviceStateFunc()
	if err != nil {
		return false
	}
	return st != "not-installed"
}

func CurrentStatus() StatusInfo {
	dnsAssessment := resolveSystemDNSFunc()

	svcState := "unknown"
	if st, err := serviceStateFunc(); err == nil {
		svcState = st
	}

	info := StatusInfo{
		Status:    "inactive",
		Ready:     false,
		SystemDNS: dnsAssessment.DNS,
		LocalDNS:  dnsAssessment.LocalDNS,
		Service:   svcState,
		Warnings:  append([]string(nil), dnsAssessment.Warnings...),
	}
	if st, err := loadInstallState(); err == nil {
		info.ProfileID = st.ProfileID
		info.DoHServer = st.DoHServer
		info.APIServer = st.APIServer
	}

	verdict := evaluateReadiness(svcState, dnsAssessment.LocalDNS)
	info.Status = verdict.status
	info.Ready = verdict.ready
	info.ReadyCode = verdict.code
	info.ReadyDetail = verdict.detail
	info.ProbeError = verdict.probeErr
	info.Warnings = append(info.Warnings, verdict.warnings...)

	return info
}

func evaluateReadiness(svcState string, localDNS bool) readinessVerdict {
	verdict := readinessVerdict{
		status: "inactive",
	}

	switch {
	case svcState == "not-installed":
		verdict.code = "service_not_installed"
		verdict.detail = "uBlockDNS service is not installed."
	case svcState == "stopped":
		verdict.code = "service_stopped"
		verdict.detail = "uBlockDNS service is installed but not running."
	case !localDNS:
		verdict.code = "dns_not_local"
		verdict.detail = "System DNS does not point to 127.0.0.1."
	default:
		if err := localDNSProbeFunc(); err != nil {
			verdict.code = "local_dns_probe_failed"
			verdict.detail = "System DNS points to 127.0.0.1, but the local proxy did not answer a DNS probe."
			verdict.probeErr = err.Error()
			verdict.warnings = append(verdict.warnings, fmt.Sprintf("local DNS probe failed: %v", err))
		} else {
			verdict.status = "active"
			verdict.ready = true
			verdict.code = "ready"
			verdict.detail = "uBlockDNS is responding on 127.0.0.1:53."
		}
	}

	if svcState == "running" && !localDNS {
		verdict.warnings = append(verdict.warnings, "service is running but system DNS is not pointing to 127.0.0.1")
	}
	if localDNS && svcState != "running" && svcState != "unknown" {
		verdict.warnings = append(verdict.warnings, "system DNS includes 127.0.0.1 but service is not running")
	}
	if svcState == "unknown" {
		verdict.warnings = append(verdict.warnings, "service state could not be determined; readiness was inferred from DNS settings and probe results")
	}

	return verdict
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
