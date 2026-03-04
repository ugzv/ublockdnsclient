package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rulesHTTPClient is used for all rules API requests. It sets transport-level
// timeouts so that a half-open TCP connection (server crash without FIN) does
// not cause the SSE goroutine to hang indefinitely.
var rulesHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
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

func watchRulesUpdates(ctx context.Context, apiServer, profileID, accountToken string, onRulesUpdated func()) {
	token := strings.TrimSpace(accountToken)
	if token == "" {
		return
	}
	log.Printf("Rules update stream enabled for profile %s", profileID)

	currentVersion := int64(0)
	if v, err := fetchRulesVersion(ctx, apiServer, profileID, token); err == nil {
		currentVersion = v.RulesVersion
	}

	// flushDebounce coalesces rapid rule updates into a single OS cache flush.
	// A generation counter ensures that if a timer fires concurrently with a
	// new schedule call, the stale callback is a no-op.
	// maxFlushDelay caps how long continuous events can postpone a flush.
	const maxFlushDelay = 10 * time.Second
	var flushMu sync.Mutex
	var flushTimer *time.Timer
	var flushGen uint64
	var firstPending time.Time
	doFlush := func(version int64) {
		if onRulesUpdated != nil {
			onRulesUpdated()
		}
		if err := flushDNSCaches(); err != nil {
			log.Printf("Rules updated (v%d) but DNS cache flush had issues: %v", version, err)
			return
		}
		log.Printf("Rules updated (v%d), local DNS cache flushed", version)
	}
	scheduleFlush := func(version int64) {
		flushMu.Lock()
		defer flushMu.Unlock()
		now := time.Now()
		if flushTimer != nil {
			flushTimer.Stop()
		} else {
			firstPending = now
		}
		// If events have been arriving continuously for too long, flush now.
		if now.Sub(firstPending) >= maxFlushDelay {
			flushTimer = nil
			go doFlush(version)
			return
		}
		flushGen++
		gen := flushGen
		flushTimer = time.AfterFunc(2*time.Second, func() {
			flushMu.Lock()
			flushTimer = nil
			if gen != flushGen {
				flushMu.Unlock()
				return
			}
			flushMu.Unlock()
			doFlush(version)
		})
	}

	backoff := time.Second
	for {
		streamStart := time.Now()
		if err := consumeRulesStream(ctx, apiServer, profileID, token, func(ev rulesUpdateEvent) {
			if ev.RulesVersion <= currentVersion {
				return
			}
			currentVersion = ev.RulesVersion
			scheduleFlush(currentVersion)
		}); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Rules stream disconnected: %v", err)
		}

		// Reset backoff if the stream was healthy for a meaningful duration.
		if time.Since(streamStart) > 30*time.Second {
			backoff = time.Second
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Reconcile state after disconnect so missed events are still applied.
		if v, err := fetchRulesVersion(ctx, apiServer, profileID, token); err == nil && v.RulesVersion > currentVersion {
			currentVersion = v.RulesVersion
			scheduleFlush(currentVersion)
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

// sseReadTimeout is how long we wait for any data on the SSE stream before
// treating the connection as stalled. The server should send a keepalive or
// event well within this window.
const sseReadTimeout = 5 * time.Minute

// timeoutReader wraps an io.Reader and enforces a per-Read deadline via a
// timer. If no data arrives within the timeout the Read returns an error,
// which causes the SSE scanner loop to exit and trigger a reconnect.
type timeoutReader struct {
	r       io.Reader
	timeout time.Duration
	timer   *time.Timer
}

func newTimeoutReader(r io.Reader, timeout time.Duration) *timeoutReader {
	return &timeoutReader{r: r, timeout: timeout, timer: time.NewTimer(timeout)}
}

func (tr *timeoutReader) Read(p []byte) (int, error) {
	// Reset the deadline for each read attempt.
	tr.timer.Reset(tr.timeout)
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		n, err := tr.r.Read(p)
		ch <- result{n, err}
	}()
	select {
	case res := <-ch:
		tr.timer.Stop()
		return res.n, res.err
	case <-tr.timer.C:
		return 0, fmt.Errorf("SSE stream idle for %v", tr.timeout)
	}
}

func consumeRulesStream(ctx context.Context, apiServer, profileID, accountToken string, onEvent func(ev rulesUpdateEvent)) error {
	resp, err := doRulesGET(ctx, apiServer, profileID, accountToken, "/rules/stream", "text/event-stream")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(newTimeoutReader(resp.Body, sseReadTimeout))
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

	resp, err := rulesHTTPClient.Do(req)
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
