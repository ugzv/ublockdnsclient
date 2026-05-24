package service

import (
	"fmt"
	"log"

	"github.com/nextdns/nextdns/host/service"
	"github.com/ugzv/ublockdnsclient/internal/core"
	"github.com/ugzv/ublockdnsclient/internal/state"
)

var (
	activatePlatformSystemDNSFunc    = core.ActivatePlatformSystemDNS
	restoreSystemDNSStrictFunc       = core.RestoreSystemDNSStrict
	restoreSystemDNSBestEffortFunc   = core.RestoreSystemDNSBestEffort
	restoreSystemDNSWithWarningsFunc = core.RestoreSystemDNSWithWarnings
)

// UninstallResult summarizes service removal and DNS restoration.
type UninstallResult struct {
	Warnings []string
}

// Uninstall removes the system service and restores DNS.
func Uninstall() (UninstallResult, error) {
	result := UninstallResult{}

	svc, err := baseService()
	if err != nil {
		return result, err
	}

	_ = stopService(svc)
	result.Warnings = append(result.Warnings, restoreSystemDNSWithWarningsFunc()...)

	if err := svc.Uninstall(); err != nil {
		return result, fmt.Errorf("uninstall service: %w", err)
	}
	_ = state.ClearPersistedTokens()

	removeServiceConfigBestEffort()

	return result, nil
}

// ServiceStart starts the service when needed and strictly re-applies system DNS.
func ServiceStart() error {
	svc, err := baseService()
	if err != nil {
		return err
	}
	_ = svc.Start()

	st, err := svc.Status()
	if err != nil {
		return fmt.Errorf("service status: %w", err)
	}
	if st != service.StatusRunning {
		return fmt.Errorf("service is not running")
	}
	if err := activatePlatformSystemDNSFunc(); err != nil {
		return fmt.Errorf("activate system DNS: %w", err)
	}
	return nil
}

func ServiceStop() error {
	svc, err := baseService()
	if err != nil {
		return err
	}
	return stopServiceAndRestoreDNS(svc)
}

func stopServiceAndRestoreDNS(svc service.Service) error {
	err := stopService(svc)
	restoreSystemDNSBestEffortFunc()
	return err
}

func stopService(svc service.Service) error {
	if err := svc.Stop(); err != nil {
		log.Printf("Warning: failed to stop service: %v", err)
		return err
	}
	return nil
}
