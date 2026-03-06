package service

import (
	"fmt"
	"log"

	"github.com/nextdns/nextdns/host"
	"github.com/ugzv/ublockdnsclient/internal/state"
)

// uninstall removes the system service and restores DNS.
func Uninstall() error {
	if err := host.ResetDNS(); err != nil {
		log.Printf("Warning: could not reset DNS: %v", err)
	}

	svc, err := baseService()
	if err != nil {
		return err
	}

	_ = svc.Stop()

	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}
	_ = state.ClearPersistedTokens()
	_ = state.ClearInstallState()

	return nil
}

func ServiceStart() error {
	svc, err := baseService()
	if err != nil {
		return err
	}
	return svc.Start()
}

func ServiceStop() error {
	svc, err := baseService()
	if err != nil {
		return err
	}
	return svc.Stop()
}
