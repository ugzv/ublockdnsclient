package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type installState struct {
	ProfileID string `json:"profile_id"`
	DoHServer string `json:"doh_server,omitempty"`
	APIServer string `json:"api_server,omitempty"`
}

const installStateFile = "install_state.json"

func tokenPath(profileID string) (string, error) {
	if err := ValidateProfileID(profileID); err != nil {
		return "", err
	}
	return filepath.Join(tokenDir(), profileID+".token"), nil
}

func persistToken(profileID, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	if err := os.MkdirAll(tokenDir(), 0o700); err != nil {
		return err
	}
	p, err := tokenPath(profileID)
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(strings.TrimSpace(token)), 0o600)
}

func loadPersistedToken(profileID string) (string, error) {
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

func clearPersistedTokens() error {
	baseDir := tokenDir()
	matches, err := filepath.Glob(filepath.Join(baseDir, "*.token"))
	if err != nil {
		return err
	}
	for _, p := range matches {
		_ = os.Remove(p)
	}
	_ = clearInstallState()
	_ = os.Remove(baseDir)
	return nil
}

func installStatePath() string {
	return filepath.Join(tokenDir(), installStateFile)
}

func persistInstallState(profileID, dohServer, apiServer string) error {
	if strings.TrimSpace(profileID) == "" {
		return nil
	}
	if err := os.MkdirAll(tokenDir(), 0o700); err != nil {
		return err
	}
	st := installState{
		ProfileID: strings.TrimSpace(profileID),
		DoHServer: strings.TrimSpace(dohServer),
		APIServer: strings.TrimSpace(apiServer),
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(installStatePath(), b, 0o600)
}

func loadInstallState() (installState, error) {
	var st installState
	b, err := os.ReadFile(installStatePath())
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	if strings.TrimSpace(st.ProfileID) == "" {
		return st, errors.New("empty install state profile id")
	}
	st.ProfileID = strings.TrimSpace(st.ProfileID)
	st.DoHServer = strings.TrimSpace(st.DoHServer)
	st.APIServer = strings.TrimSpace(st.APIServer)
	return st, nil
}

func clearInstallState() error {
	if err := os.Remove(installStatePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func tokenDir() string {
	if runtime.GOOS == "windows" {
		if programData := strings.TrimSpace(os.Getenv("ProgramData")); programData != "" {
			return filepath.Join(programData, "ublockdns")
		}
		return filepath.Join(`C:\ProgramData`, "ublockdns")
	}
	return "/etc/ublockdns"
}
