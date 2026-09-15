package service

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ugzv/ublockdnsclient/internal/core"
	"github.com/ugzv/ublockdnsclient/internal/state"
)

type InstallOutcome string

const (
	InstallOutcomeFresh    InstallOutcome = "fresh"
	InstallOutcomeUpdated  InstallOutcome = "updated"
	InstallOutcomeSwitched InstallOutcome = "switched"
)

// localDNSPreflightTimeout bounds how long install waits for the freshly
// started proxy to answer before reporting the preflight as failed.
const localDNSPreflightTimeout = 10 * time.Second

// installPreconditions is the service state an install starts from.
type installPreconditions struct {
	installed bool
	running   bool
}

// assessInstallPreconditions interprets the platform service state. An
// unreadable state is treated as "nothing there": install proceeds, and the
// steps that would have run are ones the following Uninstall covers anyway.
func assessInstallPreconditions(serviceState string, err error) installPreconditions {
	if err != nil {
		return installPreconditions{}
	}
	return installPreconditions{
		installed: serviceState != "" && serviceState != "not-installed",
		running:   serviceState == "running",
	}
}

// waitForLocalDNSProxy polls the local proxy until it answers or the timeout
// elapses, returning the last probe error.
func waitForLocalDNSProxy(timeout time.Duration, servers ...string) error {
	deadline := nowFunc().Add(timeout)
	for {
		err := localDNSProbeFunc(servers...)
		if err == nil {
			return nil
		}
		if !nowFunc().Before(deadline) {
			return err
		}
		sleepFunc(250 * time.Millisecond)
	}
}

// InstallDetailed installs (or reinstalls) the service and returns a UX-friendly outcome.
func InstallDetailed(profileID, dohServer, apiServer, accountToken string) (InstallOutcome, error) {
	if !hasInstallPrivileges() {
		return InstallOutcomeFresh, fmt.Errorf("install requires elevated privileges - %s", installPrivilegeHint())
	}

	prevDNS := core.LocalDNSAddresses(resolveSystemDNSFunc().DNS)
	prev := assessInstallPreconditions(serviceStateFunc())
	prevState, prevStateErr := state.LoadInstallState()
	hasPrevState := prevStateErr == nil && strings.TrimSpace(prevState.ProfileID) != ""

	outcome := InstallOutcomeFresh
	if prev.installed {
		outcome = InstallOutcomeUpdated
		if hasPrevState && prevState.ProfileID != profileID {
			outcome = InstallOutcomeSwitched
		}
	}

	svc, err := newService(profileID, dohServer, apiServer)
	if err != nil {
		return outcome, err
	}

	// Uninstall any previous version first. Only stop a service that is
	// actually running: every service manager reports an error when asked to
	// stop something that is already stopped or absent, and those errors were
	// surfacing as warnings that read like install failures.
	//
	//   systemd: Unit ublockdns.service not loaded          (fresh install)
	//   launchd: Unload failed: 5: Input/output error        (reinstall, since
	//            install.sh stops the service before this runs)
	//
	// Uninstall below removes the registration either way, and DNS artifacts
	// are always cleared, since a rolled-back install can leave them behind
	// with no service registered.
	if prev.running {
		_ = stopService(svc)
	}
	restoreSystemDNSBestEffortFunc()
	_ = svc.Uninstall()

	log.Println("Installing service...")
	if err := svc.Install(); err != nil {
		return outcome, errors.Join(fmt.Errorf("install service: %w", err),
			rollbackInstall(prev, prevDNS, hasPrevState, prevState))
	}
	ensureSystemdRestartPolicy()

	// Install must fail if the service cannot start; unlike ServiceStart, this
	// is not a repair path and we need a hard guarantee before activating DNS.
	log.Println("Starting service...")
	if err := svc.Start(); err != nil {
		// Rollback: remove service if it can't start.
		_ = svc.Uninstall()
		return outcome, errors.Join(fmt.Errorf("start service: %w", err),
			rollbackInstall(prev, prevDNS, hasPrevState, prevState))
	}

	if strings.TrimSpace(accountToken) != "" {
		if err := state.PersistToken(profileID, accountToken); err != nil {
			log.Printf("Warning: failed to persist account token for rules stream: %v", err)
		}
	}

	// Best-effort readiness probe only. Do not fail install if upstream DNS is
	// temporarily unavailable (matches NextDNS install behavior).
	//
	// The proxy finishes binding asynchronously after Start returns, so probing
	// once immediately reported "connection refused" on healthy installs and
	// then succeeded moments later. Poll instead, and warn only if it never
	// comes up.
	if err := waitForLocalDNSProxy(localDNSPreflightTimeout); err != nil {
		log.Printf("Warning: local DNS preflight failed (continuing): %v", err)
	}

	// Strict activation is the install contract even though the daemon also
	// best-effort activates on startup via manageSystemDNS.
	log.Println("Verifying system DNS points to 127.0.0.1...")
	if err := activatePlatformSystemDNSFunc(); err != nil {
		return outcome, errors.Join(fmt.Errorf("activate system DNS: %w", err),
			rollbackInstall(prev, prevDNS, hasPrevState, prevState))
	}

	if err := state.PersistInstallState(profileID, dohServer, apiServer); err != nil {
		log.Printf("Warning: failed to persist install state: %v", err)
	}

	return outcome, nil
}

func rollbackInstall(prev installPreconditions, prevDNS []string, hasPrevState bool, prevState state.InstallState) error {
	log.Printf("Install failed, attempting rollback...")
	if svc, err := baseService(); err == nil {
		_ = stopServiceAndRestoreDNS(svc)
		_ = svc.Uninstall()
	}

	recoverService := func() error {
		if !prev.installed {
			return nil
		}
		if !hasPrevState {
			return errors.New("previous service config unknown; manual reinstall may be required")
		}
		svc, err := newService(prevState.ProfileID, prevState.DoHServer, prevState.APIServer)
		if err != nil {
			return err
		}
		if err := svc.Install(); err != nil {
			return fmt.Errorf("reinstall previous service: %w", err)
		}
		if !prev.running {
			return nil
		}
		if err := svc.Start(); err != nil {
			return fmt.Errorf("restart previous service: %w", err)
		}
		return waitForLocalDNSProxy(localDNSPreflightTimeout, prevDNS...)
	}

	err := recoverService()
	if err == nil && prev.running && len(prevDNS) > 0 {
		err = activatePlatformSystemDNSFunc()
		if err == nil {
			return nil
		}
	}
	// Never restore loopback DNS when recovery could not establish a working proxy.
	err = errors.Join(err, restoreSystemDNSStrictFunc())
	if err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	return nil
}
