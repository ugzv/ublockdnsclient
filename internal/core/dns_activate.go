package core

import (
	"errors"
	"fmt"
	"log"

	"github.com/nextdns/nextdns/host"
)

var (
	setSystemDNSFunc                    = host.SetDNS
	resetSystemDNSFunc                  = host.ResetDNS
	activatePlatformSystemDNSFunc       = activatePlatformSystemDNS
	restorePlatformInstallArtifactsFunc = restorePlatformInstallArtifacts
)

// activateSystemDNS points the host resolver at the local uBlockDNS proxy via
// nextdns. Non-Linux platforms reach it through ActivatePlatformSystemDNS.
func activateSystemDNS() error {
	return setSystemDNSFunc(LocalDNSIP)
}

// ActivatePlatformSystemDNS applies the platform-specific system DNS install path.
// Linux uses the durable uBlockDNS layer only; other platforms use nextdns SetDNS.
func ActivatePlatformSystemDNS() error {
	return activatePlatformSystemDNSFunc()
}

// ActivatePlatformSystemDNSBestEffort logs and continues when activation fails.
func ActivatePlatformSystemDNSBestEffort() {
	if err := ActivatePlatformSystemDNS(); err != nil {
		log.Printf("Warning: failed to activate system DNS: %v", err)
	}
}

// RestoreSystemDNSStrict fails when any restore step fails.
func RestoreSystemDNSStrict() error {
	_, err := restoreSystemDNS(true)
	return err
}

// RestoreSystemDNSBestEffort logs and continues when restore fails.
func RestoreSystemDNSBestEffort() {
	_, _ = restoreSystemDNS(false)
}

// RestoreSystemDNSWithWarnings restores DNS best-effort and returns human-readable issues.
func RestoreSystemDNSWithWarnings() []string {
	warnings, _ := restoreSystemDNS(false)
	return warnings
}

// restoreSystemDNS restores DNS for durable, legacy nextdns-only, and mixed installs.
func restoreSystemDNS(strict bool) ([]string, error) {
	var warnings []string
	var errs []error

	record := func(step string, err error) {
		if err == nil {
			return
		}
		if strict {
			errs = append(errs, fmt.Errorf("%s: %w", step, err))
			return
		}
		warnings = append(warnings, step+": "+err.Error())
		log.Printf("Warning: failed to %s: %v", step, err)
	}

	record("restore install artifacts", restorePlatformInstallArtifactsFunc())
	record("deactivate legacy system DNS", resetSystemDNSFunc())

	if err := FlushDNSCaches(); err != nil {
		record("flush DNS caches", err)
	}

	return warnings, errors.Join(errs...)
}

// SwapSystemDNSFuncs overrides DNS set/reset hooks and returns a restore func.
func SwapSystemDNSFuncs(set func(string) error, reset func() error) (restore func()) {
	oldSet, oldReset := setSystemDNSFunc, resetSystemDNSFunc
	if set != nil {
		setSystemDNSFunc = set
	}
	if reset != nil {
		resetSystemDNSFunc = reset
	}
	return func() {
		setSystemDNSFunc = oldSet
		resetSystemDNSFunc = oldReset
	}
}

// SwapPlatformSystemDNSFuncs overrides platform DNS hooks and returns a restore func.
func SwapPlatformSystemDNSFuncs(activate func() error, restoreArtifacts func() error) func() {
	oldActivate := activatePlatformSystemDNSFunc
	oldRestoreArtifacts := restorePlatformInstallArtifactsFunc
	if activate != nil {
		activatePlatformSystemDNSFunc = activate
	}
	if restoreArtifacts != nil {
		restorePlatformInstallArtifactsFunc = restoreArtifacts
	}
	return func() {
		activatePlatformSystemDNSFunc = oldActivate
		restorePlatformInstallArtifactsFunc = oldRestoreArtifacts
	}
}

func HasDNS127001(dns []string) bool {
	for _, d := range dns {
		if d == LocalDNSIP {
			return true
		}
	}
	return false
}
