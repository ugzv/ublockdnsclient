//go:build !windows

package service

import (
	"github.com/nextdns/nextdns/host/service"
)

func platformServiceState() (string, error) {
	svc, err := baseService()
	if err != nil {
		return "unknown", err
	}
	st, err := svc.Status()
	if err != nil {
		return "unknown", err
	}
	return mapServiceStatus(st), nil
}

func mapServiceStatus(st service.Status) string {
	switch st {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	case service.StatusNotInstalled:
		return "not-installed"
	default:
		return "unknown"
	}
}
