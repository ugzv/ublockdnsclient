package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/nextdns/nextdns/host/service"
	"github.com/nextdns/nextdns/proxy"
	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/nextdns/nextdns/resolver/query"
)

type proxyRunner struct {
	proxy   proxy.Proxy
	onInit  []func(ctx context.Context)
	cancel  context.CancelFunc
	stopped chan struct{}
}

func (p *proxyRunner) Start() error {
	errC := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.stopped = make(chan struct{})

	for _, f := range p.onInit {
		go f(ctx)
	}

	go func() {
		defer close(p.stopped)
		if err := p.proxy.ListenAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case errC <- err:
			default:
			}
		}
	}()

	// Match NextDNS service startup behavior: return quickly after spawn,
	// while still surfacing immediate startup failures.
	select {
	case err := <-errC:
		return err
	case <-time.After(5 * time.Second):
		return nil
	}
}

func (p *proxyRunner) Stop() error {
	if p.cancel == nil {
		return nil
	}
	p.cancel()
	p.cancel = nil
	if p.stopped != nil {
		<-p.stopped
	}
	return nil
}

func (p *proxyRunner) Log(msg string) {
	log.Println(msg)
}

// run starts the DNS proxy in the foreground.
func Run(version, profileID, overrideServer, overrideAPIServer, accountToken string) error {
	listenAddr := "127.0.0.1:53"
	dohServer := ResolveDoHServer(overrideServer)
	apiServer := ResolveAPIServer(overrideAPIServer, dohServer)
	dohURL, dohHostname, dohPath, err := BuildDoHTarget(dohServer, profileID)
	if err != nil {
		return err
	}

	log.Printf("uBlockDNS CLI v%s", version)
	log.Printf("Profile: %s", profileID)
	log.Printf("DoH upstream: %s", dohURL)
	log.Printf("API server: %s", apiServer)

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
					return dohURL, profileID
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

	if token := strings.TrimSpace(accountToken); token != "" {
		accountToken = token
	} else if token := strings.TrimSpace(os.Getenv("UBLOCKDNS_ACCOUNT_TOKEN")); token != "" {
		accountToken = token
	} else if persisted, err := loadPersistedToken(profileID); err == nil {
		accountToken = persisted
	}

	if strings.TrimSpace(accountToken) != "" {
		onInit = append(onInit, func(ctx context.Context) {
			watchRulesUpdates(ctx, apiServer, profileID, accountToken, dnsCache.Purge)
		})
	} else {
		log.Printf("Rules update stream disabled: no -token provided (cache still expires naturally)")
	}

	runner := &proxyRunner{
		proxy:  p,
		onInit: onInit,
	}
	if err := service.Run(serviceName, runner); err != nil {
		log.Printf("Startup failed: %v", err)
		return err
	}
	return nil
}

func ResolveDoHServer(overrideServer string) string {
	if overrideServer = strings.TrimSpace(overrideServer); overrideServer != "" {
		return strings.TrimRight(overrideServer, "/")
	}
	if fromEnv := strings.TrimSpace(os.Getenv("UBLOCKDNS_DOH_SERVER")); fromEnv != "" {
		return strings.TrimRight(fromEnv, "/")
	}
	return DefaultDoHServer
}

func ResolveAPIServer(overrideServer, _ string) string {
	if overrideServer = strings.TrimSpace(overrideServer); overrideServer != "" {
		return strings.TrimRight(overrideServer, "/")
	}
	if fromEnv := strings.TrimSpace(os.Getenv("UBLOCKDNS_API_SERVER")); fromEnv != "" {
		return strings.TrimRight(fromEnv, "/")
	}
	return DefaultAPIServer
}

func BuildDoHTarget(base, profileID string) (string, string, string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid DoH server URL %q: %w", base, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", "", fmt.Errorf("invalid DoH server URL %q: expected absolute URL", base)
	}
	if err := ValidateProfileID(profileID); err != nil {
		return "", "", "", err
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(profileID)
	return u.String(), u.Hostname(), u.Path, nil
}
