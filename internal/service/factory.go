package service

import (
	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/host/service"
	"github.com/ugzv/ublockdnsclient/internal/core"
)

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
	return newService("", "", "")
}
