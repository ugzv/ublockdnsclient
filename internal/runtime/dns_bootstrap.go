package runtime

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/nextdns/nextdns/host"
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
				out = core.AppendUniqueString(out, seen, addr)
			}
		}
	}

	// Fallback for locked-down networks that block direct DNS to public resolvers.
	// Only use system DNS if it's not already pointed to this local proxy.
	if len(out) == 0 && !core.HasDNS127001(host.DNS()) {
		addrs, err := lookupHostSystem(hostname)
		if err != nil {
			errs = append(errs, fmt.Sprintf("system resolver: %v", err))
		} else {
			for _, addr := range addrs {
				out = core.AppendUniqueString(out, seen, addr)
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
