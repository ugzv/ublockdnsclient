package runtime

import (
	"context"
	"fmt"

	"github.com/nextdns/nextdns/resolver/endpoint"
	"github.com/ugzv/ublockdnsclient/internal/core"
)

func testEndpointDomain(ctx context.Context, e endpoint.Endpoint, hostname string) error {
	queryID := uint16(0x4D21)
	_, err := core.ExchangeDNSQuery(queryID, hostname, func(payload, buf []byte) (int, error) {
		return e.Exchange(ctx, payload, buf)
	})
	if err != nil {
		return fmt.Errorf("endpoint probe failed for %q: %w", hostname, err)
	}
	return nil
}
