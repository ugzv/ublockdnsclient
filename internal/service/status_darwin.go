//go:build darwin

package service

import (
	"regexp"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

func dnsFromScutil() ([]string, error) {
	out, err := core.CommandOutput("scutil", "--dns")
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`nameserver\[[0-9]+\]\s*:\s*([^\s]+)`)
	matches := re.FindAllStringSubmatch(string(out), -1)
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
