package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type InstallState struct {
	ProfileID string `json:"profile_id"`
	DoHServer string `json:"doh_server,omitempty"`
	APIServer string `json:"api_server,omitempty"`
}

const installStateFile = "install_state.json"

func installStatePath() string {
	return filepath.Join(tokenDir(), installStateFile)
}

func PersistInstallState(profileID, dohServer, apiServer string) error {
	if strings.TrimSpace(profileID) == "" {
		return nil
	}
	if err := ensureStateDir(); err != nil {
		return err
	}
	st := InstallState{
		ProfileID: strings.TrimSpace(profileID),
		DoHServer: strings.TrimSpace(dohServer),
		APIServer: strings.TrimSpace(apiServer),
	}
	return writeStateFile(installStatePath(), st)
}

func LoadInstallState() (InstallState, error) {
	var st InstallState
	if err := readStateFile(installStatePath(), &st); err != nil {
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

func ClearInstallState() error {
	if err := os.Remove(installStatePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeStateFile(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readStateFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
