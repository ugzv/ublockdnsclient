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

// The account token goes to the API server as a bearer header, so a plain
// HTTP server anywhere but this machine would expose it on the network.
func TestValidateAPIServerRequiresHTTPSOffLoopback(t *testing.T) {
	for server, wantOK := range map[string]bool{
		"https://ublockdns.com":    true,
		"http://localhost:8080":    true,
		"http://127.0.0.1:8080":    true,
		"http://[::1]:8080":        true,
		"http://ublockdns.com":     false,
		"http://192.168.1.10:8080": false,
		"ftp://ublockdns.com":      false,
		"ublockdns.com":            false,
	} {
		if err := validateAPIServer(server); (err == nil) != wantOK {
			t.Errorf("validateAPIServer(%q) error = %v, want ok = %v", server, err, wantOK)
		}
	}
}
