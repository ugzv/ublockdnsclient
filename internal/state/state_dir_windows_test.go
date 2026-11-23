//go:build windows

package state

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

var aceTrustee = regexp.MustCompile(`;;;([^)]+)\)`)

// assertAdminOnly fails when anyone but SYSTEM and Administrators is granted
// access to path, or when it still inherits permissions from its parent.
func assertAdminOnly(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("read security of %s: %v", path, err)
	}
	sddl := sd.String()
	if !strings.Contains(sddl, "D:P") {
		t.Errorf("%s still inherits permissions from its parent: %s", path, sddl)
	}
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
		t.Skip("needs an elevated process to assign ownership to Administrators")
	}
	t.Setenv("ProgramData", t.TempDir())
}

// The account token must not be readable by other local users, which is what
// a directory created with the inherited %ProgramData% permissions allows.
func TestPersistedTokenIsRestrictedToAdministrators(t *testing.T) {
	useTempProgramData(t)

	if err := PersistToken("profile1", "secret"); err != nil {
		t.Fatal(err)
	}
	assertAdminOnly(t, tokenDir())
	assertAdminOnly(t, filepath.Join(tokenDir(), "profile1.token"))
}

// Any local user can create the state directory before the installer runs;
// what they left there must not stay under their control.
func TestExistingStateDirIsTakenOver(t *testing.T) {
	useTempProgramData(t)

	planted := filepath.Join(tokenDir(), "planted")
	if err := os.MkdirAll(planted, 0o755); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(tokenDir(), "install_state.json")
	if err := os.WriteFile(leftover, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureStateDir(); err != nil {
		t.Fatal(err)
	}
	assertAdminOnly(t, tokenDir())
	assertAdminOnly(t, leftover)
	if _, err := os.Lstat(planted); !os.IsNotExist(err) {
		t.Errorf("planted directory survived: %v", err)
	}
}
