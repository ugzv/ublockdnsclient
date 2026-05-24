//go:build !linux

package core

func activatePlatformSystemDNS() error {
	return ActivateSystemDNS()
}

func ConfigureLinuxSystemDNS() error {
	return nil
}

func PrepareLinuxSystemDNSForInstall() error {
	return nil
}

func restorePlatformInstallArtifacts() error {
	return nil
}
