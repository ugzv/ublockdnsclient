package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type rulesTransportFunc func(*http.Request) (*http.Response, error)

func (f rulesTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type unreadableRulesBody struct {
	testing *testing.T
	closed  bool
}

func (b *unreadableRulesBody) Read([]byte) (int, error) {
	b.testing.Error("non-200 body must not be read; it may never finish")
	return 0, io.EOF
}
func (b *unreadableRulesBody) Close() error { b.closed = true; return nil }

func TestFetchRulesVersionHasDeadline(t *testing.T) {
	original := rulesHTTPClient
	t.Cleanup(func() { rulesHTTPClient = original })
	rulesHTTPClient = &http.Client{Transport: rulesTransportFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) > 30*time.Second {
			t.Error("version request needs a deadline of at most 30 seconds")
		}
		if req.Header.Get("Authorization") != "Bearer token" || req.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected headers: %v", req.Header)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"rules_version":12}`)), Header: make(http.Header)}, nil
	})}
	result, err := fetchRulesVersion(context.Background(), "https://example.invalid", "profile", "token")
	if err != nil || result.RulesVersion != 12 {
		t.Fatalf("fetchRulesVersion() = %v, %v", result, err)
	}
}

func TestRulesStreamHasNoDeadlineAndClosesErrorBody(t *testing.T) {
	original := rulesHTTPClient
	t.Cleanup(func() { rulesHTTPClient = original })
	body := &unreadableRulesBody{testing: t}
	rulesHTTPClient = &http.Client{Transport: rulesTransportFunc(func(req *http.Request) (*http.Response, error) {
		if _, ok := req.Context().Deadline(); ok {
			t.Error("SSE must remain long-lived")
		}
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: body, Header: make(http.Header)}, nil
	})}
	_, err := doRulesGET(context.Background(), "https://example.invalid", "profile", "token", "/rules/stream", "text/event-stream")
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("error = %v", err)
	}
	if !body.closed {
		t.Error("error response body was not closed")
	}
}

func TestFetchRulesVersionCancelsStalledBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := fetchRulesVersion(ctx, server.URL, "profile", "token")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled response error = %v, want deadline exceeded", err)
	}
}
