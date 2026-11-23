package core

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateAPIServer refuses plain HTTP except to this machine: the account
// token is sent to the API server as a bearer header.
func ValidateAPIServer(apiServer string) error {
	u, err := url.Parse(apiServer)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid API server URL %q: expected absolute URL", apiServer)
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	ip := net.ParseIP(host)
	local := host == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme == "https" || (u.Scheme == "http" && local) {
		return nil
	}
	return fmt.Errorf("API server %q must use https", apiServer)
}
