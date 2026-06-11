package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewer(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.9.0", "2.0.0", true},
		{"1.0.1", "1.0.0", false},
		{"1.0.0", "1.0.0", false},
		{"2.0.0", "1.9.9", false},
		{"dev", "1.0.0", false},
		{"1.0.0", "dev", false},
		{"v1.0.0", "v1.0.1", true},
		{"1.0.0", "1.0.0-rc1", false},
		{"1.0.0", "", false},
	} {
		if got := IsNewer(tc.current, tc.latest); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestHashFor(t *testing.T) {
	sums := []byte("abc123  ublockdns-linux-amd64\ndef456  *ublockdns-darwin-arm64\n")
	if h, err := hashFor(sums, "ublockdns-darwin-arm64"); err != nil || h != "def456" {
		t.Fatalf("got %q, %v", h, err)
	}
	if _, err := hashFor(sums, "missing"); err == nil {
		t.Fatal("expected error for missing asset")
	}
}

// fakeRelease serves a complete signed release for the running platform and
// swaps the package globals to point at it.
func fakeRelease(t *testing.T, version string, binary []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldKey := releasePublicKey
	releasePublicKey = pub
	t.Cleanup(func() { releasePublicKey = oldKey })

	sum := sha256.Sum256(binary)
	sums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName())
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(sums)))

	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v"+version, http.StatusFound)
	})
	mux.HandleFunc("/download/v"+version+"/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})
	mux.HandleFunc("/download/v"+version+"/SHA256SUMS.sig", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sig))
	})
	mux.HandleFunc("/download/v"+version+"/"+assetName(), func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(binary)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	oldURL := releaseBaseURL
	releaseBaseURL = srv.URL
	t.Cleanup(func() { releaseBaseURL = oldURL })
}

func fakeExecutable(t *testing.T, content string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "ublockdns")
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	old := executablePathFunc
	executablePathFunc = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executablePathFunc = old })
	return exe
}

func TestApplyUpgradesAndKeepsBackup(t *testing.T) {
	fakeRelease(t, "2.0.0", []byte("new-binary"))
	exe := fakeExecutable(t, "old-binary")

	v, err := Apply("1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if v != "2.0.0" {
		t.Fatalf("applied version = %q, want 2.0.0", v)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new-binary" {
		t.Fatalf("binary content = %q, want new-binary", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "old-binary" {
		t.Fatalf("backup content = %q, want old-binary", b)
	}
}

func TestApplyNoopWhenUpToDate(t *testing.T) {
	fakeRelease(t, "2.0.0", []byte("new-binary"))
	exe := fakeExecutable(t, "old-binary")

	v, err := Apply("2.0.0", "")
	if err != nil || v != "" {
		t.Fatalf("got %q, %v; want no-op", v, err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old-binary" {
		t.Fatal("binary must be untouched when up to date")
	}
}

func TestApplyRejectsBadSignature(t *testing.T) {
	fakeRelease(t, "2.0.0", []byte("new-binary"))
	// Different key than the one that signed the manifest.
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	releasePublicKey = pub
	exe := fakeExecutable(t, "old-binary")

	if _, err := Apply("1.0.0", ""); err == nil {
		t.Fatal("expected signature verification failure")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old-binary" {
		t.Fatal("binary must be untouched on signature failure")
	}
}

func TestApplyRejectsChecksumMismatch(t *testing.T) {
	fakeRelease(t, "2.0.0", []byte("new-binary"))
	exe := fakeExecutable(t, "old-binary")

	// Tamper with the served binary after the manifest was built.
	oldURL := releaseBaseURL
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download/v2.0.0/"+assetName() {
			_, _ = w.Write([]byte("tampered"))
			return
		}
		r2 := *r
		r2.URL.Scheme = "http"
		proxyTo(w, oldURL+r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	releaseBaseURL = srv.URL
	t.Cleanup(func() { releaseBaseURL = oldURL })

	if _, err := Apply("1.0.0", ""); err == nil {
		t.Fatal("expected checksum mismatch failure")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old-binary" {
		t.Fatal("binary must be untouched on checksum failure")
	}
	if _, err := os.Stat(exe + ".new"); !os.IsNotExist(err) {
		t.Fatal("temp download must be cleaned up")
	}
}

func proxyTo(w http.ResponseWriter, url string) {
	resp, err := http.Get(url)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func TestVersionFromAPIPreferred(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/version" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("v3.1.4\n"))
	}))
	t.Cleanup(srv.Close)

	v, err := LatestVersion(srv.URL)
	if err != nil || v != "3.1.4" {
		t.Fatalf("got %q, %v; want 3.1.4", v, err)
	}
}

func TestLatestVersionFallsBackToGitHub(t *testing.T) {
	fakeRelease(t, "1.2.3", []byte("x"))
	v, err := LatestVersion("")
	if err != nil || v != "1.2.3" {
		t.Fatalf("got %q, %v; want 1.2.3", v, err)
	}
}
