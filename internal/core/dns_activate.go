package core

import (
	"errors"
	"fmt"
	"log"

	"github.com/nextdns/nextdns/host"
)

const LocalDNSAddress = "127.0.0.1"

var (
	setSystemDNSFunc   = host.SetDNS
	resetSystemDNSFunc = host.ResetDNS
)

// ActivateSystemDNS points the host resolver at the local uBlockDNS proxy via nextdns.
// Non-Linux platforms use this through ActivatePlatformSystemDNS.
func ActivateSystemDNS() error {
	return setSystemDNSFunc(LocalDNSAddress)
}

// ActivatePlatformSystemDNS applies the platform-specific system DNS install path.
// Linux uses the durable uBlockDNS layer only; other platforms use nextdns SetDNS.
func ActivatePlatformSystemDNS() error {
	return activatePlatformSystemDNS()
}

// ActivatePlatformSystemDNSBestEffort logs and continues when activation fails.
func ActivatePlatformSystemDNSBestEffort() {
	if err := ActivatePlatformSystemDNS(); err != nil {
		log.Printf("Warning: failed to activate system DNS: %v", err)
	}
}

// RestoreSystemDNS restores DNS for durable, legacy nextdns-only, and mixed installs.
func RestoreSystemDNS(strict bool) ([]string, error) {
	return restoreSystemDNS(strict)
}

// RestoreSystemDNSStrict fails when any restore step fails.
func RestoreSystemDNSStrict() error {
	_, err := RestoreSystemDNS(true)
	return err
}

// RestoreSystemDNSBestEffort logs and continues when restore fails.
func RestoreSystemDNSBestEffort() {
	_, _ = RestoreSystemDNS(false)
}

// RestoreSystemDNSWithWarnings restores DNS best-effort and returns human-readable issues.
func RestoreSystemDNSWithWarnings() []string {
	warnings, _ := RestoreSystemDNS(false)
	return warnings
}

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

	record("restore install artifacts", restorePlatformInstallArtifacts())
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
