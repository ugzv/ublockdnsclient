//go:build darwin

package service

import (
	"regexp"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

var scutilNameserverRE = regexp.MustCompile(`nameserver\[[0-9]+\]\s*:\s*([^\s]+)`)

func resolveSystemDNS() systemDNSAssessment {
	return assessFromPrimary("scutil", dnsFromScutil)
}

func dnsFromScutil() ([]string, error) {
	out, err := commandOutputFunc("scutil", "--dns")
	if err != nil {
		return nil, err
	}
	matches := scutilNameserverRE.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return nil, nil
	}
	raw := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		raw = append(raw, m[1])
	}
	return core.CollectUniqueNonEmpty(raw), nil
}
