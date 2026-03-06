package service

import (
	"fmt"
	"log"
	"strings"

	"github.com/nextdns/nextdns/host"
	"github.com/ugzv/ublockdnsclient/internal/core"
	"github.com/ugzv/ublockdnsclient/internal/state"
)

type InstallOutcome string

const (
	InstallOutcomeFresh    InstallOutcome = "fresh"
	InstallOutcomeUpdated  InstallOutcome = "updated"
	InstallOutcomeSwitched InstallOutcome = "switched"
)

// install sets up ublockdns as a system service.
func Install(profileID, dohServer, apiServer, accountToken string) error {
	_, err := InstallDetailed(profileID, dohServer, apiServer, accountToken)
	return err
}

// InstallDetailed installs (or reinstalls) the service and returns a UX-friendly outcome.
func InstallDetailed(profileID, dohServer, apiServer, accountToken string) (InstallOutcome, error) {
	if !hasInstallPrivileges() {
		return InstallOutcomeFresh, fmt.Errorf("install requires elevated privileges - %s", installPrivilegeHint())
	}

	prevDNSLocal := core.HasDNS127001(currentSystemDNS())
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

	// Uninstall any previous version first.
	_ = svc.Stop()
	_ = svc.Uninstall()

	log.Println("Installing service...")
	if err := svc.Install(); err != nil {
		rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState, prevState)
		return outcome, fmt.Errorf("install service: %w", err)
	}

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
	if err := core.CheckLocalDNSProxy("example.com"); err != nil {
		log.Printf("Warning: local DNS preflight failed (continuing): %v", err)
	}

	log.Println("Setting system DNS to 127.0.0.1...")
	if err := host.SetDNS("127.0.0.1"); err != nil {
		rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState, prevState)
		return outcome, fmt.Errorf("set system DNS: %w", err)
	}

	if err := state.PersistInstallState(profileID, dohServer, apiServer); err != nil {
		log.Printf("Warning: failed to persist install state: %v", err)
	}

	return outcome, nil
}

func rollbackInstall(prevInstalled, prevDNSLocal, hasPrevState bool, prevState state.InstallState) {
	log.Printf("Install failed, attempting rollback...")

	if svc, err := baseService(); err == nil {
		_ = svc.Stop()
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
		if err := host.SetDNS("127.0.0.1"); err != nil {
			log.Printf("Rollback warning: failed to restore local DNS setting: %v", err)
		}
	} else {
		if err := host.ResetDNS(); err != nil {
			log.Printf("Rollback warning: failed to restore DNS defaults: %v", err)
		}
	}
}
