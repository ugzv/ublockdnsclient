package service

import (
	"errors"
	"testing"
	"time"

	"github.com/ugzv/ublockdnsclient/internal/state"
)

type statusTestEnv struct {
	serviceState          func() (string, error)
	resolveSystemDNS      func() systemDNSAssessment
	localDNSProbe         func() error
	loadInstallState      func() (state.InstallState, error)
	hostDNS               func() []string
	commandOutput         func(name string, args ...string) ([]byte, error)
	commandCombinedOutput func(name string, args ...string) ([]byte, error)
	readFile              func(name string) ([]byte, error)
	currentStatus         func() StatusInfo
	now                   func() time.Time
	sleep                 func(time.Duration)
}

func withStatusTestEnv(t *testing.T, env statusTestEnv) {
	t.Helper()

	oldServiceState := serviceStateFunc
	oldResolveSystemDNS := resolveSystemDNSFunc
	oldProbe := localDNSProbeFunc
	oldLoadInstallState := loadInstallState
	oldHostDNS := hostDNSFunc
	oldCommandOutput := commandOutputFunc
	oldCommandCombinedOutput := commandCombinedOutputFunc
	oldReadFile := readFileFunc
	oldCurrentStatus := currentStatusFunc
	oldNow := nowFunc
	oldSleep := sleepFunc

	t.Cleanup(func() {
		serviceStateFunc = oldServiceState
		resolveSystemDNSFunc = oldResolveSystemDNS
		localDNSProbeFunc = oldProbe
		loadInstallState = oldLoadInstallState
		hostDNSFunc = oldHostDNS
		commandOutputFunc = oldCommandOutput
		commandCombinedOutputFunc = oldCommandCombinedOutput
		readFileFunc = oldReadFile
		currentStatusFunc = oldCurrentStatus
		nowFunc = oldNow
		sleepFunc = oldSleep
	})

	if env.serviceState != nil {
		serviceStateFunc = env.serviceState
	}
	if env.resolveSystemDNS != nil {
		resolveSystemDNSFunc = env.resolveSystemDNS
	}
	if env.localDNSProbe != nil {
		localDNSProbeFunc = env.localDNSProbe
	}
	if env.loadInstallState != nil {
		loadInstallState = env.loadInstallState
	}
	if env.hostDNS != nil {
		hostDNSFunc = env.hostDNS
	}
	if env.commandOutput != nil {
		commandOutputFunc = env.commandOutput
	}
	if env.commandCombinedOutput != nil {
		commandCombinedOutputFunc = env.commandCombinedOutput
	}
	if env.readFile != nil {
		readFileFunc = env.readFile
	}
	if env.currentStatus != nil {
		currentStatusFunc = env.currentStatus
	}
	if env.now != nil {
		nowFunc = env.now
	}
	if env.sleep != nil {
		sleepFunc = env.sleep
	}
}

func localSystemDNS() systemDNSAssessment {
	return systemDNSAssessment{
		DNS:      []string{"127.0.0.1"},
		LocalDNS: true,
	}
}

func missingInstallState() (state.InstallState, error) {
	return state.InstallState{}, errors.New("missing")
}
