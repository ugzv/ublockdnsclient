package runtime

import (
	"context"
	"log"
	"math/rand"
	"os"
	goruntime "runtime"
	"time"

	"github.com/ugzv/ublockdnsclient/internal/service"
	"github.com/ugzv/ublockdnsclient/internal/update"
)

var (
	applyUpdateFunc = update.Apply
	relaunchFunc    = relaunch
	canRelaunchFunc = service.RelaunchAfterUpdateSupported
	autoUpdateDelay = func(first bool) time.Duration {
		if first {
			return 5*time.Minute + time.Duration(rand.Int63n(int64(10*time.Minute)))
		}
		return 23*time.Hour + time.Duration(rand.Int63n(int64(2*time.Hour)))
	}
)

// watchUpdates checks for new releases roughly daily and applies them by
// swapping the binary and exiting, relying on the service manager to relaunch
// the new version.
func watchUpdates(ctx context.Context, currentVersion, apiServer string) {
	if os.Getenv("UBLOCKDNS_NO_AUTOUPDATE") == "1" {
		log.Println("Auto-update disabled by UBLOCKDNS_NO_AUTOUPDATE")
		return
	}
	if !update.ValidVersion(currentVersion) {
		return
	}
	if !canRelaunchFunc() {
		log.Println("Auto-update disabled: service manager would not relaunch after update; run 'ublockdns upgrade' to update manually")
		return
	}

	timer := time.NewTimer(autoUpdateDelay(true))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		switch v, err := applyUpdateFunc(currentVersion, apiServer); {
		case err != nil:
			log.Printf("Auto-update check failed: %v", err)
		case v != "":
			log.Printf("Updated to v%s, restarting", v)
			relaunchFunc()
			return
		}
		timer.Reset(autoUpdateDelay(false))
	}
}

func relaunch() {
	// Windows SCM recovery only restarts on failure exits; launchd KeepAlive
	// and systemd Restart=always relaunch on any exit.
	if goruntime.GOOS == "windows" {
		os.Exit(1)
	}
	os.Exit(0)
}
