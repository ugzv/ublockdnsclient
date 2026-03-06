package runtime

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

func watchRulesUpdates(ctx context.Context, apiServer, profileID, accountToken string, onRulesUpdated func()) {
	token := strings.TrimSpace(accountToken)
	if token == "" {
		return
	}
	log.Printf("Rules update stream enabled for profile %s", profileID)

	currentVersion := int64(0)
	if v, err := fetchRulesVersion(ctx, apiServer, profileID, token); err == nil {
		currentVersion = v.RulesVersion
	}

	// flushDebounce coalesces rapid rule updates into a single OS cache flush.
	// A generation counter ensures that if a timer fires concurrently with a
	// new schedule call, the stale callback is a no-op.
	// maxFlushDelay caps how long continuous events can postpone a flush.
	const maxFlushDelay = 10 * time.Second
	var flushMu sync.Mutex
	var flushTimer *time.Timer
	var flushGen uint64
	var firstPending time.Time
	doFlush := func(version int64) {
		if onRulesUpdated != nil {
			onRulesUpdated()
		}
		if err := core.FlushDNSCaches(); err != nil {
			log.Printf("Rules updated (v%d) but DNS cache flush had issues: %v", version, err)
			return
		}
		log.Printf("Rules updated (v%d), local DNS cache flushed", version)
	}
	scheduleFlush := func(version int64) {
		flushMu.Lock()
		defer flushMu.Unlock()
		now := time.Now()
		if flushTimer != nil {
			flushTimer.Stop()
		} else {
			firstPending = now
		}
		// If events have been arriving continuously for too long, flush now.
		if now.Sub(firstPending) >= maxFlushDelay {
			flushTimer = nil
			go doFlush(version)
			return
		}
		flushGen++
		gen := flushGen
		flushTimer = time.AfterFunc(2*time.Second, func() {
			flushMu.Lock()
			flushTimer = nil
			if gen != flushGen {
				flushMu.Unlock()
				return
			}
			flushMu.Unlock()
			doFlush(version)
		})
	}

	backoff := time.Second
	for {
		streamStart := time.Now()
		if err := consumeRulesStream(ctx, apiServer, profileID, token, func(ev rulesUpdateEvent) {
			if ev.RulesVersion <= currentVersion {
				return
			}
			currentVersion = ev.RulesVersion
			scheduleFlush(currentVersion)
		}); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Rules stream disconnected: %v", err)
		}

		// Reset backoff if the stream was healthy for a meaningful duration.
		if time.Since(streamStart) > 30*time.Second {
			backoff = time.Second
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Reconcile state after disconnect so missed events are still applied.
		if v, err := fetchRulesVersion(ctx, apiServer, profileID, token); err == nil && v.RulesVersion > currentVersion {
			currentVersion = v.RulesVersion
			scheduleFlush(currentVersion)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}
