package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

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

func watchRulesUpdates(ctx context.Context, apiServer, profileID, accountToken string) {
	token := strings.TrimSpace(accountToken)
	if token == "" {
		return
	}
	log.Printf("Rules update stream enabled for profile %s", profileID)

	currentVersion := int64(0)
	if v, err := fetchRulesVersion(ctx, apiServer, profileID, token); err == nil {
		currentVersion = v.RulesVersion
	}

	backoff := time.Second
	for {
		if err := consumeRulesStream(ctx, apiServer, profileID, token, func(ev rulesUpdateEvent) {
			if ev.RulesVersion <= currentVersion {
				return
			}
			currentVersion = ev.RulesVersion
			if err := flushDNSCaches(); err != nil {
				log.Printf("Rules updated (v%d) but DNS cache flush had issues: %v", currentVersion, err)
				return
			}
			log.Printf("Rules updated (v%d), local DNS cache flushed", currentVersion)
		}); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Rules stream disconnected: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Reconcile state after disconnect so missed events are still applied.
		if v, err := fetchRulesVersion(ctx, apiServer, profileID, token); err == nil && v.RulesVersion > currentVersion {
			currentVersion = v.RulesVersion
			if err := flushDNSCaches(); err != nil {
				log.Printf("Rules version advanced to v%d; DNS cache flush had issues: %v", currentVersion, err)
			} else {
				log.Printf("Rules version advanced to v%d, local DNS cache flushed", currentVersion)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func fetchRulesVersion(ctx context.Context, apiServer, profileID, accountToken string) (rulesVersionResponse, error) {
	var out rulesVersionResponse
	resp, err := doRulesGET(ctx, apiServer, profileID, accountToken, "/rules/version", "application/json")
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func consumeRulesStream(ctx context.Context, apiServer, profileID, accountToken string, onEvent func(ev rulesUpdateEvent)) error {
	resp, err := doRulesGET(ctx, apiServer, profileID, accountToken, "/rules/stream", "text/event-stream")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var dataLines []string

	emit := func() {
		if len(dataLines) == 0 {
			return
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]

		var ev rulesUpdateEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			return
		}
		onEvent(ev)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			emit()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	emit()

	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		endpoint := strings.TrimPrefix(suffix, "/")
		return nil, fmt.Errorf("%s status %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func flushDNSCaches() error {
	type cmdSpec struct {
		name string
		args []string
	}

	var commands []cmdSpec
	switch runtime.GOOS {
	case "darwin":
		commands = []cmdSpec{
			{name: "dscacheutil", args: []string{"-flushcache"}},
			{name: "killall", args: []string{"-HUP", "mDNSResponder"}},
		}
	case "linux":
		commands = []cmdSpec{
			{name: "resolvectl", args: []string{"flush-caches"}},
			{name: "systemd-resolve", args: []string{"--flush-caches"}},
			{name: "service", args: []string{"nscd", "restart"}},
		}
	case "windows":
		commands = []cmdSpec{
			{name: "ipconfig", args: []string{"/flushdns"}},
		}
	default:
		return nil
	}

	var errs []string
	success := false
	for _, c := range commands {
		cmd := exec.Command(c.name, c.args...)
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Sprintf("%s %s: %v", c.name, strings.Join(c.args, " "), err))
			continue
		}
		success = true
	}
	if success {
		return nil
	}
	if len(errs) == 0 {
		return errors.New("no cache flush command available")
	}
	return errors.New(strings.Join(errs, "; "))
}

func tokenPath(profileID string) (string, error) {
	if err := ValidateProfileID(profileID); err != nil {
		return "", err
	}
	return filepath.Join(tokenDir(), profileID+".token"), nil
}

func persistToken(profileID, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	if err := os.MkdirAll(tokenDir(), 0o700); err != nil {
		return err
	}
	p, err := tokenPath(profileID)
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(strings.TrimSpace(token)), 0o600)
}

func loadPersistedToken(profileID string) (string, error) {
	p, err := tokenPath(profileID)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", errors.New("empty persisted token")
	}
	return token, nil
}

func clearPersistedTokens() error {
	baseDir := tokenDir()
	matches, err := filepath.Glob(filepath.Join(baseDir, "*.token"))
	if err != nil {
		return err
	}
	for _, p := range matches {
		_ = os.Remove(p)
	}
	_ = os.Remove(baseDir)
	return nil
}

func tokenDir() string {
	if runtime.GOOS == "windows" {
		if programData := strings.TrimSpace(os.Getenv("ProgramData")); programData != "" {
			return filepath.Join(programData, "ublockdns")
		}
		return filepath.Join(`C:\ProgramData`, "ublockdns")
	}
	return "/etc/ublockdns"
}
