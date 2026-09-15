package runtime

import (
	"context"
	"log"
	"sync"
	"time"
)

type proxyRunner struct {
	proxy   interface{ ListenAndServe(context.Context) error }
	onInit  []func(ctx context.Context)
	onReady []func(ctx context.Context)
	cancel  context.CancelFunc
	stopped chan struct{}
	err     error
}

func (p *proxyRunner) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.stopped = make(chan struct{})
	p.err = nil

	for _, f := range p.onInit {
		go f(ctx)
	}

	// Ready hooks own system DNS and must finish restoring it before Stop
	// returns. Init hooks are cancelled but not joined: an update download
	// does not currently support context cancellation.
	ready := make(chan struct{})
	var hooks sync.WaitGroup
	for _, f := range p.onReady {
		hooks.Add(1)
		go func() {
			defer hooks.Done()
			select {
			case <-ctx.Done():
				return
			case <-ready:
				if ctx.Err() == nil {
					f(ctx)
				}
			}
		}()
	}

	go func() {
		defer close(p.stopped)
		if err := p.proxy.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
			p.err = err
			log.Printf("DNS proxy stopped: %v", err)
		}
		cancel()
		hooks.Wait()
	}()

	// Match NextDNS service startup behavior: return quickly after spawn,
	// while still surfacing immediate startup failures. The 5s window is a
	// best-effort readiness heuristic, not a confirmed bind — onReady hooks
	// (e.g. system DNS activation) must tolerate the proxy stopping shortly after.
	select {
	case <-p.stopped:
		return p.err
	case <-time.After(5 * time.Second):
		close(ready)
		return nil
	}
}

func (p *proxyRunner) Stop() error {
	if p.cancel == nil {
		return nil
	}
	p.cancel()
	if p.stopped != nil {
		<-p.stopped
	}
	return p.err
}

func (p *proxyRunner) Log(msg string) {
	log.Println(msg)
}
