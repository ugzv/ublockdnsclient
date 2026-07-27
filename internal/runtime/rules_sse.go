package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"
)

// sseReadTimeout is how long we wait for any data on the SSE stream before
// treating the connection as stalled. The server should send a keepalive or
// event well within this window.
const sseReadTimeout = 5 * time.Minute

// idleReader enforces an inactivity deadline on the SSE stream. On expiry it
// closes the underlying body, so the in-progress Read fails on its own and the
// scanner loop exits into a reconnect. Racing a goroutine against the Read
// instead would leave that goroutine writing into a buffer the caller has
// already taken back.
type idleReader struct {
	body    io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	expired atomic.Bool
}

func newIdleReader(body io.ReadCloser, timeout time.Duration) *idleReader {
	r := &idleReader{body: body, timeout: timeout}
	r.timer = time.AfterFunc(timeout, func() {
		r.expired.Store(true)
		_ = body.Close()
	})
	return r
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.timer.Reset(r.timeout)
	}
	if err != nil && r.expired.Load() {
		// Report why the body closed; the raw error is "use of closed
		// network connection", which says nothing about the stall.
		return n, fmt.Errorf("SSE stream idle for %v", r.timeout)
	}
	return n, err
}

func (r *idleReader) Close() error {
	r.timer.Stop()
	return r.body.Close()
}

func consumeRulesStream(ctx context.Context, apiServer, profileID, accountToken string, onEvent func(ev rulesUpdateEvent)) error {
	resp, err := doRulesGET(ctx, apiServer, profileID, accountToken, "/rules/stream", "text/event-stream")
	if err != nil {
		return err
	}
	body := newIdleReader(resp.Body, sseReadTimeout)
	defer func() { _ = body.Close() }()

	scanner := bufio.NewScanner(body)
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
