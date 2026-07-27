package core

const (
	ServiceName      = "ublockdns"
	DefaultDoHServer = "https://my.ublockdns.com"
	DefaultAPIServer = "https://ublockdns.com"

	LocalDNSIP   = "127.0.0.1"
	LocalDNSAddr = LocalDNSIP + ":53"

	// ProbeDomain is resolved to check that a DNS path answers at all. It is
	// IANA-reserved, so no filter list blocks it and no provider branding leaks
	// into the lookup.
	ProbeDomain = "example.com"
)
