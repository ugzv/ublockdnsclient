package service

import (
	"errors"
	"strings"
	"testing"
)

func TestAssessFromPrimaryUsesPrimaryWhenAvailable(t *testing.T) {
	assessment := assessFromPrimary("test-source", func() ([]string, error) {
		return []string{"127.0.0.1", "8.8.8.8"}, nil
	})
	if !assessment.LocalDNS {
		t.Fatalf("expected local DNS, got %+v", assessment)
	}
	if len(assessment.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", assessment.Warnings)
	}
}

func TestAssessFromPrimaryFallsBackAndWarnsOnError(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		hostDNS: func() []string { return []string{"8.8.8.8"} },
	})

	assessment := assessFromPrimary("test-source", func() ([]string, error) {
		return nil, errors.New("command failed")
	})
	if assessment.LocalDNS {
		t.Fatalf("expected non-local fallback DNS, got %+v", assessment)
	}
	if len(assessment.Warnings) != 1 {
		t.Fatalf("expected one warning, got %+v", assessment.Warnings)
	}
	if !strings.Contains(assessment.Warnings[0], "test-source unavailable") {
		t.Fatalf("expected source warning, got %+v", assessment.Warnings)
	}
}

func TestAssessFromPrimaryFallsBackSilentlyOnEmpty(t *testing.T) {
	withStatusTestEnv(t, statusTestEnv{
		hostDNS: func() []string { return []string{"127.0.0.1"} },
	})

	assessment := assessFromPrimary("test-source", func() ([]string, error) {
		return nil, nil
	})
	if !assessment.LocalDNS {
		t.Fatalf("expected local fallback DNS, got %+v", assessment)
	}
	if len(assessment.Warnings) != 0 {
		t.Fatalf("expected no warnings for empty primary result, got %+v", assessment.Warnings)
	}
}
