package runtime

import (
	"context"
	"errors"
	"net/http"
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
func TestAPIServerRequiresHTTPSOffLoopback(t *testing.T) {
	for server, wantOK := range map[string]bool{
		"https://ublockdns.com":    true,
		"http://localhost:8080":    true,
		"http://127.0.0.1:8080":    true,
		"http://[::1]:8080":        true,
		"http://LOCALHOST:8080":    true,
		"http://localhost.:8080":   true,
		"http://ublockdns.com":     false,
		"http://192.168.1.10:8080": false,
		"ftp://ublockdns.com":      false,
		"ublockdns.com":            false,
	} {
		if err := ValidateAPIServer(server); (err == nil) != wantOK {
			t.Errorf("ValidateAPIServer(%q) error = %v, want ok = %v", server, err, wantOK)
		}
	}
}

type recordingTransport struct {
	redirectTo string
	requested  []string
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.requested = append(r.requested, req.URL.String())
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{r.redirectTo}},
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

// Go keeps the Authorization header on a redirect to the same host, so an
// https API server redirecting to http would put the token on the wire.
func TestRulesRequestDoesNotFollowRedirectToPlainHTTP(t *testing.T) {
	rec := &recordingTransport{redirectTo: "http://ublockdns.example/api/profile/p/rules/version"}
	oldTransport := rulesHTTPClient.Transport
	rulesHTTPClient.Transport = rec
	t.Cleanup(func() { rulesHTTPClient.Transport = oldTransport })

	resp, err := doRulesGET(context.Background(), "https://ublockdns.example", "p", "token", "/rules/version", "application/json")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected the redirect to plain HTTP to be refused")
	}
	if len(rec.requested) != 1 {
		t.Fatalf("requests sent: %v, want only the original", rec.requested)
	}
}

// A service installed with a plain-HTTP API server must keep resolving DNS
// after an update, but must stop sending its token there.
func TestPlainHTTPAPIServerKeepsDaemonUpWithoutToken(t *testing.T) {
	cfg, err := resolveRuntimeConfig("profile1", "", "http://192.168.1.10:8080", "token")
	if err != nil {
		t.Fatalf("daemon config must still resolve: %v", err)
	}
	if cfg.AccountToken != "" {
		t.Fatal("token kept for a plain-HTTP API server")
	}
}
