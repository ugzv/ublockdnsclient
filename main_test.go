package main

import (
	"testing"

	"github.com/ugzv/ublockdnsclient/internal/app"
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
			gotURL, gotHost, gotPath, err := app.BuildDoHTarget(tt.base, tt.profileID)
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
		if err := app.ValidateProfileID(id); err != nil {
			t.Fatalf("expected valid profile id %q, got error: %v", id, err)
		}
	}

	invalid := []string{"", " ", "abc/123", "../evil", "a b", "-token"}
	for _, id := range invalid {
		if err := app.ValidateProfileID(id); err == nil {
			t.Fatalf("expected invalid profile id %q to fail validation", id)
		}
	}
}

func TestResolveDoHServer(t *testing.T) {
	t.Setenv("UBLOCKDNS_DOH_SERVER", "")
	if got := app.ResolveDoHServer(""); got != app.DefaultDoHServer {
		t.Fatalf("expected default server %q, got %q", app.DefaultDoHServer, got)
	}

	t.Setenv("UBLOCKDNS_DOH_SERVER", "https://env.example.com/")
	if got := app.ResolveDoHServer(""); got != "https://env.example.com" {
		t.Fatalf("expected env override, got %q", got)
	}

	if got := app.ResolveDoHServer("https://flag.example.com/"); got != "https://flag.example.com" {
		t.Fatalf("expected flag override, got %q", got)
	}
}

func TestResolveAPIServer(t *testing.T) {
	t.Setenv("UBLOCKDNS_API_SERVER", "")
	if got := app.ResolveAPIServer("", app.DefaultDoHServer); got != app.DefaultAPIServer {
		t.Fatalf("expected default API server %q, got %q", app.DefaultAPIServer, got)
	}

	t.Setenv("UBLOCKDNS_API_SERVER", "https://api-env.example.com/")
	if got := app.ResolveAPIServer("", app.DefaultDoHServer); got != "https://api-env.example.com" {
		t.Fatalf("expected env API override, got %q", got)
	}

	if got := app.ResolveAPIServer("https://api-flag.example.com/", app.DefaultDoHServer); got != "https://api-flag.example.com" {
		t.Fatalf("expected flag API override, got %q", got)
	}
}
