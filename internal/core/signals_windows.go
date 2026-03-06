//go:build windows

package core

import "os"

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
