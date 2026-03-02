//go:build windows

package app

import "os"

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
