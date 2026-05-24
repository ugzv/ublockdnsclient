//go:build windows

package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

var windowsServiceStateRE = regexp.MustCompile(`STATE\s*:\s*(\d+)`)

func resolveSystemDNS() systemDNSAssessment {
	return assessFromPrimary("Get-DnsClientServerAddress", dnsFromWindowsPowerShell)
}

func platformServiceState() (string, error) {
	out, err := commandCombinedOutputFunc("sc.exe", "query", core.ServiceName)
	text := string(out)
	if err != nil {
		if strings.Contains(text, "FAILED 1060") {
			return "not-installed", nil
		}
		return "unknown", fmt.Errorf("could not determine windows service state")
	}

	m := windowsServiceStateRE.FindStringSubmatch(text)
	if len(m) < 2 {
		return "unknown", fmt.Errorf("could not determine windows service state")
	}
	switch m[1] {
	case "1":
		return "stopped", nil
	case "4":
		return "running", nil
	default:
		return "unknown", nil
	}
}

func dnsFromWindowsPowerShell() ([]string, error) {
	out, err := commandOutputFunc(
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
