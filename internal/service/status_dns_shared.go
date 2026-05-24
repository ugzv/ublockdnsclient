package service

import (
	"fmt"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

type systemDNSAssessment struct {
	DNS      []string
	LocalDNS bool
	Warnings []string
}

func newDNSAssessment(dns []string) systemDNSAssessment {
	return systemDNSAssessment{
		DNS:      dns,
		LocalDNS: core.HasDNS127001(dns),
	}
}

func sourceUnavailableWarning(source string, err error) string {
	return fmt.Sprintf("%s unavailable: %v", source, err)
}

func assessFromPrimary(source string, primary func() ([]string, error)) systemDNSAssessment {
	dns, err := primary()
	if err == nil && len(dns) > 0 {
		return newDNSAssessment(dns)
	}

	assessment := assessFromHostDNS()
	if err != nil {
		assessment.Warnings = append(assessment.Warnings, sourceUnavailableWarning(source, err))
	}
	return assessment
}

func assessFromHostDNS() systemDNSAssessment {
	return newDNSAssessment(core.CollectUniqueNonEmpty(hostDNSFunc()))
}
