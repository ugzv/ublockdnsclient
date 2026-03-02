package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/proxy"
	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/nextdns/nextdns/resolver/query"
)

// run starts the DNS proxy in the foreground.
func run(profileID, overrideServer, overrideAPIServer, accountToken string) error {
	listenAddr := "127.0.0.1:53"
	dohServer := resolveDoHServer(overrideServer)
	apiServer := resolveAPIServer(overrideAPIServer, dohServer)
	dohURL, dohHostname, dohPath, err := buildDoHTarget(dohServer, profileID)
	if err != nil {
		return err
	}

	log.Printf("uBlock DNS CLI v%s", version)
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

	p := proxy.Proxy{
		Addrs: []string{listenAddr},
		Upstream: &resolver.DNS{
			DOH: resolver.DOH{
				URL: dohURL,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, shutdownSignals()...)
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	if strings.TrimSpace(accountToken) != "" {
		accountToken = strings.TrimSpace(accountToken)
	} else if fromEnv := strings.TrimSpace(os.Getenv("UBLOCKDNS_ACCOUNT_TOKEN")); fromEnv != "" {
		accountToken = fromEnv
	} else if persisted, err := loadPersistedToken(profileID); err == nil {
		accountToken = persisted
	}

	if strings.TrimSpace(accountToken) != "" {
		go watchRulesUpdates(ctx, apiServer, profileID, accountToken)
	} else {
		log.Printf("Rules update stream disabled: no -token provided (cache still expires naturally)")
	}

	return p.ListenAndServe(ctx)
}

func resolveDoHServer(overrideServer string) string {
	if overrideServer != "" {
		return strings.TrimRight(overrideServer, "/")
	}
	if fromEnv := strings.TrimSpace(os.Getenv("UBLOCKDNS_DOH_SERVER")); fromEnv != "" {
		return strings.TrimRight(fromEnv, "/")
	}
	return defaultDoHServer
}

func resolveAPIServer(overrideServer, _ string) string {
	if overrideServer != "" {
		return strings.TrimRight(overrideServer, "/")
	}
	if fromEnv := strings.TrimSpace(os.Getenv("UBLOCKDNS_API_SERVER")); fromEnv != "" {
		return strings.TrimRight(fromEnv, "/")
	}
	return defaultAPIServer
}

func buildDoHTarget(base, profileID string) (string, string, string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid DoH server URL %q: %w", base, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", "", fmt.Errorf("invalid DoH server URL %q: expected absolute URL", base)
	}
	if err := validateProfileID(profileID); err != nil {
		return "", "", "", err
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(profileID)
	return u.String(), u.Hostname(), u.Path, nil
}

// resolveBootstrapIPs resolves a hostname via public resolvers (UDP/TCP),
// bypassing the system resolver. This prevents a circular dependency when
// system DNS is pointed at 127.0.0.1 (i.e., at this proxy).
func resolveBootstrapIPs(hostname string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	var errs []string

	for _, server := range bootstrapResolvers {
		for _, network := range []string{"udp", "tcp"} {
			addrs, err := lookupHostVia(server, network, hostname)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", server, network, err))
				continue
			}
			for _, addr := range addrs {
				if _, ok := seen[addr]; ok {
					continue
				}
				seen[addr] = struct{}{}
				out = append(out, addr)
			}
		}
	}

	// Fallback for locked-down networks that block direct DNS to public resolvers.
	// Only use system DNS if it's not already pointed to this local proxy.
	if len(out) == 0 && !hasDNS127001(host.DNS()) {
		addrs, err := lookupHostSystem(hostname)
		if err != nil {
			errs = append(errs, fmt.Sprintf("system resolver: %v", err))
		} else {
			for _, addr := range addrs {
				if _, ok := seen[addr]; ok {
					continue
				}
				seen[addr] = struct{}{}
				out = append(out, addr)
			}
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("bootstrap resolution failed for %s (%s)", hostname, strings.Join(errs, "; "))
	}
	return out, nil
}

func lookupHostVia(serverAddr, transport, hostname string) ([]string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, transport, serverAddr)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	addrs, err := r.LookupHost(ctx, hostname)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses found for %s", hostname)
	}
	return addrs, nil
}

