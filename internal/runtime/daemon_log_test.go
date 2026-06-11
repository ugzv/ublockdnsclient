package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriterRotatesAtCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	w := &rotatingWriter{path: path}
	if err := w.openLocked(); err != nil {
		t.Fatal(err)
	}

	line := strings.Repeat("x", 1024)
	for written := 0; written <= maxDaemonLogSize; written += len(line) {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}

	old, err := os.Stat(path + ".old")
	if err != nil {
		t.Fatalf("expected rotated file: %v", err)
	}
	cur, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if old.Size() > maxDaemonLogSize || cur.Size() > maxDaemonLogSize {
		t.Fatalf("sizes exceed cap: old=%d cur=%d", old.Size(), cur.Size())
	}
	if cur.Size() == 0 {
		t.Fatal("expected writes to continue into fresh file after rotation")
	}
}
