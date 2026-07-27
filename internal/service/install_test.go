package service

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForLocalDNSProxyRetriesUntilProxyBinds(t *testing.T) {
	var probes atomic.Int32
	now := time.Now()
	withStatusTestEnv(t, statusTestEnv{
		localDNSProbe: func() error {
			// The proxy binds asynchronously; the first probes hit a closed port.
			if probes.Add(1) < 4 {
				return errors.New("connection refused")
			}
			return nil
		},
		now:   func() time.Time { return now },
		sleep: func(d time.Duration) { now = now.Add(d) },
	})

	if err := waitForLocalDNSProxy(localDNSPreflightTimeout); err != nil {
		t.Fatalf("waitForLocalDNSProxy() error = %v, want success once the proxy binds", err)
	}
	if got := probes.Load(); got != 4 {
		t.Fatalf("probed %d times, want 4", got)
	}
}

func TestWaitForLocalDNSProxyGivesUpAfterTimeout(t *testing.T) {
	want := errors.New("connection refused")
	now := time.Now()
	withStatusTestEnv(t, statusTestEnv{
		localDNSProbe: func() error { return want },
		now:           func() time.Time { return now },
		sleep:         func(d time.Duration) { now = now.Add(d) },
	})

	err := waitForLocalDNSProxy(50 * time.Millisecond)
	if !errors.Is(err, want) {
		t.Fatalf("waitForLocalDNSProxy() error = %v, want %v", err, want)
	}
}
