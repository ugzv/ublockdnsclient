//go:build linux

package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testLinuxDNSPaths(t *testing.T, dir string) linuxDNSPaths {
	t.Helper()
	paths := linuxDNSPaths{
		ResolvConf:         filepath.Join(dir, "resolv.conf"),
		ResolvUBlockDNSBak: filepath.Join(dir, "resolv.conf.ublockdns.bak"),
		ResolvNextDNSBak:   filepath.Join(dir, "resolv.conf.nextdns-bak"),
		ResolvedDropIn:     filepath.Join(dir, "resolved.conf.d", "ublockdns.conf"),
		NMUBlockDNSConf:    filepath.Join(dir, "NetworkManager", "conf.d", "ublockdns.conf"),
		NMNextDNSConf:      filepath.Join(dir, "NetworkManager", "conf.d", "nextdns.conf"),
		ResolvconfHead:     filepath.Join(dir, "resolvconf", "head"),
		ResolvconfHeadBak:  filepath.Join(dir, "resolvconf", "head.ublockdns.bak"),
		ConnmanMainConf:    filepath.Join(dir, "connman", "main.conf"),
		ConnmanMainConfBak: filepath.Join(dir, "connman", "main.conf.ublockdns.bak"),
		DhclientConfs:      []string{filepath.Join(dir, "dhclient.conf")},
	}
	t.Cleanup(SwapLinuxDNSPaths(paths))
	return paths
}

func stubLinuxDNSCommands(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", "")
	t.Cleanup(SwapCommandRunner(func(name string, args ...string) error {
		if name == "systemctl" && len(args) >= 2 && args[0] == "is-active" {
			return errors.New("inactive")
		}
		return nil
	}))
}

