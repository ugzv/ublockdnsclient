package runtime

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/nextdns/nextdns/proxy"
)

type proxyRunner struct {
	proxy   proxy.Proxy
	onInit  []func(ctx context.Context)
	cancel  context.CancelFunc
	stopped chan struct{}
}

func (p *proxyRunner) Start() error {
	errC := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.stopped = make(chan struct{})

	for _, f := range p.onInit {
		go f(ctx)
	}

	go func() {
		defer close(p.stopped)
		if err := p.proxy.ListenAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case errC <- err:
			default:
			}
		}
	}()

	// Match NextDNS service startup behavior: return quickly after spawn,
	// while still surfacing immediate startup failures.
	select {
	case err := <-errC:
		return err
	case <-time.After(5 * time.Second):
		return nil
	}
}

func (p *proxyRunner) Stop() error {
	if p.cancel == nil {
		return nil
	}
	p.cancel()
	p.cancel = nil
	if p.stopped != nil {
		<-p.stopped
	}
	return nil
}

func (p *proxyRunner) Log(msg string) {
	log.Println(msg)
}
