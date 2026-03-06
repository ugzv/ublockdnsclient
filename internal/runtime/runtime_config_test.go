package runtime

import (
	"errors"
	"testing"
)

func TestResolveAccountTokenIgnoresPersistedTokenLoadErrors(t *testing.T) {
	oldLoader := loadPersistedTokenFunc
	t.Cleanup(func() {
		loadPersistedTokenFunc = oldLoader
	})

	loadPersistedTokenFunc = func(profileID string) (string, error) {
		return "", errors.New("corrupt token file")
	}

	got, err := resolveAccountToken("profile123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty token fallback, got %q", got)
	}
}

func TestResolveAccountTokenPrefersExplicitSources(t *testing.T) {
	oldLoader := loadPersistedTokenFunc
	t.Cleanup(func() {
		loadPersistedTokenFunc = oldLoader
	})

	loadPersistedTokenFunc = func(profileID string) (string, error) {
		return "persisted-token", nil
	}

	got, err := resolveAccountToken("profile123", " flag-token ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "flag-token" {
		t.Fatalf("expected explicit token to win, got %q", got)
	}
}
