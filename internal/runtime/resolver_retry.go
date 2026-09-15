package runtime

import (
	"context"
	"time"

	"github.com/nextdns/nextdns/resolver"
	"github.com/nextdns/nextdns/resolver/query"
)

const queryTimeout = 5 * time.Second

// retryResolver reserves time for recovery and a second attempt within one
// deadline. An expired cached answer survives even if the fallback has no cache.
type retryResolver struct {
	inner   resolver.Resolver
	recover func(context.Context) error
}

func (r retryResolver) Resolve(ctx context.Context, q query.Query, buf []byte) (int, resolver.ResolveInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	deadline, _ := ctx.Deadline()
	// The resolver contract allows Payload and buf to share memory. Preserve the
	// query before an expired cached response can overwrite it on the first call.
	payload := append([]byte(nil), q.Payload...)
	attempt, cancelAttempt := context.WithTimeout(ctx, min(1500*time.Millisecond, time.Until(deadline)/3))
	n, info, err := r.inner.Resolve(attempt, q, buf)
	cancelAttempt()
	if err == nil {
		return n, info, nil
	}
	var stale []byte
	staleInfo := info
	if n > 0 && info.FromCache {
		stale = append([]byte(nil), buf[:n]...)
	}
	if ctx.Err() == nil {
		if r.recover != nil {
			recovery, cancelRecovery := context.WithTimeout(ctx, min(2*time.Second, time.Until(deadline)/2))
			_ = r.recover(recovery)
			cancelRecovery()
		}
		if ctx.Err() == nil {
			q.Payload = payload
			n, info, err = r.inner.Resolve(ctx, q, buf)
		}
	}
	if err != nil {
		if n > 0 && info.FromCache {
			return n, info, nil
		}
		if len(stale) > 0 {
			return copy(buf, stale), staleInfo, nil
		}
	}
	return n, info, err
}
