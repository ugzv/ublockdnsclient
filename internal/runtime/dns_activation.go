package runtime

import (
	"context"
	"log"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

var watchNetworkDNSChanges = defaultWatchNetworkDNSChanges

func defaultWatchNetworkDNSChanges(ctx context.Context) {
	changes := make(chan string, 1)
	go watchNetworkChanges(ctx, changes)

	for {
		select {
		case <-ctx.Done():
			return
		case reason := <-changes:
			log.Printf("Network change detected: %s; re-activating system DNS", reason)
			core.ActivatePlatformSystemDNSBestEffort()
		}
	}
}

// manageSystemDNS activates local DNS for the lifetime of ctx, re-applying on
// network changes and restoring defaults when the proxy stops.
func manageSystemDNS(ctx context.Context) {
	log.Println("Activating system DNS")
	core.ActivatePlatformSystemDNSBestEffort()
	defer func() {
		log.Println("Restoring system DNS")
		core.RestoreSystemDNSBestEffort()
	}()

	watchNetworkDNSChanges(ctx)
}
