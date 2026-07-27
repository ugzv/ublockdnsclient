package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ugzv/ublockdnsclient/internal/core"
	"github.com/ugzv/ublockdnsclient/internal/runtime"
)

func TestBuildDoHTarget(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		profileID string
		wantURL   string
		wantHost  string
		wantPath  string
		wantErr   bool
	}{
		{
			name:      "valid base",
			base:      "https://my.ublockdns.com",
			profileID: "abc123",
			wantURL:   "https://my.ublockdns.com/abc123",
			wantHost:  "my.ublockdns.com",
			wantPath:  "/abc123",
		},
		{
			name:      "base with existing path",
			base:      "https://example.com/dns-query",
			profileID: "abc123",
			wantURL:   "https://example.com/dns-query/abc123",
			wantHost:  "example.com",
			wantPath:  "/dns-query/abc123",
		},
		{
			name:      "invalid base",
			base:      "://invalid",
			profileID: "abc123",
			wantErr:   true,
		},
		{
			name:      "invalid profile id",
			base:      "https://my.ublockdns.com",
			profileID: "../etc/passwd",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotHost, gotPath, err := runtime.BuildDoHTarget(tt.base, tt.profileID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotURL != tt.wantURL {
				t.Fatalf("url mismatch: want %q got %q", tt.wantURL, gotURL)
			}
			if gotHost != tt.wantHost {
				t.Fatalf("host mismatch: want %q got %q", tt.wantHost, gotHost)
			}
			if gotPath != tt.wantPath {
				t.Fatalf("path mismatch: want %q got %q", tt.wantPath, gotPath)
			}
		})
	}
}

func TestValidateProfileID(t *testing.T) {
	valid := []string{"abc123", "ABC_123", "profile-id"}
	for _, id := range valid {
		if err := core.ValidateProfileID(id); err != nil {
			t.Fatalf("expected valid profile id %q, got error: %v", id, err)
		}
	}

	invalid := []string{"", " ", "abc/123", "../evil", "a b", "-token"}
	for _, id := range invalid {
		if err := core.ValidateProfileID(id); err == nil {
			t.Fatalf("expected invalid profile id %q to fail validation", id)
		}
	}
}

func TestNormalizeProfileIDInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		err   bool
	}{
		{
			name:  "plain id",
			input: "mn1b8wig",
			want:  "mn1b8wig",
		},
		{
			name:  "profile url",
			input: "https://my.ublockdns.com/mn1b8wig",
			want:  "mn1b8wig",
		},
		{
			name:  "profile url with slash and query",
			input: "https://my.ublockdns.com/mn1b8wig/?ref=abc",
			want:  "mn1b8wig",
		},
		{
			name:  "url without id path",
			input: "https://my.ublockdns.com/",
			err:   true,
		},
		{
			name:  "invalid extracted id",
			input: "https://my.ublockdns.com/a%20b",
			err:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.NormalizeProfileIDInput(tt.input)
			if tt.err {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("want %q got %q", tt.want, got)
			}
		})
	}
}

func TestResolveDoHServer(t *testing.T) {
	t.Setenv("UBLOCKDNS_DOH_SERVER", "")
	if got := runtime.ResolveDoHServer(""); got != core.DefaultDoHServer {
		t.Fatalf("expected default server %q, got %q", core.DefaultDoHServer, got)
	}

	t.Setenv("UBLOCKDNS_DOH_SERVER", "https://env.example.com/")
	if got := runtime.ResolveDoHServer(""); got != "https://env.example.com" {
		t.Fatalf("expected env override, got %q", got)
	}

	if got := runtime.ResolveDoHServer("https://flag.example.com/"); got != "https://flag.example.com" {
		t.Fatalf("expected flag override, got %q", got)
	}

	if got := runtime.ResolveDoHServer("   https://trim.example.com/   "); got != "https://trim.example.com" {
		t.Fatalf("expected trimmed flag override, got %q", got)
	}
}

func TestResolveAPIServer(t *testing.T) {
	t.Setenv("UBLOCKDNS_API_SERVER", "")
	if got := runtime.ResolveAPIServer(""); got != core.DefaultAPIServer {
		t.Fatalf("expected default API server %q, got %q", core.DefaultAPIServer, got)
	}

	t.Setenv("UBLOCKDNS_API_SERVER", "https://api-env.example.com/")
	if got := runtime.ResolveAPIServer(""); got != "https://api-env.example.com" {
		t.Fatalf("expected env API override, got %q", got)
	}

	if got := runtime.ResolveAPIServer("https://api-flag.example.com/"); got != "https://api-flag.example.com" {
		t.Fatalf("expected flag API override, got %q", got)
	}

	if got := runtime.ResolveAPIServer("   https://api-trim.example.com/   "); got != "https://api-trim.example.com" {
		t.Fatalf("expected trimmed flag API override, got %q", got)
	}
}

