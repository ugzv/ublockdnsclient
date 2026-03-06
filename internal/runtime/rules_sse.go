package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

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
