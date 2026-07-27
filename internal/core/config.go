package core

const (
	ServiceName      = "ublockdns"
	DefaultDoHServer = "https://my.ublockdns.com"
	DefaultAPIServer = "https://ublockdns.com"

	// LocalDNSAddress is what system DNS is pointed at; LocalDNSAddr is where
	// the proxy listens for those queries.
	LocalDNSAddress = "127.0.0.1"
	LocalDNSAddr    = LocalDNSAddress + ":53"

	// ProbeDomain is resolved to check that a DNS path answers at all. It is
	// IANA-reserved, so no filter list blocks it and no provider branding leaks
	// into the lookup.
	ProbeDomain = "example.com"
)
