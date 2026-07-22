//go:build linux

package core

func activatePlatformSystemDNS() error {
	return ConfigureLinuxSystemDNS()
}
