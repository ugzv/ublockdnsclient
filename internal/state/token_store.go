package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ugzv/ublockdnsclient/internal/core"
)

func tokenPath(profileID string) (string, error) {
	if err := core.ValidateProfileID(profileID); err != nil {
		return "", err
	}
	return filepath.Join(tokenDir(), profileID+".token"), nil
}

func PersistToken(profileID, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	if err := ensureStateDir(); err != nil {
		return err
	}
	p, err := tokenPath(profileID)
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(strings.TrimSpace(token)), 0o600)
}

func LoadPersistedToken(profileID string) (string, error) {
	p, err := tokenPath(profileID)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", errors.New("empty persisted token")
	}
	return token, nil
}

func ClearPersistedTokens() error {
	baseDir := tokenDir()
	matches, err := filepath.Glob(filepath.Join(baseDir, "*.token"))
	if err != nil {
		return err
	}
	for _, p := range matches {
		_ = os.Remove(p)
	}
	_ = ClearInstallState()
	_ = os.Remove(baseDir)
	return nil
}
