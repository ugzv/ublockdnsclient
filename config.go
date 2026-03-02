package main

var version = "dev"

const (
	serviceName      = "ublockdns"
	defaultDoHServer = "https://my.ublockdns.com"
	defaultAPIServer = "https://ublockdns.com"
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
