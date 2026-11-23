//go:build !windows

package state

import "os"

func secureStateDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}
