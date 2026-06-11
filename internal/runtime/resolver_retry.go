package runtime

import (
	"context"

	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/query"
)

// retryResolver masks transient upstream failures (dead connections after
// wake/network change) that the proxy would otherwise surface as SERVFAIL:
// it retries a failed resolve once, and if the upstream is still unreachable
// it serves the expired cache entry the inner resolver leaves in buf (see
// resolver.Resolver contract) instead of failing.
type retryResolver struct {
	inner resolver.Resolver
}

func (r retryResolver) Resolve(ctx context.Context, q query.Query, buf []byte) (int, resolver.ResolveInfo, error) {
	n, i, err := r.inner.Resolve(ctx, q, buf)
	if err != nil && ctx.Err() == nil {
		n, i, err = r.inner.Resolve(ctx, q, buf)
	}
	if err != nil && n > 0 && i.FromCache {
		return n, i, nil
	}
	return n, i, err
}
