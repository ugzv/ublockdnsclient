package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/query"
)

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

	n, i, err := retryResolver{inner: inner}.Resolve(context.Background(), query.Query{}, nil)
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
