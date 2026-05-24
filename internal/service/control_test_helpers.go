package service

import (
	"testing"

	hostservice "github.com/nextdns/nextdns/host/service"
)

type controlTestEnv struct {
	baseService                  func() (hostservice.Service, error)
	activatePlatformSystemDNS    func() error
	restoreSystemDNSBestEffort   func()
	restoreSystemDNSWithWarnings func() []string
}

func withControlTestEnv(t *testing.T, env controlTestEnv) {
	t.Helper()

	oldBase := baseServiceFunc
	oldActivate := activatePlatformSystemDNSFunc
	oldRestoreBestEffort := restoreSystemDNSBestEffortFunc
	oldRestoreWithWarnings := restoreSystemDNSWithWarningsFunc

	t.Cleanup(func() {
		baseServiceFunc = oldBase
		activatePlatformSystemDNSFunc = oldActivate
		restoreSystemDNSBestEffortFunc = oldRestoreBestEffort
		restoreSystemDNSWithWarningsFunc = oldRestoreWithWarnings
	})

	if env.baseService != nil {
		baseServiceFunc = env.baseService
	}
	if env.activatePlatformSystemDNS != nil {
		activatePlatformSystemDNSFunc = env.activatePlatformSystemDNS
	}
	if env.restoreSystemDNSBestEffort != nil {
		restoreSystemDNSBestEffortFunc = env.restoreSystemDNSBestEffort
	}
	if env.restoreSystemDNSWithWarnings != nil {
		restoreSystemDNSWithWarningsFunc = env.restoreSystemDNSWithWarnings
	}
}
