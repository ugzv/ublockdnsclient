package service

import (
	"fmt"

	"github.com/ugzv/ublockdnsclient/internal/update"
)

// Upgrade updates the binary to the latest release and restarts the service.
// Returns the new version, or "" when already up to date.
func Upgrade(currentVersion, apiServer string) (string, error) {
	if !hasInstallPrivileges() {
		return "", fmt.Errorf("upgrade requires elevated privileges - %s", installPrivilegeHint())
	}
	ensureSystemdRestartPolicy()
	v, err := update.Apply(currentVersion, apiServer)
	if err != nil || v == "" {
		return v, err
	}
	if serviceCurrentlyInstalled() {
		svc, err := baseService()
		if err == nil {
			err = svc.Restart()
		}
		if err != nil {
			return v, fmt.Errorf("binary updated to v%s but service restart failed: %w", v, err)
		}
	}
	return v, nil
}
