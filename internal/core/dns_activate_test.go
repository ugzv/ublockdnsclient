package core

import (
	"errors"
	"testing"
)

func installTestDNSHooks(t *testing.T, set func(string) error, reset func() error) {
	t.Helper()
	t.Cleanup(SwapSystemDNSFuncs(set, reset))
}

func TestActivateSystemDNSUsesLocalAddress(t *testing.T) {
	var got string
	installTestDNSHooks(t, func(addr string) error {
		got = addr
		return nil
	}, nil)

	if err := ActivateSystemDNS(); err != nil {
		t.Fatalf("ActivateSystemDNS() error = %v", err)
	}
	if got != LocalDNSAddress {
		t.Fatalf("set DNS address = %q, want %q", got, LocalDNSAddress)
	}
}

func TestActivateSystemDNSPropagatesError(t *testing.T) {
	want := errors.New("set dns failed")
	installTestDNSHooks(t, func(string) error { return want }, nil)

	if err := ActivateSystemDNS(); !errors.Is(err, want) {
		t.Fatalf("ActivateSystemDNS() error = %v, want %v", err, want)
	}
}

func TestRestoreSystemDNSStrictPropagatesError(t *testing.T) {
	want := errors.New("reset dns failed")
	installTestDNSHooks(t, nil, func() error { return want })

	if err := RestoreSystemDNSStrict(); !errors.Is(err, want) {
		t.Fatalf("RestoreSystemDNSStrict() error = %v, want %v", err, want)
	}
}

func TestRestoreSystemDNSWithWarningsReturnsLegacyFailure(t *testing.T) {
	installTestDNSHooks(t, nil, func() error { return errors.New("reset dns failed") })

	warnings := RestoreSystemDNSWithWarnings()
	if len(warnings) == 0 {
		t.Fatal("expected restore warnings")
	}
}
