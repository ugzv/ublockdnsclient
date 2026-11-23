//go:build windows

package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SYSTEM and Administrators only, with inheritance from the parent cut off.
// File modes mean nothing on Windows, and the default %ProgramData% ACL lets
// every local user read what is inside and create files of their own.
const (
	stateDirSDDL  = "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	stateFileSDDL = "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)"
)

// secureStateDir creates dir restricted to SYSTEM and Administrators. A
// directory that already exists (an older install, or one any local user
// made ahead of the installer) is taken over: ownership and permissions are
// reset on it and on the files inside.
func secureStateDir(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(stateDirSDDL)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	err = windows.CreateDirectory(name, sa)
	if err == nil {
		return nil
	}
	if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return &os.PathError{Op: "mkdir", Path: dir, Err: err}
	}
	return takeOverStateDir(dir)
}

func takeOverStateDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	// A junction or symlink here would send privileged writes somewhere else.
	if !info.IsDir() {
		return fmt.Errorf("%s is not a plain directory; remove it and try again", dir)
	}
	if err := applySecurity(dir, stateDirSDDL); err != nil {
		return fmt.Errorf("restrict %s: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		// Only plain files are ever written here; anything else was planted.
		if !info.Mode().IsRegular() {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			continue
		}
		if err := applySecurity(path, stateFileSDDL); err != nil {
			return fmt.Errorf("restrict %s: %w", path, err)
		}
	}
	return nil
}

func applySecurity(path, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner, nil, dacl, nil)
}
