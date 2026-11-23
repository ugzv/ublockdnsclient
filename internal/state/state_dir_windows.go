//go:build windows

package state

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SYSTEM and Administrators only, with inheritance from the parent cut off.
// File modes mean nothing on Windows, and the default %ProgramData% ACL lets
// every local user read what is inside and create files of their own.
// maxMigratedFileSize covers the largest file kept here, a full daemon log.
const maxMigratedFileSize = 16 << 20

const stateDirSDDL = "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// secureStateDir makes sure dir exists and only SYSTEM and Administrators can
// reach it. A directory that is there but open to others (an older install,
// or one any local user made ahead of the installer) is replaced rather than
// re-permissioned: handles opened on it or its files earlier keep the access
// they were granted, so only new objects are safe to write secrets into.
func secureStateDir(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	err := createRestrictedDir(dir)
	if err == nil {
		return nil
	}
	if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return &os.PathError{Op: "mkdir", Path: dir, Err: err}
	}
	if isRestricted(dir) {
		return nil
	}

	aside := fmt.Sprintf("%s.replaced-%d", dir, time.Now().UnixNano())
	if err := os.Rename(dir, aside); err != nil {
		return fmt.Errorf("replace unrestricted %s: %w", dir, err)
	}
	// Another process may have recreated it in between; that is fine as long
	// as what is there now is restricted.
	if err := createRestrictedDir(dir); err != nil &&
		!(errors.Is(err, windows.ERROR_ALREADY_EXISTS) && isRestricted(dir)) {
		return &os.PathError{Op: "mkdir", Path: dir, Err: err}
	}
	migrateTrustedFiles(aside, dir)
	return nil
}

func createRestrictedDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(stateDirSDDL)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	return windows.CreateDirectory(name, &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	})
}

// migrateTrustedFiles copies the files an administrator or the service wrote
// in the old directory into the new one, as new files. Everything else stays
// in the old directory, which is removed only if that leaves it empty.
func migrateTrustedFiles(from, to string) {
	if info, err := os.Lstat(from); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(from)
		for _, entry := range entries {
			src := filepath.Join(from, entry.Name())
			if migrateTrustedFile(src, filepath.Join(to, entry.Name())) {
				_ = os.Remove(src)
			}
		}
	}
	if err := os.Remove(from); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: some files were not moved to the restricted %s and remain in %s", to, from)
	}
}

// migrateTrustedFile checks and reads src through one handle: the old
// directory may still belong to someone who can swap the name in between.
func migrateTrustedFile(src, dst string) bool {
	if info, err := os.Lstat(src); err != nil || !info.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(src)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return false
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	if owner, _, err := sd.Owner(); err != nil || !trustedSID(owner) {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(f, maxMigratedFileSize))
	if err != nil {
		return false
	}
	// Never over a file already there: a token persisted to the new
	// directory in the meantime is newer than the one being migrated.
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	_, err = out.Write(data)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(dst)
		return false
	}
	return true
}

func trustedSID(sid *windows.SID) bool {
	return sid != nil && (sid.IsWellKnown(windows.WinLocalSystemSid) ||
		sid.IsWellKnown(windows.WinBuiltinAdministratorsSid))
}

// isRestricted reports whether path is a plain directory owned by SYSTEM or
// Administrators that grants access to nobody else and inherits nothing.
func isRestricted(path string) bool {
	// A junction or symlink is not a directory to Lstat.
	if info, err := os.Lstat(path); err != nil || !info.IsDir() {
		return false
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	if owner, _, err := sd.Owner(); err != nil || !trustedSID(owner) {
		return false
	}
	if control, _, err := sd.Control(); err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	// A missing DACL grants everyone full access.
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return false
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return false
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
			!trustedSID((*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
			return false
		}
	}
	return true
}
