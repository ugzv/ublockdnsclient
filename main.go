package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/nextdns/nextdns/host"
	"github.com/nextdns/nextdns/host/service"
	"github.com/nextdns/nextdns/proxy"
	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/nextdns/nextdns/resolver/query"
)

var version = "dev"

const (
	serviceName      = "ublockdns"
	defaultDoHServer = "https://my.ublockdns.com"
	defaultAPIServer = "https://ublockdns.com"
	tokenDir         = "/etc/ublockdns"
)

var bootstrapResolvers = []string{
	"1.1.1.1:53",
	"8.8.8.8:53",
	"9.9.9.9:53",
	"1.0.0.1:53",
}

var fallbackDNSServers = []string{
	"1.1.1.1:53",
	"8.8.8.8:53",
	"9.9.9.9:53",
}

func usage() {
	fmt.Fprintf(os.Stderr, `uBlock DNS CLI v%s

Usage:
  ublockdns install   -profile <id>   Install as system service and activate
                      [-server <url>] Optional DoH server base URL (for local/dev)
                      [-api-server <url>] Optional API server URL (for local/dev)
                      [-token <account-key>] Optional account key for instant rule-update cache flush
  ublockdns uninstall                  Remove service and restore DNS
  ublockdns start                      Start the service
  ublockdns stop                       Stop the service
  ublockdns run       -profile <id>    Run in foreground (for testing)
                      [-server <url>] Optional DoH server base URL (for local/dev)
                      [-api-server <url>] Optional API server URL (for local/dev)
                      [-token <account-key>] Optional account key for instant rule-update cache flush
  ublockdns status                     Show current status
  ublockdns version                    Print version

`, version)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "version":
		fmt.Printf("ublockdns-cli v%s\n", version)

	case "run":
		profileID := flagValue("-profile")
		dohServer := flagValue("-server")
		apiServer := flagValue("-api-server")
		token := flagValue("-token")
		if profileID == "" {
			fmt.Fprintln(os.Stderr, "Error: -profile <id> is required")
			os.Exit(1)
		}
		if err := run(profileID, dohServer, apiServer, token); err != nil {
			log.Fatalf("Error: %v", err)
		}

	case "install":
		profileID := flagValue("-profile")
		dohServer := flagValue("-server")
		apiServer := flagValue("-api-server")
		token := flagValue("-token")
		if profileID == "" {
			fmt.Fprintln(os.Stderr, "Error: -profile <id> is required")
			os.Exit(1)
		}
		if err := install(profileID, dohServer, apiServer, token); err != nil {
			log.Fatalf("Install failed: %v", err)
		}
		fmt.Println("uBlock DNS installed and activated.")
		fmt.Printf("All DNS queries now route through your profile: %s\n", profileID)

	case "uninstall":
		if err := uninstall(); err != nil {
			log.Fatalf("Uninstall failed: %v", err)
		}
		fmt.Println("uBlock DNS uninstalled. DNS restored to defaults.")

	case "start":
		if err := serviceStart(); err != nil {
			log.Fatalf("Start failed: %v", err)
		}
		fmt.Println("uBlock DNS started.")

	case "stop":
		if err := serviceStop(); err != nil {
			log.Fatalf("Stop failed: %v", err)
		}
		fmt.Println("uBlock DNS stopped.")

	case "status":
		showStatus()

	default:
		usage()
		os.Exit(1)
	}
}

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
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
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

func resolveAPIServer(overrideServer, dohServer string) string {
	if overrideServer != "" {
		return strings.TrimRight(overrideServer, "/")
	}
	if fromEnv := strings.TrimSpace(os.Getenv("UBLOCKDNS_API_SERVER")); fromEnv != "" {
		return strings.TrimRight(fromEnv, "/")
	}
	u, err := url.Parse(dohServer)
	if err == nil && (u.Hostname() == "my.ublockdns.com" || u.Hostname() == "dns.ublockdns.com") {
		return defaultAPIServer
	}
	return defaultAPIServer
}

type rulesVersionResponse struct {
	ProfileID      string `json:"profile_id"`
	AccountID      string `json:"account_id,omitempty"`
	RulesVersion   int64  `json:"rules_version"`
	RulesUpdatedAt int64  `json:"rules_updated_at"`
}

type rulesUpdateEvent struct {
	ProfileID          string `json:"profile_id"`
	AccountID          string `json:"account_id,omitempty"`
	RulesVersion       int64  `json:"rules_version"`
	RulesUpdatedAt     int64  `json:"rules_updated_at"`
	ListsChanged       bool   `json:"lists_changed"`
	CustomRulesChanged bool   `json:"custom_rules_changed"`
}

