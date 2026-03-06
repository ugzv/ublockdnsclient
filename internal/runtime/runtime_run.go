package runtime

import (
	"context"
	"fmt"
	"log"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/nextdns/nextdns/host/service"
	"github.com/nextdns/nextdns/proxy"
	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/nextdns/nextdns/resolver/query"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

// Run starts the DNS proxy in the foreground.
func Run(version, profileID, overrideServer, overrideAPIServer, accountToken string) error {
	listenAddr := "127.0.0.1:53"
	cfg, err := resolveRuntimeConfig(profileID, overrideServer, overrideAPIServer, accountToken)
	if err != nil {
		return err
	}

	dohURL, dohHostname, dohPath, err := BuildDoHTarget(cfg.DoHServer, cfg.ProfileID)
	if err != nil {
		return err
	}

	log.Printf("uBlockDNS CLI v%s", version)
	log.Printf("Profile: %s", cfg.ProfileID)
	log.Printf("DoH upstream: %s", dohURL)
	log.Printf("API server: %s", cfg.APIServer)

	var bootstrapIPs []string
	if ips, err := resolveBootstrapIPs(dohHostname); err != nil {
		log.Printf("Warning: bootstrap resolution failed: %v", err)
	} else {
		bootstrapIPs = ips
	}
	log.Printf("Listening on: %s", listenAddr)

	// Build the DoH endpoint with bootstrap IPs so it can connect
	// without relying on system DNS.
	dohEndpoint := &endpoint.DOHEndpoint{
		Hostname:  dohHostname,
		Path:      dohPath,
		Bootstrap: bootstrapIPs,
	}

	mgr := newEndpointManager(dohEndpoint)

	// Client-side DNS response cache. Avoids upstream round-trips for
	// frequently queried domains. Purged on rule updates via SSE so that
	// blocklist changes take effect immediately.
	dnsCache, err := lru.NewARC(4096)
	if err != nil {
		return fmt.Errorf("create DNS cache: %w", err)
	}

	p := proxy.Proxy{
		Addrs: []string{listenAddr},
		Upstream: &resolver.DNS{
			DOH: resolver.DOH{
				URL:   dohURL,
				Cache: dnsCache,
				GetProfileURL: func(q query.Query) (string, string) {
					return dohURL, cfg.ProfileID
				},
			},
			Manager: mgr,
		},
		QueryLog: func(qi proxy.QueryInfo) {
			log.Printf("%-5s %s %s", qi.Protocol, qi.UpstreamTransport, qi.Name)
		},
		Timeout:             5 * time.Second,
		MaxInflightRequests: 256,
		InfoLog:             func(msg string) { log.Println(msg) },
		ErrorLog:            func(err error) { log.Printf("ERROR: %v", err) },
	}
	var onInit []func(ctx context.Context)

	if cfg.AccountToken != "" {
		onInit = append(onInit, func(ctx context.Context) {
			watchRulesUpdates(ctx, cfg.APIServer, cfg.ProfileID, cfg.AccountToken, dnsCache.Purge)
		})
	} else {
		log.Printf("Rules update stream disabled: no -token provided (cache still expires naturally)")
	}

	runner := &proxyRunner{
		proxy:  p,
		onInit: onInit,
	}
	if err := service.Run(core.ServiceName, runner); err != nil {
		log.Printf("Startup failed: %v", err)
		return err
	}
	return nil
}
