package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	releaseBaseURL     = "https://github.com/ugzv/ublockdnsclient/releases"
	httpClient         = &http.Client{Timeout: 5 * time.Minute}
	executablePathFunc = defaultExecutablePath
)

func defaultExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// LatestVersion returns the newest released version without the leading "v".
// The API server is asked first so checks stay first-party; the GitHub
// release redirect is the fallback.
func LatestVersion(apiServer string) (string, error) {
	if v, err := versionFromAPI(apiServer); err == nil {
		return v, nil
	}
	return versionFromGitHub()
}

func versionFromAPI(apiServer string) (string, error) {
	if strings.TrimSpace(apiServer) == "" {
		return "", errors.New("no API server configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", apiServer+"/client/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version endpoint returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(strings.SplitN(string(body), "\n", 2)[0])
	v = strings.TrimPrefix(v, "v")
	if !ValidVersion(v) {
		return "", fmt.Errorf("invalid version %q from API server", v)
	}
	return v, nil
}

func versionFromGitHub() (string, error) {
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Get(releaseBaseURL + "/latest")
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
	loc := resp.Header.Get("Location")
	idx := strings.LastIndex(loc, "/tag/")
	if idx < 0 {
		return "", fmt.Errorf("no release tag in redirect %q", loc)
	}
	v := strings.TrimPrefix(loc[idx+len("/tag/"):], "v")
	if !ValidVersion(v) {
		return "", fmt.Errorf("invalid version %q from release tag", v)
	}
	return v, nil
}

func ValidVersion(v string) bool {
	_, ok := parseVersion(v)
	return ok
}

// IsNewer reports whether latest is a strictly newer release than current.
// Unparseable versions (e.g. "dev" builds) are never upgraded.
func IsNewer(current, latest string) bool {
	c, okC := parseVersion(current)
	l, okL := parseVersion(latest)
	if !okC || !okL {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// Apply replaces the current executable with the latest release when one is
// newer than currentVersion, after verifying the signed checksum manifest.
// The previous binary is kept alongside as ".old". Returns the applied
// version, or "" when already up to date.
func Apply(currentVersion, apiServer string) (string, error) {
	latest, err := LatestVersion(apiServer)
	if err != nil {
		return "", fmt.Errorf("check latest version: %w", err)
	}
	if !IsNewer(currentVersion, latest) {
		return "", nil
	}

	exe, err := executablePathFunc()
	if err != nil {
		return "", err
	}
	base := releaseBaseURL + "/download/v" + latest
	asset := assetName()

	sums, err := fetch(base + "/SHA256SUMS")
	if err != nil {
		return "", fmt.Errorf("fetch checksums: %w", err)
	}
	sig, err := fetch(base + "/SHA256SUMS.sig")
	if err != nil {
		return "", fmt.Errorf("fetch checksum signature: %w", err)
	}
	if err := verifySignature(sums, sig); err != nil {
		return "", fmt.Errorf("checksum signature: %w", err)
	}
	wantHash, err := hashFor(sums, asset)
	if err != nil {
		return "", err
	}

	newPath := exe + ".new"
	if err := downloadTo(base+"/"+asset, newPath); err != nil {
		return "", fmt.Errorf("download %s: %w", asset, err)
	}
	defer func() { _ = os.Remove(newPath) }()
	gotHash, err := fileSHA256(newPath)
	if err != nil {
		return "", err
	}
	if gotHash != wantHash {
		return "", fmt.Errorf("checksum mismatch for %s", asset)
	}
	if err := os.Chmod(newPath, 0o755); err != nil {
		return "", err
	}

	oldPath := exe + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(exe, oldPath); err != nil {
		return "", fmt.Errorf("back up current binary: %w", err)
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(oldPath, exe)
		return "", fmt.Errorf("install new binary: %w", err)
	}
	return latest, nil
}

func assetName() string {
	name := "ublockdns-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOARCH == "arm" {
		name += "v7"
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func get(url string) (*http.Response, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %d", url, resp.StatusCode)
	}
	return resp, nil
}

func fetch(url string) ([]byte, error) {
	resp, err := get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func downloadTo(url, path string) error {
	resp, err := get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 200<<20)); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func hashFor(sums []byte, asset string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum for %s in manifest", asset)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
