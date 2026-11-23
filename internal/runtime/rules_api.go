package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

// rulesHTTPClient is used for all rules API requests. It sets transport-level
// timeouts so that a half-open TCP connection (server crash without FIN) does
// not cause the SSE goroutine to hang indefinitely.
var rulesHTTPClient = &http.Client{
	// Requests carry the account token, and Go keeps it on a redirect to the
	// same host even when that redirect drops to plain HTTP.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return core.ValidateAPIServer(req.URL.String())
	},
	Transport: &http.Transport{
		// Plain HTTP is only ever this machine (see ValidateAPIServer), and a
		// proxy would carry the token off it.
		Proxy: func(req *http.Request) (*url.URL, error) {
			if req.URL.Scheme != "https" {
				return nil, nil
			}
			return http.ProxyFromEnvironment(req)
		},
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

type rulesVersionResponse struct {
	ProfileID      string `json:"profile_id"`
	AccountID      string `json:"account_id,omitempty"`
	RulesVersion   int64  `json:"rules_version"`
	RulesUpdatedAt int64  `json:"rules_updated_at"`
}

type rulesUpdateEvent struct {
	ProfileID          string `json:"profile_id"`
	AccountID          string `json:"account_id,omitempty"`
	RulesVersion       int64  `json:"rules_version"`
	RulesUpdatedAt     int64  `json:"rules_updated_at"`
	ListsChanged       bool   `json:"lists_changed"`
	CustomRulesChanged bool   `json:"custom_rules_changed"`
}

func fetchRulesVersion(ctx context.Context, apiServer, profileID, accountToken string) (rulesVersionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var out rulesVersionResponse
	resp, err := doRulesGET(ctx, apiServer, profileID, accountToken, "/rules/version", "application/json")
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func doRulesGET(ctx context.Context, apiServer, profileID, accountToken, suffix, accept string) (*http.Response, error) {
	u := strings.TrimRight(apiServer, "/") + "/api/profile/" + profileID + suffix
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accountToken)
	req.Header.Set("Accept", accept)
	if accept == "text/event-stream" {
		req.Header.Set("Cache-Control", "no-cache")
	}

	resp, err := rulesHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		endpoint := strings.TrimPrefix(suffix, "/")
		return nil, fmt.Errorf("%s status %d", endpoint, resp.StatusCode)
	}
	return resp, nil
}