func lookupHostSystem(hostname string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupHost(ctx, hostname)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses found for %s", hostname)
	}
	return addrs, nil
}

func newEndpointManager(dohEndpoint endpoint.Endpoint) *endpoint.Manager {
	m := &endpoint.Manager{
		Providers: []endpoint.Provider{
			endpoint.StaticProvider([]endpoint.Endpoint{dohEndpoint}),
			endpoint.ProviderFunc(func(ctx context.Context) ([]endpoint.Endpoint, error) {
				eps := fallbackDNSEndpoints()
				return eps, nil
			}),
		},
		InitEndpoint: dohEndpoint,
	}

	// Ensure plaintext DNS fallback remains available as last-resort connectivity.
	m.EndpointTester = func(e endpoint.Endpoint) endpoint.Tester {
		if e.Protocol() == endpoint.ProtocolDNS {
			return func(ctx context.Context, testDomain string) error { return nil }
		}
		return nil
	}
	m.GetMinTestInterval = func(e endpoint.Endpoint) time.Duration {
		if e.Protocol() == endpoint.ProtocolDNS {
			return 5 * time.Second
		}
		return 0
	}
	return m
}

func fallbackDNSEndpoints() []endpoint.Endpoint {
	seen := map[string]struct{}{}
	out := make([]endpoint.Endpoint, 0, len(host.DNS())+len(fallbackDNSServers))

	// Prefer currently configured non-loopback system resolvers.
	for _, ip := range host.DNS() {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.IsLoopback() {
			continue
		}
		addr := net.JoinHostPort(ip, "53")
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, &endpoint.DNSEndpoint{Addr: addr})
	}

	// Add known public fallbacks to avoid full outage when local resolver is unreachable.
	for _, addr := range fallbackDNSServers {
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, &endpoint.DNSEndpoint{Addr: addr})
	}

	return out
}

func checkLocalDNSProxy(hostname string) error {
	resp, err := queryDNSUDP("127.0.0.1:53", hostname)
	if err != nil {
		return fmt.Errorf("dns query failed via local proxy: %w", err)
	}

	if len(resp) < 4 {
		return errors.New("short DNS response from local proxy")
	}

	flags := binary.BigEndian.Uint16(resp[2:4])
	rcode := flags & 0x000F
	// Treat NXDOMAIN as healthy transport path (proxy is responding).
	if rcode != 0 && rcode != 3 {
		return fmt.Errorf("local proxy returned DNS rcode=%d", rcode)
	}

	return nil
}

func queryDNSUDP(serverAddr, hostname string) ([]byte, error) {
	id := uint16(rand.New(rand.NewSource(time.Now().UnixNano())).Intn(65535))
	q := buildDNSQuery(id, hostname)

	conn, err := net.DialTimeout("udp", serverAddr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(4 * time.Second)); err != nil {
		return nil, err
	}

	if _, err := conn.Write(q); err != nil {
		return nil, err
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	resp := buf[:n]
	if len(resp) < 2 {
		return nil, errors.New("short DNS response")
	}
	if binary.BigEndian.Uint16(resp[0:2]) != id {
		return nil, errors.New("mismatched DNS transaction id")
	}
	return resp, nil
}

func buildDNSQuery(id uint16, hostname string) []byte {
	q := make([]byte, 12)
	binary.BigEndian.PutUint16(q[0:2], id)
	binary.BigEndian.PutUint16(q[2:4], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(q[4:6], 1)      // QDCOUNT

	for _, label := range strings.Split(hostname, ".") {
		if label == "" {
			continue
		}
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	q = append(q, 0x00)                   // end of QNAME
	q = append(q, 0x00, 0x01, 0x00, 0x01) // QTYPE=A, QCLASS=IN
	return q
}