func TestRestoreResolvConfFromUBlockDNSBackup(t *testing.T) {
	dir := t.TempDir()
	paths := testLinuxDNSPaths(t, dir)

	backup := []byte("nameserver 192.168.1.1\n")
	if err := os.WriteFile(paths.ResolvUBlockDNSBak, backup, 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	if err := os.WriteFile(paths.ResolvConf, []byte("# Managed by uBlockDNS\nnameserver 127.0.0.1\n"), 0o644); err != nil {
		t.Fatalf("write resolv.conf: %v", err)
	}

	restored, err := restoreResolvConfFromUBlockDNSBackup(paths)
	if err != nil {
		t.Fatalf("restoreResolvConfFromUBlockDNSBackup() error = %v", err)
	}
	if !restored {
		t.Fatal("expected backup restore")
	}

	got, err := os.ReadFile(paths.ResolvConf)
	if err != nil {
		t.Fatalf("read resolv.conf: %v", err)
	}
	if string(got) != string(backup) {
		t.Fatalf("resolv.conf = %q, want %q", got, backup)
	}
}

func TestCleanupManagedResolvConfIfNeeded(t *testing.T) {
	stubLinuxDNSCommands(t)
	dir := t.TempDir()
	paths := testLinuxDNSPaths(t, dir)

	managed := "# Managed by uBlockDNS — do not edit\nnameserver 127.0.0.1\n"
	if err := os.WriteFile(paths.ResolvConf, []byte(managed), 0o644); err != nil {
		t.Fatalf("write resolv.conf: %v", err)
	}

	if err := cleanupManagedResolvConfIfNeeded(paths); err != nil {
		t.Fatalf("cleanupManagedResolvConfIfNeeded() error = %v", err)
	}
	if _, err := os.Stat(paths.ResolvConf); !os.IsNotExist(err) {
		t.Fatalf("expected managed resolv.conf removed, stat err = %v", err)
	}
}

func TestRestoreDhclientConfig(t *testing.T) {
	dir := t.TempDir()
	paths := testLinuxDNSPaths(t, dir)

	original := "option rfc3442-classless-static-routes code 121 = array of unsigned integer 8;\n"
	managed := original + "\n# uBlockDNS\nsupersede domain-name-servers 127.0.0.1;\n"
	path := paths.DhclientConfs[0]
	if err := os.WriteFile(path, []byte(managed), 0o644); err != nil {
		t.Fatalf("write dhclient.conf: %v", err)
	}

	if err := restoreDhclientConfig(paths); err != nil {
		t.Fatalf("restoreDhclientConfig() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dhclient.conf: %v", err)
	}
	if trimLine(string(got)) != trimLine(original) {
		t.Fatalf("dhclient.conf = %q, want %q", got, original)
	}
}

func TestRestorePlatformInstallArtifactsRemovesManagedFiles(t *testing.T) {
	stubLinuxDNSCommands(t)
	dir := t.TempDir()
	paths := testLinuxDNSPaths(t, dir)

	if err := os.MkdirAll(filepath.Dir(paths.NMUBlockDNSConf), 0o755); err != nil {
		t.Fatalf("mkdir nm: %v", err)
	}
	if err := os.WriteFile(paths.NMUBlockDNSConf, []byte("[main]\ndns=none\n"), 0o644); err != nil {
		t.Fatalf("write nm conf: %v", err)
	}
	if err := os.WriteFile(paths.NMNextDNSConf, []byte("[main]\ndns=none\n"), 0o644); err != nil {
		t.Fatalf("write nextdns nm conf: %v", err)
	}
	if err := os.WriteFile(paths.ResolvConf, []byte("# Managed by uBlockDNS\nnameserver 127.0.0.1\n"), 0o644); err != nil {
		t.Fatalf("write resolv.conf: %v", err)
	}

	if err := restorePlatformInstallArtifacts(); err != nil {
		t.Fatalf("restorePlatformInstallArtifacts() error = %v", err)
	}
	if _, err := os.Stat(paths.NMUBlockDNSConf); !os.IsNotExist(err) {
		t.Fatalf("expected ublockdns nm conf removed, stat err = %v", err)
	}
	if _, err := os.Stat(paths.NMNextDNSConf); err != nil {
		t.Fatalf("expected standalone nextdns nm conf preserved: %v", err)
	}
	if _, err := os.Stat(paths.ResolvConf); !os.IsNotExist(err) {
		t.Fatalf("expected managed resolv.conf removed, stat err = %v", err)
	}
}

func TestRestoreConnmanConfigPreservesPreExistingDNSProxy(t *testing.T) {
	dir := t.TempDir()
	paths := testLinuxDNSPaths(t, dir)

	original := "[General]\nDNSProxy=none\nOtherSetting=1\n"
	if err := os.MkdirAll(filepath.Dir(paths.ConnmanMainConf), 0o755); err != nil {
		t.Fatalf("mkdir connman: %v", err)
	}
	if err := os.WriteFile(paths.ConnmanMainConf, []byte(original), 0o644); err != nil {
		t.Fatalf("write connman conf: %v", err)
	}

	if err := configureConnman(paths); err != nil {
		t.Fatalf("configureConnman() error = %v", err)
	}

	changed, err := restoreConnmanConfig(paths)
	if err != nil {
		t.Fatalf("restoreConnmanConfig() error = %v", err)
	}
	if changed {
		t.Fatal("expected no connman changes when DNSProxy=none pre-existed")
	}

	got, err := os.ReadFile(paths.ConnmanMainConf)
	if err != nil {
		t.Fatalf("read connman conf: %v", err)
	}
	if string(got) != original {
		t.Fatalf("connman conf = %q, want %q", got, original)
	}
}

func TestRestoreConnmanConfigRestoresBackup(t *testing.T) {
	dir := t.TempDir()
	paths := testLinuxDNSPaths(t, dir)

	original := "[General]\nOtherSetting=1\n"
	if err := os.MkdirAll(filepath.Dir(paths.ConnmanMainConf), 0o755); err != nil {
		t.Fatalf("mkdir connman: %v", err)
	}
	if err := os.WriteFile(paths.ConnmanMainConf, []byte(original), 0o644); err != nil {
		t.Fatalf("write connman conf: %v", err)
	}

	if err := configureConnman(paths); err != nil {
		t.Fatalf("configureConnman() error = %v", err)
	}

	changed, err := restoreConnmanConfig(paths)
	if err != nil {
		t.Fatalf("restoreConnmanConfig() error = %v", err)
	}
	if !changed {
		t.Fatal("expected connman restore to apply")
	}

	got, err := os.ReadFile(paths.ConnmanMainConf)
	if err != nil {
		t.Fatalf("read connman conf: %v", err)
	}
	if string(got) != original {
		t.Fatalf("connman conf = %q, want %q", got, original)
	}
}

func TestConfigureAndRestoreRoundTrip(t *testing.T) {
	stubLinuxDNSCommands(t)
	dir := t.TempDir()
	paths := testLinuxDNSPaths(t, dir)

	original := []byte("nameserver 192.168.50.1\n")
	if err := os.WriteFile(paths.ResolvConf, original, 0o644); err != nil {
		t.Fatalf("write original resolv.conf: %v", err)
	}

	if err := ConfigureLinuxSystemDNS(); err != nil {
		t.Fatalf("ConfigureLinuxSystemDNS() error = %v", err)
	}

	got, err := os.ReadFile(paths.ResolvConf)
	if err != nil {
		t.Fatalf("read configured resolv.conf: %v", err)
	}
	if !containsLine(got, "nameserver 127.0.0.1") {
		t.Fatalf("configured resolv.conf = %q", got)
	}

	if err := restorePlatformInstallArtifacts(); err != nil {
		t.Fatalf("restorePlatformInstallArtifacts() error = %v", err)
	}

	restored, err := os.ReadFile(paths.ResolvConf)
	if err != nil {
		t.Fatalf("read restored resolv.conf: %v", err)
	}
	if string(restored) != string(original) {
		t.Fatalf("restored resolv.conf = %q, want %q", restored, original)
	}
}

func TestRestoreResolvConfFromSymlinkBackup(t *testing.T) {
	dir := t.TempDir()
	paths := testLinuxDNSPaths(t, dir)

	target := "/run/systemd/resolve/stub-resolv.conf"
	if err := os.Symlink(target, paths.ResolvConf); err != nil {
		t.Fatalf("symlink resolv.conf: %v", err)
	}
	if err := backupResolvConf(paths); err != nil {
		t.Fatalf("backupResolvConf() error = %v", err)
	}

	_ = os.Remove(paths.ResolvConf)
	restored, err := restoreResolvConfFromUBlockDNSBackup(paths)
	if err != nil {
		t.Fatalf("restoreResolvConfFromUBlockDNSBackup() error = %v", err)
	}
	if !restored {
		t.Fatal("expected symlink backup restore")
	}
	link, err := os.Readlink(paths.ResolvConf)
	if err != nil {
		t.Fatalf("read restored symlink: %v", err)
	}
	if link != target {
		t.Fatalf("restored symlink target = %q, want %q", link, target)
	}
}
