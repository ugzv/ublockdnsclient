//go:build !linux && !darwin && !windows

package service

func resolveSystemDNS() systemDNSAssessment {
	return assessFromHostDNS()
}
