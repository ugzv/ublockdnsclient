//go:build windows

package state

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"golang.org/x/sys/windows"
)

var aceTrustee = regexp.MustCompile(`;;;([^)]+)\)`)

// assertAdminOnly fails when path grants access to anyone but SYSTEM and
// Administrators.
func assertAdminOnly(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("read security of %s: %v", path, err)
	}
	sddl := sd.String()
	trustees := aceTrustee.FindAllStringSubmatch(sddl, -1)
	if len(trustees) == 0 {
		t.Fatalf("%s has no access entries: %s", path, sddl)
	}
	for _, m := range trustees {
		if m[1] != "SY" && m[1] != "BA" {
			t.Errorf("%s grants access to %s: %s", path, m[1], sddl)
		}
	}
}

func useTempProgramData(t *testing.T) {
	t.Helper()
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("needs an elevated process: files must be owned by Administrators")
	}
	t.Setenv("ProgramData", t.TempDir())
}

// unrestrictedStateDir creates the state directory the way older versions
// did, inheriting whatever its parent allows.
func unrestrictedStateDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(tokenDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if isRestricted(tokenDir()) {
		t.Fatal("a directory with inherited permissions must not count as restricted")
	}
}

// The account token must not be readable by other local users, which is what
// a directory created with the inherited %ProgramData% permissions allows.
func TestPersistedTokenIsRestrictedToAdministrators(t *testing.T) {
	useTempProgramData(t)

	if err := PersistToken("profile1", "secret"); err != nil {
		t.Fatal(err)
	}
	if !isRestricted(tokenDir()) {
		t.Error("state directory is not restricted")
	}
	assertAdminOnly(t, tokenDir())
	assertAdminOnly(t, filepath.Join(tokenDir(), "profile1.token"))
}

// Upgrading from a version that left the directory open must lock it down
// without losing the install state, and without deleting things that are not
// ours.
func TestUnrestrictedStateDirIsReplacedKeepingItsFiles(t *testing.T) {
	useTempProgramData(t)
	unrestrictedStateDir(t)
	if err := os.WriteFile(installStatePath(), []byte(`{"profile_id":"profile1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tokenDir(), "backup"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureStateDir(); err != nil {
		t.Fatal(err)
	}
	if !isRestricted(tokenDir()) {
		t.Error("state directory is not restricted after replacement")
	}
	assertAdminOnly(t, installStatePath())
	if st, err := LoadInstallState(); err != nil || st.ProfileID != "profile1" {
		t.Errorf("install state lost in replacement: %+v, %v", st, err)
	}
	kept, _ := filepath.Glob(filepath.Join(tokenDir()+".replaced-*", "backup"))
	if len(kept) != 1 {
		t.Errorf("unexpected subdirectory was not preserved: %v", kept)
	}
}

// A user who opened the token file while the directory was still open keeps
// that handle after permissions change; it must never see a token written
// later.
func TestHandleOpenedBeforeLockdownCannotReadNewToken(t *testing.T) {
	useTempProgramData(t)
	unrestrictedStateDir(t)
	path := filepath.Join(tokenDir(), "profile1.token")
	if err := os.WriteFile(path, []byte("old-token"), 0o644); err != nil {
		t.Fatal(err)
	}
	held, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()

	// Refusing to persist is as good as writing to a new file.
	_ = PersistToken("profile1", "new-secret")

	buf := make([]byte, 64)
	n, _ := held.ReadAt(buf, 0)
	if bytes.Contains(buf[:n], []byte("new-secret")) {
		t.Fatal("handle opened before lockdown read the new token")
	}
}
