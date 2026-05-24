//go:build linux

package service

import (
	"bufio"
	"bytes"
	"net"
	"regexp"
	"strings"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

var resolvConfNameserverRE = regexp.MustCompile(`(?m)^\s*nameserver\s+([^\s#]+)`)

func resolveSystemDNS() systemDNSAssessment {
	resolvedDNS, resolvectlErr := dnsFromResolvectl()
	resolvConfDNS, resolvConfErr := dnsFromResolvConf()
	hostDNS := core.CollectUniqueNonEmpty(hostDNSFunc())

	primary := pickLinuxPrimaryDNS(resolvedDNS, resolvConfDNS, hostDNS)
	assessment := newDNSAssessment(primary)
	assessment.Warnings = appendSourceWarnings(assessment.Warnings, resolvectlErr, resolvConfErr)

	if len(primary) == 0 {
		return assessment
	}

	if assessment.LocalDNS && len(hostDNS) > 0 && !core.HasDNS127001(hostDNS) {
		assessment.Warnings = append(assessment.Warnings,
			fmtResolverDisagreement("active resolver appears local, but NetworkManager/lease metadata still reports upstream DNS", hostDNS))
	}
	if len(resolvedDNS) > 0 && len(resolvConfDNS) > 0 && !sameDNSSet(resolvedDNS, resolvConfDNS) {
		assessment.Warnings = append(assessment.Warnings,
			fmtResolverDisagreement("resolvectl and /etc/resolv.conf disagree", resolvConfDNS))
	}

	return assessment
}

func appendSourceWarnings(warnings []string, resolvectlErr, resolvConfErr error) []string {
	if resolvectlErr != nil {
		warnings = append(warnings, sourceUnavailableWarning("resolvectl", resolvectlErr))
	}
	if resolvConfErr != nil {
		warnings = append(warnings, sourceUnavailableWarning("/etc/resolv.conf", resolvConfErr))
	}
	return warnings
}

func pickLinuxPrimaryDNS(resolvedDNS, resolvConfDNS, hostDNS []string) []string {
	switch {
	case core.HasDNS127001(resolvedDNS):
		return resolvedDNS
	case core.HasDNS127001(resolvConfDNS):
		return resolvConfDNS
	case len(resolvedDNS) > 0:
		return resolvedDNS
	case len(resolvConfDNS) > 0:
		return resolvConfDNS
	default:
		return hostDNS
	}
}

func sameDNSSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
		if seen[v] < 0 {
			return false
		}
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func fmtResolverDisagreement(prefix string, dns []string) string {
	if len(dns) == 0 {
		return prefix
	}
	return prefix + ": " + strings.Join(dns, ", ")
}

func dnsFromResolvectl() ([]string, error) {
	out, err := commandOutputFunc("resolvectl", "dns")
	if err != nil {
		return nil, err
	}

	var raw []string
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := s.Text()
		if idx := strings.Index(line, ":"); idx >= 0 {
			line = line[idx+1:]
		}
		for _, field := range strings.Fields(line) {
			if ip := cleanIPToken(field); ip != "" {
				raw = append(raw, ip)
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return core.CollectUniqueNonEmpty(raw), nil
}

func dnsFromResolvConf() ([]string, error) {
	body, err := readFileFunc("/etc/resolv.conf")
	if err != nil {
		return nil, err
	}

	matches := resolvConfNameserverRE.FindAllStringSubmatch(string(body), -1)
	raw := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		if ip := cleanIPToken(m[1]); ip != "" {
			raw = append(raw, ip)
		}
	}
	return core.CollectUniqueNonEmpty(raw), nil
}

func cleanIPToken(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "[]")
	if i := strings.Index(raw, "%"); i >= 0 {
		raw = raw[:i]
	}
	if net.ParseIP(raw) == nil {
		return ""
	}
	return raw
}
