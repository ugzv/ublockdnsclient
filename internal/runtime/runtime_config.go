package runtime

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ugzv/ublockdnsclient/internal/core"
	"github.com/ugzv/ublockdnsclient/internal/state"
)

type RuntimeConfig struct {
	ProfileID    string
	DoHServer    string
	APIServer    string
	AccountToken string
}

var loadPersistedTokenFunc = state.LoadPersistedToken

func resolveRuntimeConfig(profileID, overrideDoHServer, overrideAPIServer, accountToken string) (RuntimeConfig, error) {
	dohServer := ResolveDoHServer(overrideDoHServer)
	apiServer := ResolveAPIServer(overrideAPIServer, dohServer)
	token, err := resolveAccountToken(profileID, accountToken)
	if err != nil {
		return RuntimeConfig{}, err
	}
	return RuntimeConfig{
		ProfileID:    profileID,
		DoHServer:    dohServer,
		APIServer:    apiServer,
		AccountToken: token,
	}, nil
}

func resolveAccountToken(profileID, accountToken string) (string, error) {
	if token := strings.TrimSpace(accountToken); token != "" {
		return token, nil
	}
	if token := strings.TrimSpace(os.Getenv("UBLOCKDNS_ACCOUNT_TOKEN")); token != "" {
		return token, nil
	}
	persisted, err := loadPersistedTokenFunc(profileID)
	if err != nil {
		// Preserve pre-refactor behavior: token load failures should only
		// disable the rules stream, not block runtime startup entirely.
		return "", nil
	}
	return persisted, nil
}

func ResolveDoHServer(overrideServer string) string {
	if overrideServer = strings.TrimSpace(overrideServer); overrideServer != "" {
		return strings.TrimRight(overrideServer, "/")
	}
	if fromEnv := strings.TrimSpace(os.Getenv("UBLOCKDNS_DOH_SERVER")); fromEnv != "" {
		return strings.TrimRight(fromEnv, "/")
	}
	return core.DefaultDoHServer
}

func ResolveAPIServer(overrideServer, _ string) string {
	if overrideServer = strings.TrimSpace(overrideServer); overrideServer != "" {
		return strings.TrimRight(overrideServer, "/")
	}
	if fromEnv := strings.TrimSpace(os.Getenv("UBLOCKDNS_API_SERVER")); fromEnv != "" {
		return strings.TrimRight(fromEnv, "/")
	}
	return core.DefaultAPIServer
}

func BuildDoHTarget(base, profileID string) (string, string, string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid DoH server URL %q: %w", base, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", "", fmt.Errorf("invalid DoH server URL %q: expected absolute URL", base)
	}
	if err := core.ValidateProfileID(profileID); err != nil {
		return "", "", "", err
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(profileID)
	return u.String(), u.Hostname(), u.Path, nil
}
