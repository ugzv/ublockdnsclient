package runtime

import (
	"context"
	"testing"

	"github.com/nextdns/nextdns/resolver/endpoint"
)

type recordingEndpoint struct {
	lastQuery []byte
}

func (e *recordingEndpoint) String() string { return "recording" }

func (e *recordingEndpoint) Protocol() endpoint.Protocol { return endpoint.ProtocolDOH }

func (e *recordingEndpoint) Equal(other endpoint.Endpoint) bool { return e == other }

func (e *recordingEndpoint) Exchange(_ context.Context, payload, buf []byte) (int, error) {
	e.lastQuery = append([]byte(nil), payload...)
	copy(buf, payload)
	return len(payload), nil
}

func TestEndpointTesterOverridesProbeDomain(t *testing.T) {
	e := &recordingEndpoint{}
	mgr := newEndpointManager(e)

	tester := mgr.EndpointTester(e)
	if tester == nil {
		t.Fatal("expected DoH endpoint tester")
	}
	if err := tester(context.Background(), "probe-test.dns.nextdns.io."); err != nil {
		t.Fatalf("tester returned error: %v", err)
	}
	if len(e.lastQuery) == 0 {
		t.Fatal("expected query to be recorded")
	}

	gotHost := parsedQueryName(t, e.lastQuery)
	if gotHost != dohProbeDomain {
		t.Fatalf("expected probe domain %q, got %q", dohProbeDomain, gotHost)
	}
}

func parsedQueryName(t *testing.T, payload []byte) string {
	t.Helper()
	if len(payload) < 17 {
		t.Fatalf("query too short: %d", len(payload))
	}
	i := 12
	out := ""
	for {
		if i >= len(payload) {
			t.Fatal("unterminated qname")
		}
		labelLen := int(payload[i])
		i++
		if labelLen == 0 {
			return out
		}
		if i+labelLen > len(payload) {
			t.Fatal("label exceeds payload")
		}
		if out != "" {
			out += "."
		}
		out += string(payload[i : i+labelLen])
		i += labelLen
	}
}
