package runtime

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBootstrapReturnsFirstAnswerAndCancelsOtherLookups(t *testing.T) {
	var running sync.WaitGroup
	var calls atomic.Int32
	allStarted := make(chan struct{})
	lookup := bootstrapLookup{
		via: func(ctx context.Context, server, transport, hostname string) ([]string, error) {
			running.Add(1)
			defer running.Done()
			if calls.Add(1) == int32(len(bootstrapResolvers)*2) {
				close(allStarted)
			}
			if server == bootstrapResolvers[1] && transport == "tcp" {
				<-allStarted
				return []string{"203.0.113.1", "203.0.113.1", "2001:db8::1"}, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
		systemDNS: func(context.Context) []string { return []string{"127.0.0.1"} },
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ips, err := lookup.resolve(ctx, "example.org")
	if err != nil || !slices.Equal(ips, []string{"203.0.113.1", "2001:db8::1"}) {
		t.Fatalf("resolve = %v, %v", ips, err)
	}
	done := make(chan struct{})
	go func() { running.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("losing lookups were not canceled")
	}
}

func TestBootstrapHonorsCallerCancellation(t *testing.T) {
	started := make(chan struct{}, len(bootstrapResolvers)*2)
	lookup := bootstrapLookup{
		via: func(ctx context.Context, _, _, _ string) ([]string, error) {
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		},
		systemDNS: func(context.Context) []string { return []string{"127.0.0.1"} },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := lookup.resolve(ctx, "example.org"); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("bootstrap ignored caller cancellation")
	}
}

func TestBootstrapSystemFallbackAvoidsLocalDNS(t *testing.T) {
	for _, dns := range []string{"127.0.0.1", "::1", "192.0.2.53"} {
		t.Run(dns, func(t *testing.T) {
			var calls atomic.Int32
			lookup := bootstrapLookup{
				via: func(_ context.Context, server, _, _ string) ([]string, error) {
					if server == "192.0.2.53:53" {
						calls.Add(1)
						return []string{"203.0.113.2"}, nil
					}
					return nil, errors.New("public DNS unavailable")
				},
				systemDNS: func(context.Context) []string { return []string{dns} },
			}
			ips, err := lookup.resolve(context.Background(), "example.org")
			if net.ParseIP(dns).IsLoopback() {
				if err == nil || calls.Load() != 0 {
					t.Fatalf("local DNS fallback: ips=%v err=%v calls=%d", ips, err, calls.Load())
				}
			} else if err != nil || calls.Load() == 0 || !slices.Equal(ips, []string{"203.0.113.2"}) {
				t.Fatalf("system fallback: ips=%v err=%v calls=%d", ips, err, calls.Load())
			}
		})
	}
}

func TestBootstrapSharesDeadlineWithSystemFallback(t *testing.T) {
	var systemCalled atomic.Bool
	lookup := bootstrapLookup{
		via: func(ctx context.Context, server, _, _ string) ([]string, error) {
			if server == "192.0.2.53:53" {
				systemCalled.Store(true)
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
		systemDNS: func(context.Context) []string { return []string{"192.0.2.53"} },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := lookup.resolve(ctx, "example.org")
	if !errors.Is(err, context.DeadlineExceeded) || !systemCalled.Load() {
		t.Fatalf("error = %v, system called = %v", err, systemCalled.Load())
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("deadline exceeded: %s", elapsed)
	}
}

func TestDiscoverDNSServersCancelsPlatformCommand(t *testing.T) {
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan []string, 1)
	go func() {
		done <- discoverDNSServersWith(ctx, "darwin", func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}, nil)
	}()
	<-started
	cancel()
	select {
	case servers := <-done:
		if len(servers) != 0 {
			t.Fatalf("servers = %v, want none after cancellation", servers)
		}
	case <-time.After(time.Second):
		t.Fatal("discovery did not propagate cancellation")
	}
}

func TestDiscoverDNSServersFiltersAndDeduplicates(t *testing.T) {
	for _, platform := range []string{"darwin", "linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			servers := discoverDNSServersWith(context.Background(), platform, func(_ context.Context, name string, args ...string) ([]byte, error) {
				if platform == "windows" {
					if name != "powershell" || !slices.Equal(args, []string{"-NoProfile", "-NonInteractive", "-Command", `Get-DnsClientServerAddress | ForEach-Object { $_.ServerAddresses } | Where-Object { $_ }`}) {
						t.Fatalf("unexpected Windows discovery command: %s %v", name, args)
					}
					return []byte("127.0.0.1\r\n192.0.2.53\r\n2001:db8::53\r\n192.0.2.53\r\n::1\r\n0.0.0.0\r\ninvalid\r\n"), nil
				}
				if platform == "darwin" {
					return []byte("127.0.0.1 192.0.2.53 2001:db8::53 192.0.2.53 ::1 0.0.0.0 invalid"), nil
				}
				return []byte("IP4.DNS[1]: 192.0.2.53\nIP6.DNS[1]: 2001:db8::53\nIP4.ADDRESS[1]: 192.0.2.10"), nil
			}, func(string) ([]byte, error) {
				return []byte("nameserver 127.0.0.1\nnameserver 192.0.2.53\n# nameserver 203.0.113.1\nnameserver ::1"), nil
			})
			if !slices.Equal(servers, []string{"192.0.2.53", "2001:db8::53"}) {
				t.Fatalf("servers = %v", servers)
			}
		})
	}
}

func TestWindowsDNSDiscoveryPreservesParentDeadline(t *testing.T) {
	for _, budget := range []time.Duration{250 * time.Millisecond, 10 * time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			called := false
			discoverDNSServersWith(ctx, "windows", func(commandCtx context.Context, _ string, _ ...string) ([]byte, error) {
				called = true
				deadline, ok := commandCtx.Deadline()
				remaining := time.Until(deadline)
				want := min(budget, 3*time.Second)
				if !ok || remaining > want || remaining < want-100*time.Millisecond {
					t.Fatalf("command deadline remaining = %v, want near %v", remaining, want)
				}
				return nil, nil
			}, nil)
			if !called {
				t.Fatal("Windows discovery command was not called")
			}
		})
	}
}
