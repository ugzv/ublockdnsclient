package runtime

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/ugzv/ublockdnsclient/internal/core"
)

var bootstrapResolvers = []string{
	"1.1.1.1:53",
	"8.8.8.8:53",
	"9.9.9.9:53",
	"1.0.0.1:53",
}

// resolveBootstrapIPs resolves a hostname via public resolvers (UDP/TCP),
// bypassing the system resolver. This prevents a circular dependency when
// system DNS is pointed at 127.0.0.1 (i.e., at this proxy).
func resolveBootstrapIPs(ctx context.Context, hostname string) ([]string, error) {
	return (bootstrapLookup{lookupHostVia, discoverDNSServers}).resolve(ctx, hostname)
}

type bootstrapLookup struct {
	via       func(context.Context, string, string, string) ([]string, error)
	systemDNS func(context.Context) []string
}

func (l bootstrapLookup) resolve(ctx context.Context, hostname string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Reserve half the total budget for system DNS on networks blocking public DNS.
	deadline, _ := ctx.Deadline()
	publicCtx, cancelPublic := context.WithTimeout(ctx, time.Until(deadline)/2)
	defer cancelPublic()
	ips, err := l.lookup(publicCtx, bootstrapResolvers, hostname)
	cancelPublic()
	if err == nil {
		return ips, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Query discovered DNS servers explicitly. The system resolver may already
	// point to this proxy even when DHCP advertises a non-local DNS server.
	var servers []string
	for _, entry := range l.systemDNS(ctx) {
		for _, addr := range strings.Fields(entry) {
			if ip := net.ParseIP(addr); ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				servers = append(servers, net.JoinHostPort(addr, "53"))
			}
		}
	}
	if len(servers) > 0 {
		return l.lookup(ctx, servers, hostname)
	}
	return nil, err
}

// lookup returns the first usable answer and cancels all remaining exchanges.
func (l bootstrapLookup) lookup(ctx context.Context, servers []string, hostname string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		addrs []string
		err   error
	}
	count := len(servers) * 2
	results := make(chan result, count)
	for _, server := range servers {
		for _, network := range []string{"udp", "tcp"} {
			go func() {
				addrs, err := l.via(ctx, server, network, hostname)
				if err != nil {
					err = fmt.Errorf("%s/%s: %w", server, network, err)
				}
				results <- result{addrs, err}
			}()
		}
	}
	var errs []string
	for range count {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-results:
			if r.err != nil {
				errs = append(errs, r.err.Error())
				continue
			}
			seen := map[string]struct{}{}
			var ips []string
			for _, addr := range r.addrs {
				if net.ParseIP(addr) != nil {
					ips = core.AppendUniqueString(ips, seen, addr)
				}
			}
			if len(ips) > 0 {
				return ips, nil
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("bootstrap resolution failed for %s (%s)", hostname, strings.Join(errs, "; "))
}

func lookupHostVia(ctx context.Context, serverAddr, transport, hostname string) ([]string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, transport, serverAddr)
		},
	}

	addrs, err := r.LookupHost(ctx, hostname)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses found for %s", hostname)
	}
	return addrs, nil
}

// discoverDNSServers obtains network DNS without going through the local proxy.
// External platform commands share a short, cancellable discovery budget.
func discoverDNSServers(ctx context.Context) []string {
	return discoverDNSServersWith(ctx, runtime.GOOS, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.WaitDelay = 100 * time.Millisecond
		return cmd.Output()
	}, os.ReadFile)
}

func discoverDNSServersWith(ctx context.Context, platform string, command func(context.Context, string, ...string) ([]byte, error), readFile func(string) ([]byte, error)) []string {
	budget := time.Second
	if platform == "windows" {
		budget = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if ctx.Err() != nil {
		return nil
	}
	var entries []string
	switch platform {
	case "windows":
		if b, err := command(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
			`Get-DnsClientServerAddress | ForEach-Object { $_.ServerAddresses } | Where-Object { $_ }`); err == nil {
			entries = strings.Fields(string(b))
		}
	case "darwin":
		if b, err := command(ctx, "ipconfig", "getoption", "", "domain_name_server"); err == nil {
			entries = strings.Fields(string(b))
		}
	case "linux":
		if b, err := command(ctx, "nmcli", "dev", "show"); err == nil {
			for line := range strings.SplitSeq(string(b), "\n") {
				if strings.HasPrefix(line, "IP4.DNS") || strings.HasPrefix(line, "IP6.DNS") {
					_, value, _ := strings.Cut(line, ":")
					entries = append(entries, strings.Fields(value)...)
				}
			}
		}
		fallthrough
	case "freebsd", "openbsd", "netbsd", "dragonfly":
		for _, path := range []string{"/run/systemd/resolve/resolv.conf", "/etc/resolv.conf"} {
			if ctx.Err() != nil {
				break
			}
			if b, err := readFile(path); err == nil {
				for line := range strings.SplitSeq(string(b), "\n") {
					fields := strings.Fields(line)
					if len(fields) >= 2 && fields[0] == "nameserver" {
						entries = append(entries, fields[1])
					}
				}
			}
		}
	}
	seen := map[string]struct{}{}
	var servers []string
	for _, entry := range entries {
		if ip := net.ParseIP(entry); ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
			servers = core.AppendUniqueString(servers, seen, ip.String())
		}
	}
	return servers
}

type endpointTester interface {
	Test(context.Context) error
}

var resolveBootstrapIPsFunc = resolveBootstrapIPs

// bootstrapRefresher re-resolves the DoH hostname in the background and swaps
// in a new endpoint when its IPs change, so a server IP migration does not
// require a daemon restart. At most one refresh runs at a time.
type bootstrapRefresher struct {
	hostname, path string
	ep             *atomic.Pointer[endpoint.DOHEndpoint]
	mgr            endpointTester
	busy           atomic.Bool
}

func (b *bootstrapRefresher) refresh(ctx context.Context) {
	if !b.busy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer b.busy.Store(false)
		ips, err := resolveBootstrapIPsFunc(ctx, b.hostname)
		if err != nil || ctx.Err() != nil || slices.Equal(ips, b.ep.Load().Bootstrap) {
			return
		}
		log.Printf("Bootstrap IPs changed for %s: %v", b.hostname, ips)
		b.ep.Store(&endpoint.DOHEndpoint{
			Hostname:  b.hostname,
			Path:      b.path,
			Bootstrap: ips,
		})
		testEndpoints(ctx, b.mgr, "bootstrap refresh")
	}()
}

func testEndpoints(ctx context.Context, mgr endpointTester, reason string) {
	if err := mgr.Test(ctx); err != nil {
		log.Printf("Endpoint test after %s failed: %v", reason, err)
	}
}
