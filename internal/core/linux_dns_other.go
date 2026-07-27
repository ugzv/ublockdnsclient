//go:build !linux

package core

func activatePlatformSystemDNS() error {
	return activateSystemDNS()
}

func ConfigureLinuxSystemDNS() error {
	return nil
}

func restorePlatformInstallArtifacts() error {
	return nil
}
