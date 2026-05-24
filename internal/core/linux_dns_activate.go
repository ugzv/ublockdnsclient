//go:build linux

package core

import "log"

func activatePlatformSystemDNS() error {
	if err := PrepareLinuxSystemDNSForInstall(); err != nil {
		log.Printf("Warning: failed to prepare Linux system DNS for install: %v", err)
	}
	return ConfigureLinuxSystemDNS()
}
