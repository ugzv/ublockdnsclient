package service

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/host/service"
	"github.com/ugzv/ublockdnsclient/internal/core"
	"github.com/ugzv/ublockdnsclient/internal/state"
	"github.com/ugzv/ublockdnsclient/internal/update"
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
		Name:        core.ServiceName,
		DisplayName: "uBlockDNS",
		Description: "DNS-level ad blocker - routes DNS through ublockdns.com",
		Arguments:   args,
	})
}

func baseService() (service.Service, error) {
	return baseServiceFunc()
}

var baseServiceFunc = func() (service.Service, error) {
	return newService("", "", "")
}

func removeServiceConfigBestEffort() {
	for _, path := range serviceConfigPaths() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: failed to remove service config %s: %v", path, err)
		}
	}
}

func serviceConfigPaths() []string {
	switch runtime.GOOS {
	case "windows":
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			return nil
		}
		return []string{
			filepath.Join(programFiles, "uBlockDNS", "ublockdns.conf"),
			filepath.Join(programFiles, "ublockdns", "ublockdns.conf"),
		}
	default:
		return []string{"/etc/ublockdns.conf"}
	}
}

// Upgrade updates the binary to the latest release and restarts the service.
// Returns the new version, or "" when already up to date.
func Upgrade(currentVersion, apiServer string) (string, error) {
	if !hasInstallPrivileges() {
		return "", fmt.Errorf("upgrade requires elevated privileges - %s", installPrivilegeHint())
	}
	ensureSystemdRestartPolicy()
	v, err := update.Apply(currentVersion, apiServer)
	if err != nil || v == "" {
		return v, err
	}
	if serviceCurrentlyInstalled() {
		svc, err := baseService()
		if err == nil {
			err = svc.Restart()
		}
		if err != nil {
			return v, fmt.Errorf("binary updated to v%s but service restart failed: %w", v, err)
		}
	}
	return v, nil
}
