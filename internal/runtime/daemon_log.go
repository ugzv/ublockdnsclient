package runtime

import (
	"log"
	"os"
	"sync"

	"github.com/nextdns/nextdns/host/service"

	"github.com/ugzv/ublockdnsclient/internal/state"
)

const maxDaemonLogSize = 5 << 20

// setupDaemonLogging redirects log output to a size-capped file when running
// as a system service; the init systems we install under otherwise discard
// stderr.
func setupDaemonLogging() {
	if service.CurrentRunMode() != service.RunModeService {
		return
	}
	// Under systemd stderr already lands in journald.
	if os.Getenv("JOURNAL_STREAM") != "" {
		return
	}
	w := &rotatingWriter{path: state.DaemonLogPath()}
	if w.openLocked() == nil {
		log.SetOutput(w)
	}
}

// rotatingWriter appends to path, renaming it to path+".old" whenever it
// would exceed maxDaemonLogSize, so a long-running daemon keeps at most two
// capped files.
type rotatingWriter struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
}

func (w *rotatingWriter) openLocked() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w.f = f
	w.size = 0
	if info, err := f.Stat(); err == nil {
		w.size = info.Size()
	}
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > maxDaemonLogSize {
		_ = w.f.Close()
		_ = os.Rename(w.path, w.path+".old")
		if err := w.openLocked(); err != nil {
			return len(p), nil
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}
