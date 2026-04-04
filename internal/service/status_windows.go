//go:build windows

package service

import (
	"regexp"
	"strings"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

func dnsFromWindowsPowerShell() ([]string, error) {
	out, err := core.CommandOutput(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		`Get-DnsClientServerAddress -AddressFamily IPv4 | ForEach-Object { $_.ServerAddresses } | Where-Object { $_ }`,
	)
	if err != nil {
		return nil, err
	}
	return core.CollectUniqueNonEmpty(strings.Split(string(out), "\n")), nil
}

func windowsServiceState() (string, bool) {
	out, err := core.CommandCombinedOutput("sc.exe", "query", core.ServiceName)
	text := string(out)
	if err != nil {
		if strings.Contains(text, "FAILED 1060") {
			return "not-installed", true
		}
		return "unknown", false
	}

	m := regexp.MustCompile(`STATE\s*:\s*(\d+)`).FindStringSubmatch(text)
	if len(m) < 2 {
		return "unknown", false
	}
	switch m[1] {
	case "1":
		return "stopped", true
	case "4":
		return "running", true
	default:
		return "unknown", true
	}
}
