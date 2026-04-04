//go:build !windows

package service

func dnsFromWindowsPowerShell() ([]string, error) {
	return nil, nil
}

func windowsServiceState() (string, bool) {
	return "", false
}
