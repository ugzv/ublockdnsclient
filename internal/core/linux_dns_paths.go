//go:build linux

package core

const (
	managedResolvMarker = "Managed by uBlockDNS"
	managedConfigMarker = "# uBlockDNS"
)

type linuxDNSPaths struct {
	ResolvConf         string
	ResolvUBlockDNSBak string
	ResolvNextDNSBak   string
	ResolvedDropIn     string
	NMUBlockDNSConf    string
	NMNextDNSConf      string
	ResolvconfHead     string
	ResolvconfHeadBak  string
	ConnmanMainConf    string
	ConnmanMainConfBak string
	DhclientConfs      []string
}

func defaultLinuxDNSPaths() linuxDNSPaths {
	return linuxDNSPaths{
		ResolvConf:         "/etc/resolv.conf",
		ResolvUBlockDNSBak: "/etc/resolv.conf.ublockdns.bak",
		ResolvNextDNSBak:   "/etc/resolv.conf.nextdns-bak",
		ResolvedDropIn:     "/etc/systemd/resolved.conf.d/ublockdns.conf",
		NMUBlockDNSConf:    "/etc/NetworkManager/conf.d/ublockdns.conf",
		NMNextDNSConf:      "/etc/NetworkManager/conf.d/nextdns.conf",
		ResolvconfHead:     "/etc/resolvconf/resolv.conf.d/head",
		ResolvconfHeadBak:  "/etc/resolvconf/resolv.conf.d/head.ublockdns.bak",
		ConnmanMainConf:    "/etc/connman/main.conf",
		ConnmanMainConfBak: "/etc/connman/main.conf.ublockdns.bak",
		DhclientConfs:      []string{"/etc/dhcp/dhclient.conf", "/etc/dhclient.conf"},
	}
}

var linuxDNSPathsVar = defaultLinuxDNSPaths()

func linuxDNSPathsActive() linuxDNSPaths {
	return linuxDNSPathsVar
}

// SwapLinuxDNSPaths overrides Linux DNS paths for tests and returns a restore func.
func SwapLinuxDNSPaths(paths linuxDNSPaths) func() {
	old := linuxDNSPathsVar
	linuxDNSPathsVar = paths
	return func() {
		linuxDNSPathsVar = old
	}
}
