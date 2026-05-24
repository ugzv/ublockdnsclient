package service

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCurrentStatusIncludesProbeFailureDetails(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		serviceState:     func() (string, error) { return "running", nil },
		resolveSystemDNS: func() systemDNSAssessment { return localSystemDNS() },
		localDNSProbe:    func() error { return errors.New("udp timeout") },
		loadInstallState: missingInstallState,
	})

	info := CurrentStatus()
	if info.Ready {
		t.Fatalf("expected not ready status, got %+v", info)
	}
	if info.ReadyCode != "local_dns_probe_failed" {
		t.Fatalf("expected ready_code local_dns_probe_failed, got %+v", info)
	}
	if !strings.Contains(info.ProbeError, "udp timeout") {
		t.Fatalf("expected probe error to be preserved, got %+v", info)
	}
	if info.Status != "inactive" {
		t.Fatalf("expected inactive status, got %+v", info)
	}
}

func TestCurrentStatusMarksReadyAfterSuccessfulProbe(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		serviceState:     func() (string, error) { return "running", nil },
		resolveSystemDNS: func() systemDNSAssessment { return localSystemDNS() },
		localDNSProbe:    func() error { return nil },
		loadInstallState: missingInstallState,
	})

	info := CurrentStatus()
	if !info.Ready {
		t.Fatalf("expected ready status, got %+v", info)
	}
	if info.ReadyCode != "ready" {
		t.Fatalf("expected ready_code ready, got %+v", info)
	}
	if info.Status != "active" {
		t.Fatalf("expected active status, got %+v", info)
	}
}

func TestResolveSystemDNSFuncUsesInjection(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		resolveSystemDNS: func() systemDNSAssessment {
			return systemDNSAssessment{DNS: []string{"1.2.3.4"}, LocalDNS: false}
		},
	})

	assessment := resolveSystemDNSFunc()
	if len(assessment.DNS) != 1 || assessment.DNS[0] != "1.2.3.4" {
		t.Fatalf("expected injected DNS, got %+v", assessment)
	}
}

func TestEvaluateReadiness(t *testing.T) {
	tests := []struct {
		name     string
		svcState string
		localDNS bool
		probeErr error
		want     readinessVerdict
	}{
		{
			name:     "not installed",
			svcState: "not-installed",
			want: readinessVerdict{
				status: "inactive",
				code:   "service_not_installed",
				detail: "uBlockDNS service is not installed.",
			},
		},
		{
			name:     "stopped",
			svcState: "stopped",
			want: readinessVerdict{
				status: "inactive",
				code:   "service_stopped",
				detail: "uBlockDNS service is installed but not running.",
			},
		},
		{
			name:     "dns not local",
			svcState: "running",
			localDNS: false,
			want: readinessVerdict{
				status:   "inactive",
				code:     "dns_not_local",
				detail:   "System DNS does not point to 127.0.0.1.",
				warnings: []string{"service is running but system DNS is not pointing to 127.0.0.1"},
			},
		},
		{
			name:     "probe failed",
			svcState: "running",
			localDNS: true,
			probeErr: errors.New("udp timeout"),
			want: readinessVerdict{
				status:   "inactive",
				code:     "local_dns_probe_failed",
				detail:   "System DNS points to 127.0.0.1, but the local proxy did not answer a DNS probe.",
				probeErr: "udp timeout",
				warnings: []string{"local DNS probe failed: udp timeout"},
			},
		},
		{
			name:     "ready",
			svcState: "running",
			localDNS: true,
			want: readinessVerdict{
				status: "active",
				ready:  true,
				code:   "ready",
				detail: "uBlockDNS is responding on 127.0.0.1:53.",
			},
		},
		{
			name:     "local dns but stopped service",
			svcState: "stopped",
			localDNS: true,
			want: readinessVerdict{
				status:   "inactive",
				code:     "service_stopped",
				detail:   "uBlockDNS service is installed but not running.",
				warnings: []string{"system DNS includes 127.0.0.1 but service is not running"},
			},
		},
		{
			name:     "unknown service state",
			svcState: "unknown",
			localDNS: true,
			probeErr: errors.New("probe failed"),
			want: readinessVerdict{
				status:   "inactive",
				code:     "local_dns_probe_failed",
				detail:   "System DNS points to 127.0.0.1, but the local proxy did not answer a DNS probe.",
				probeErr: "probe failed",
				warnings: []string{
					"local DNS probe failed: probe failed",
					"service state could not be determined; readiness was inferred from DNS settings and probe results",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStatusTestEnv(t, statusTestEnv{
				localDNSProbe: func() error { return tt.probeErr },
			})

			got := evaluateReadiness(tt.svcState, tt.localDNS)
			if got.status != tt.want.status ||
				got.ready != tt.want.ready ||
				got.code != tt.want.code ||
				got.detail != tt.want.detail ||
				got.probeErr != tt.want.probeErr ||
				!sameStringSet(got.warnings, tt.want.warnings) {
				t.Fatalf("evaluateReadiness() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWaitUntilReadyReturnsReadinessFailure(t *testing.T) {
	now := time.Unix(0, 0)
	withStatusTestEnv(t, statusTestEnv{
		currentStatus: func() StatusInfo {
			return StatusInfo{
				Ready:       false,
				Status:      "inactive",
				Service:     "running",
				LocalDNS:    true,
				ReadyCode:   "local_dns_probe_failed",
				ReadyDetail: "System DNS points to 127.0.0.1, but the local proxy did not answer a DNS probe.",
				ProbeError:  "probe failed",
			}
		},
		now: func() time.Time { return now },
		sleep: func(time.Duration) {
			now = now.Add(30 * time.Millisecond)
		},
	})

	info, err := WaitUntilReady(50 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error when DNS probe keeps failing")
	}
	if !strings.Contains(err.Error(), "did not answer a DNS probe") {
		t.Fatalf("expected probe failure in error, got %v", err)
	}
	if info.Ready {
		t.Fatalf("expected last status to remain not ready, got %+v", info)
	}
}

func TestWaitUntilReadySucceedsAfterProbePasses(t *testing.T) {
	now := time.Unix(0, 0)
	statusCalls := 0
	withStatusTestEnv(t, statusTestEnv{
		currentStatus: func() StatusInfo {
			statusCalls++
			if statusCalls < 2 {
				return StatusInfo{
					Ready:       false,
					Status:      "inactive",
					Service:     "running",
					LocalDNS:    true,
					ReadyCode:   "local_dns_probe_failed",
					ReadyDetail: "System DNS points to 127.0.0.1, but the local proxy did not answer a DNS probe.",
					ProbeError:  "not yet",
				}
			}
			return StatusInfo{Ready: true, Status: "active", Service: "running", LocalDNS: true, ReadyCode: "ready"}
		},
		now: func() time.Time { return now },
		sleep: func(time.Duration) {
			now = now.Add(10 * time.Millisecond)
		},
	})

	info, err := WaitUntilReady(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Ready {
		t.Fatalf("expected ready status, got %+v", info)
	}
	if statusCalls != 2 {
		t.Fatalf("expected 2 readiness checks, got %d", statusCalls)
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
		if seen[v] < 0 {
			return false
		}
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