func watchRulesUpdates(ctx context.Context, apiServer, profileID, accountToken string) {
	token := strings.TrimSpace(accountToken)
	if token == "" {
		return
	}
	log.Printf("Rules update stream enabled for profile %s", profileID)

	currentVersion := int64(0)
	if v, err := fetchRulesVersion(ctx, apiServer, profileID, token); err == nil {
		currentVersion = v.RulesVersion
	}

	backoff := time.Second
	for {
		if err := consumeRulesStream(ctx, apiServer, profileID, token, func(ev rulesUpdateEvent) {
			if ev.RulesVersion <= currentVersion {
				return
			}
			currentVersion = ev.RulesVersion
			if err := flushDNSCaches(); err != nil {
				log.Printf("Rules updated (v%d) but DNS cache flush had issues: %v", currentVersion, err)
				return
			}
			log.Printf("Rules updated (v%d), local DNS cache flushed", currentVersion)
		}); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Rules stream disconnected: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Reconcile state after disconnect so missed events are still applied.
		if v, err := fetchRulesVersion(ctx, apiServer, profileID, token); err == nil && v.RulesVersion > currentVersion {
			currentVersion = v.RulesVersion
			if err := flushDNSCaches(); err != nil {
				log.Printf("Rules version advanced to v%d; DNS cache flush had issues: %v", currentVersion, err)
			} else {
				log.Printf("Rules version advanced to v%d, local DNS cache flushed", currentVersion)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func fetchRulesVersion(ctx context.Context, apiServer, profileID, accountToken string) (rulesVersionResponse, error) {
	var out rulesVersionResponse
	u := strings.TrimRight(apiServer, "/") + "/api/profile/" + profileID + "/rules/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+accountToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return out, fmt.Errorf("rules/version status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func consumeRulesStream(ctx context.Context, apiServer, profileID, accountToken string, onEvent func(ev rulesUpdateEvent)) error {
	u := strings.TrimRight(apiServer, "/") + "/api/profile/" + profileID + "/rules/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accountToken)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("rules/stream status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var ev rulesUpdateEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		onEvent(ev)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func flushDNSCaches() error {
	type cmdSpec struct {
		name string
		args []string
	}

	var commands []cmdSpec
	switch runtime.GOOS {
	case "darwin":
		commands = []cmdSpec{
			{name: "dscacheutil", args: []string{"-flushcache"}},
			{name: "killall", args: []string{"-HUP", "mDNSResponder"}},
		}
	case "linux":
		commands = []cmdSpec{
			{name: "resolvectl", args: []string{"flush-caches"}},
			{name: "systemd-resolve", args: []string{"--flush-caches"}},
			{name: "service", args: []string{"nscd", "restart"}},
		}
	case "windows":
		commands = []cmdSpec{
			{name: "ipconfig", args: []string{"/flushdns"}},
		}
	default:
		return nil
	}

	var errs []string
	success := false
	for _, c := range commands {
		cmd := exec.Command(c.name, c.args...)
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Sprintf("%s %s: %v", c.name, strings.Join(c.args, " "), err))
			continue
		}
		success = true
	}
	if success {
		return nil
	}
	if len(errs) == 0 {
		return errors.New("no cache flush command available")
	}
	return errors.New(strings.Join(errs, "; "))
}

func buildDoHTarget(base, profileID string) (string, string, string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid DoH server URL %q: %w", base, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", "", fmt.Errorf("invalid DoH server URL %q: expected absolute URL", base)
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/" + profileID
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
	var q []byte
	q = make([]byte, 12)
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

// install sets up ublockdns as a system service.
func install(profileID, dohServer, apiServer, accountToken string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("install requires root — run with sudo")
	}

	svc, err := newService(profileID, dohServer, apiServer)
	if err != nil {
		return err
	}

	// Uninstall any previous version first
	_ = svc.Stop()
	_ = svc.Uninstall()

	log.Println("Installing service...")
	if err := svc.Install(); err != nil {
		return fmt.Errorf("install service: %w", err)
	}

	log.Println("Starting service...")
	if err := svc.Start(); err != nil {
		// Rollback: remove service if it can't start
		_ = svc.Uninstall()
		return fmt.Errorf("start service: %w", err)
	}

	if strings.TrimSpace(accountToken) != "" {
		if err := persistToken(profileID, accountToken); err != nil {
			log.Printf("Warning: failed to persist account token for rules stream: %v", err)
		}
	}

	// Best-effort readiness probe only. Do not fail install if upstream DNS is
	// temporarily unavailable (matches NextDNS install behavior).
	if err := checkLocalDNSProxy("example.com"); err != nil {
		log.Printf("Warning: local DNS preflight failed (continuing): %v", err)
	}

	log.Println("Setting system DNS to 127.0.0.1...")
	if err := host.SetDNS("127.0.0.1"); err != nil {
		return fmt.Errorf("set system DNS: %w", err)
	}

	return nil
}

// uninstall removes the system service and restores DNS.
func uninstall() error {
	if err := host.ResetDNS(); err != nil {
		log.Printf("Warning: could not reset DNS: %v", err)
	}

	svc, err := newService("", "", "")
	if err != nil {
		return err
	}

	_ = svc.Stop()

	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}
	_ = clearPersistedTokens()

	return nil
}

func serviceStart() error {
	svc, err := newService("", "", "")
	if err != nil {
		return err
	}
	return svc.Start()
}

func serviceStop() error {
	svc, err := newService("", "", "")
	if err != nil {
		return err
	}
	return svc.Stop()
}

func showStatus() {
	dns := currentSystemDNS()
	localDNS := hasDNS127001(dns)

	svcState := "unknown"
	if svc, err := newService("", "", ""); err == nil {
		if st, err := svc.Status(); err == nil {
			switch st {
			case service.StatusRunning:
				svcState = "running"
			case service.StatusStopped:
				svcState = "stopped"
			case service.StatusNotInstalled:
				svcState = "not-installed"
			default:
				svcState = "unknown"
			}
		}
	}

	active := localDNS
	if svcState == "running" {
		active = localDNS
	}
	if svcState == "stopped" || svcState == "not-installed" {
		active = false
	}

	if active {
		fmt.Println("Status: active")
	} else {
		fmt.Println("Status: inactive")
	}

	if len(dns) == 0 {
		fmt.Println("System DNS: (none)")
	} else if localDNS {
		fmt.Printf("System DNS: %s (includes 127.0.0.1 via uBlock DNS)\n", strings.Join(dns, ", "))
	} else {
		fmt.Printf("System DNS: %s\n", strings.Join(dns, ", "))
	}

	fmt.Printf("Service: %s\n", svcState)

	if svcState == "running" && !localDNS {
		fmt.Println("Warning: service is running but system DNS is not pointing to 127.0.0.1")
	}
	if localDNS && svcState != "running" && svcState != "unknown" {
		fmt.Println("Warning: system DNS includes 127.0.0.1 but service is not running")
	}
}

func hasDNS127001(dns []string) bool {
	for _, d := range dns {
		if d == "127.0.0.1" {
			return true
		}
	}
	return false
}

func currentSystemDNS() []string {
	// On macOS, host.DNS() can report stale/non-primary resolver values.
	// Prefer scutil output when available.
	if runtime.GOOS == "darwin" {
		if dns, err := dnsFromScutil(); err == nil && len(dns) > 0 {
			return dns
		}
	}
	return host.DNS()
}

func dnsFromScutil() ([]string, error) {
	out, err := exec.Command("scutil", "--dns").Output()
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`nameserver\[[0-9]+\]\s*:\s*([^\s]+)`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	var dns []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		ip := strings.TrimSpace(m[1])
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		dns = append(dns, ip)
	}
	return dns, nil
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
		Name:        serviceName,
		DisplayName: "uBlock DNS",
		Description: "DNS-level ad blocker — routes DNS through ublockdns.com",
		Arguments:   args,
	})
}

func flagValue(name string) string {
	for i, arg := range os.Args {
		if arg == name && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func tokenPath(profileID string) string {
	return filepath.Join(tokenDir, profileID+".token")
}

func persistToken(profileID, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(tokenPath(profileID), []byte(strings.TrimSpace(token)), 0o600)
}

func loadPersistedToken(profileID string) (string, error) {
	b, err := os.ReadFile(tokenPath(profileID))
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", errors.New("empty persisted token")
	}
	return token, nil
}

func clearPersistedTokens() error {
	matches, err := filepath.Glob(filepath.Join(tokenDir, "*.token"))
	if err != nil {
		return err
	}
	for _, p := range matches {
		_ = os.Remove(p)
	}
	_ = os.Remove(tokenDir)
	return nil
}