func TestFlagParsing(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	tests := []struct {
		name  string
		args  []string
		want  string
		json  bool
		token string
	}{
		{
			name: "space separated",
			args: []string{"ublockdns", "install", "-profile", "abc123"},
			want: "abc123",
		},
		{
			name: "inline value",
			args: []string{"ublockdns", "install", "-profile=abc123"},
			want: "abc123",
		},
		{
			name: "double dash",
			args: []string{"ublockdns", "install", "--profile", "abc123"},
			want: "abc123",
		},
		{
			name: "double dash inline",
			args: []string{"ublockdns", "install", "--profile=abc123"},
			want: "abc123",
		},
		{
			name:  "mixed forms with boolean flag",
			args:  []string{"ublockdns", "install", "--profile=abc123", "-token", "t0k", "--json"},
			want:  "abc123",
			json:  true,
			token: "t0k",
		},
		{
			name: "missing value before another flag",
			args: []string{"ublockdns", "install", "-profile", "-json"},
			want: "",
			json: true,
		},
		{
			name: "value containing equals is preserved",
			args: []string{"ublockdns", "install", "-token=a=b"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			if got := flagValue("-profile"); got != tt.want {
				t.Fatalf("flagValue(-profile) = %q, want %q", got, tt.want)
			}
			if got := flagPresent("-json"); got != tt.json {
				t.Fatalf("flagPresent(-json) = %v, want %v", got, tt.json)
			}
			if tt.token != "" {
				if got := flagValue("-token"); got != tt.token {
					t.Fatalf("flagValue(-token) = %q, want %q", got, tt.token)
				}
			}
		})
	}

	os.Args = []string{"ublockdns", "install", "-token=a=b"}
	if got := flagValue("-token"); got != "a=b" {
		t.Fatalf("flagValue(-token) = %q, want %q", got, "a=b")
	}
}

func TestResolveTokenArg(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "tok")
	if err := os.WriteFile(tokenFile, []byte("  file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		env  string
		want string
	}{
		{
			name: "no token configured",
			args: []string{"ublockdns", "install", "-profile", "p"},
			want: "",
		},
		{
			name: "flag value",
			args: []string{"ublockdns", "install", "-token", "flag-token"},
			want: "flag-token",
		},
		{
			name: "token file contents are read and trimmed",
			args: []string{"ublockdns", "install", "-token-file", tokenFile},
			want: "file-token",
		},
		{
			name: "environment fallback",
			args: []string{"ublockdns", "install", "-profile", "p"},
			env:  "env-token",
			want: "env-token",
		},
		{
			name: "flag beats token file",
			args: []string{"ublockdns", "install", "-token", "flag-token", "-token-file", tokenFile},
			want: "flag-token",
		},
		{
			name: "token file beats environment",
			args: []string{"ublockdns", "install", "-token-file", tokenFile},
			env:  "env-token",
			want: "file-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			t.Setenv("UBLOCKDNS_ACCOUNT_TOKEN", tt.env)

			got, err := resolveTokenArg()
			if err != nil {
				t.Fatalf("resolveTokenArg() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveTokenArg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTokenArgMissingFile(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	os.Args = []string{"ublockdns", "install", "-token-file", filepath.Join(t.TempDir(), "absent")}
	if _, err := resolveTokenArg(); err == nil {
		t.Fatal("resolveTokenArg() returned no error for a missing token file")
	}
}

// The installed service invokes the binary with the argument vector built by
// service.newService, e.g. "ublockdns run -profile <id>". If parsing of that
// vector ever regresses, every daemon fails to start after an auto-update and
// users lose DNS entirely, so it is pinned here.
func TestParsesInstalledServiceArguments(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	const exe = "/usr/local/bin/ublockdns"

	tests := []struct {
		name      string
		args      []string
		profileID string
		dohServer string
		apiServer string
	}{
		{
			name:      "profile only, as installed by the default flow",
			args:      []string{exe, "run", "-profile", "sp5a1t42"},
			profileID: "sp5a1t42",
		},
		{
			name:      "profile and DoH server override",
			args:      []string{exe, "run", "-profile", "abc123", "-server", "https://my.ublockdns.com"},
			profileID: "abc123",
			dohServer: "https://my.ublockdns.com",
		},
		{
			name:      "every override the service factory can emit",
			args:      []string{exe, "run", "-profile", "abc123", "-server", "https://my.ublockdns.com", "-api-server", "https://ublockdns.com"},
			profileID: "abc123",
			dohServer: "https://my.ublockdns.com",
			apiServer: "https://ublockdns.com",
		},
		{
			name:      "profile id supplied as a dashboard URL",
			args:      []string{exe, "install", "-profile", "https://ublockdns.com/p/abc123"},
			profileID: "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			t.Setenv("UBLOCKDNS_ACCOUNT_TOKEN", "")

			got, err := parseProfileArgs()
			if err != nil {
				t.Fatalf("parseProfileArgs() error = %v; the daemon would fail to start", err)
			}
			if got.profileID != tt.profileID {
				t.Errorf("profileID = %q, want %q", got.profileID, tt.profileID)
			}
			if got.dohServer != tt.dohServer {
				t.Errorf("dohServer = %q, want %q", got.dohServer, tt.dohServer)
			}
			if got.apiServer != tt.apiServer {
				t.Errorf("apiServer = %q, want %q", got.apiServer, tt.apiServer)
			}
		})
	}
}
