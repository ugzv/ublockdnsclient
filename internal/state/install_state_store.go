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
	if err := EnsureStateDir(); err != nil {
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
	return writeFileFresh(path, data)
}

// writeFileFresh replaces path with a newly created file instead of writing
// into the existing one, so a handle someone opened on the old file earlier
// never sees the new content. The temporary file is created 0600.
func writeFileFresh(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

func readStateFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
