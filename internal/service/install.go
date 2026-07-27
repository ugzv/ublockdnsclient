package service

import (
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

// waitForLocalDNSProxy polls the local proxy until it answers or the timeout
// elapses, returning the last probe error.
func waitForLocalDNSProxy(timeout time.Duration) error {
	deadline := nowFunc().Add(timeout)
	for {
		err := localDNSProbeFunc()
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

	prevDNSLocal := core.HasDNS127001(resolveSystemDNSFunc().DNS)
	prevInstalled := serviceCurrentlyInstalled()
	prevState, prevStateErr := state.LoadInstallState()
	hasPrevState := prevStateErr == nil && strings.TrimSpace(prevState.ProfileID) != ""

	outcome := InstallOutcomeFresh
	if prevInstalled {
		outcome = InstallOutcomeUpdated
		if hasPrevState && prevState.ProfileID != profileID {
			outcome = InstallOutcomeSwitched
		}
	}

	svc, err := newService(profileID, dohServer, apiServer)
	if err != nil {
		return outcome, err
	}

	// Uninstall any previous version first. Stopping is skipped when nothing is
	// installed: attempting it anyway is what printed "Warning: failed to stop
	// service: Unit ublockdns.service not loaded" on every first-time install,
	// which reads as a failure in an install that is going fine. DNS artifacts
	// are still cleared, since a previously rolled-back install can leave them
	// behind with no service registered.
	if prevInstalled {
		_ = stopService(svc)
	}
	restoreSystemDNSBestEffortFunc()
	_ = svc.Uninstall()

	log.Println("Installing service...")
	if err := svc.Install(); err != nil {
		rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState, prevState)
		return outcome, fmt.Errorf("install service: %w", err)
	}
	ensureSystemdRestartPolicy()

	// Install must fail if the service cannot start; unlike ServiceStart, this
	// is not a repair path and we need a hard guarantee before activating DNS.
	log.Println("Starting service...")
	if err := svc.Start(); err != nil {
		// Rollback: remove service if it can't start.
		_ = svc.Uninstall()
		rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState, prevState)
		return outcome, fmt.Errorf("start service: %w", err)
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
		rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState, prevState)
		return outcome, fmt.Errorf("activate system DNS: %w", err)
	}

	if err := state.PersistInstallState(profileID, dohServer, apiServer); err != nil {
		log.Printf("Warning: failed to persist install state: %v", err)
	}

	return outcome, nil
}

func rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState bool, prevState state.InstallState) {
	log.Printf("Install failed, attempting rollback...")

	if svc, err := baseService(); err == nil {
		_ = stopServiceAndRestoreDNS(svc)
		_ = svc.Uninstall()
	}

	if prevInstalled && hasPrevState {
		if oldSvc, err := newService(prevState.ProfileID, prevState.DoHServer, prevState.APIServer); err == nil {
			if err := oldSvc.Install(); err != nil {
				log.Printf("Rollback warning: failed to reinstall previous service config: %v", err)
			}
			if err := oldSvc.Start(); err != nil {
				log.Printf("Rollback warning: failed to start previous service config: %v", err)
			}
		} else {
			log.Printf("Rollback warning: could not create previous service config: %v", err)
		}
	} else if prevInstalled {
		log.Printf("Rollback warning: previous service config unknown; manual reinstall may be required.")
	}

	if prevDNSLocal {
		if err := activatePlatformSystemDNSFunc(); err != nil {
			log.Printf("Rollback warning: failed to restore local DNS setting: %v", err)
		}
	} else {
		if err := restoreSystemDNSStrictFunc(); err != nil {
			log.Printf("Rollback warning: failed to restore DNS defaults: %v", err)
		}
	}
}
