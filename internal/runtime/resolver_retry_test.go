package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/query"
)

type contextResolver func(context.Context, query.Query, []byte) (int, resolver.ResolveInfo, error)

func (f contextResolver) Resolve(ctx context.Context, q query.Query, buf []byte) (int, resolver.ResolveInfo, error) {
	return f(ctx, q, buf)
}

func TestRetryResolverReservesTimeAfterStalledAttempt(t *testing.T) {
	calls := 0
	inner := contextResolver(func(ctx context.Context, _ query.Query, _ []byte) (int, resolver.ResolveInfo, error) {
		calls++
		if calls == 1 {
			<-ctx.Done()
			return 0, resolver.ResolveInfo{}, ctx.Err()
		}
		return 42, resolver.ResolveInfo{}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	n, _, err := (retryResolver{inner: inner}).Resolve(ctx, query.Query{}, nil)
	if err != nil || n != 42 || calls != 2 {
		t.Fatalf("n=%d calls=%d err=%v; stalled attempt consumed retry budget", n, calls, err)
	}
}

func TestRetryResolverPreservesStaleAnswerAcrossFallbackFailure(t *testing.T) {
	calls := 0
	inner := contextResolver(func(_ context.Context, _ query.Query, buf []byte) (int, resolver.ResolveInfo, error) {
		calls++
		if calls == 1 {
			copy(buf, []byte("stale"))
			return 5, resolver.ResolveInfo{FromCache: true}, errors.New("DoH offline")
		}
		clear(buf)
		return 0, resolver.ResolveInfo{}, errors.New("fallback offline")
	})
	buf := make([]byte, 10)
	n, info, err := (retryResolver{inner: inner}).Resolve(context.Background(), query.Query{}, buf)
	if err != nil || !info.FromCache || string(buf[:n]) != "stale" {
		t.Fatalf("lost stale answer: n=%d cache=%v err=%v", n, info.FromCache, err)
	}
}

type fakeResolver struct {
	calls   int
	results []func() (int, resolver.ResolveInfo, error)
}

func (f *fakeResolver) Resolve(context.Context, query.Query, []byte) (int, resolver.ResolveInfo, error) {
	res := f.results[f.calls]
	f.calls++
	return res()
}

func TestRetryResolverRetriesOnce(t *testing.T) {
	inner := &fakeResolver{results: []func() (int, resolver.ResolveInfo, error){
		func() (int, resolver.ResolveInfo, error) { return 0, resolver.ResolveInfo{}, errors.New("conn reset") },
		func() (int, resolver.ResolveInfo, error) { return 42, resolver.ResolveInfo{Transport: "h2"}, nil },
	}}

	n, _, err := retryResolver{inner: inner}.Resolve(context.Background(), query.Query{}, nil)
	if err != nil || n != 42 {
		t.Fatalf("got n=%d err=%v, want n=42 err=nil", n, err)
	}
	if inner.calls != 2 {
		t.Fatalf("calls = %d, want 2", inner.calls)
	}
}

func TestRetryResolverServesStaleCacheOnPersistentError(t *testing.T) {
	fail := func() (int, resolver.ResolveInfo, error) {
		return 100, resolver.ResolveInfo{FromCache: true}, errors.New("upstream unreachable")
	}
	inner := &fakeResolver{results: []func() (int, resolver.ResolveInfo, error){fail, fail}}

	n, i, err := retryResolver{inner: inner}.Resolve(context.Background(), query.Query{}, make([]byte, 100))
	if err != nil {
		t.Fatalf("expected stale answer instead of error, got %v", err)
	}
	if n != 100 || !i.FromCache {
		t.Fatalf("got n=%d fromCache=%v, want stale cached answer", n, i.FromCache)
	}
}

func TestRetryResolverPropagatesErrorWithoutStale(t *testing.T) {
	fail := func() (int, resolver.ResolveInfo, error) {
		return 0, resolver.ResolveInfo{}, errors.New("upstream unreachable")
	}
	inner := &fakeResolver{results: []func() (int, resolver.ResolveInfo, error){fail, fail}}

	if _, _, err := (retryResolver{inner: inner}).Resolve(context.Background(), query.Query{}, nil); err == nil {
		t.Fatal("expected error when no stale cache entry is available")
	}
	if inner.calls != 2 {
		t.Fatalf("calls = %d, want 2", inner.calls)
	}
}

func TestRetryResolverSkipsRetryWhenContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inner := &fakeResolver{results: []func() (int, resolver.ResolveInfo, error){
		func() (int, resolver.ResolveInfo, error) { return 0, resolver.ResolveInfo{}, context.Canceled },
	}}

	if _, _, err := (retryResolver{inner: inner}).Resolve(ctx, query.Query{}, nil); err == nil {
		t.Fatal("expected error")
	}
	if inner.calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on done context)", inner.calls)
	}
}
