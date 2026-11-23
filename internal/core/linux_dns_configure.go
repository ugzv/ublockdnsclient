//go:build linux

package core

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// ConfigureLinuxSystemDNS applies durable Linux DNS settings for new installs.
func ConfigureLinuxSystemDNS() error {
	paths := linuxDNSPathsActive()

	if err := backupResolvConf(paths); err != nil {
		return fmt.Errorf("backup resolv.conf: %w", err)
	}

	manager := detectLinuxDNSManager()
	changed, err := applyLinuxDNSManagerConfigure(paths, manager)
	if err != nil {
		return err
	}
	if err := writeManagedResolvConf(paths); err != nil {
		return err
	}
	if changed {
		reloadLinuxDNSManagerAfterConfigure(manager)
	}
	return nil
}

// applyLinuxDNSManagerConfigure writes the manager's config and reports whether
// it changed. This runs on every network change, so an unchanged config must
// not bounce the manager: bouncing it is itself a network change.
func applyLinuxDNSManagerConfigure(paths linuxDNSPaths, manager string) (bool, error) {
	switch manager {
	case "systemd-resolved":
		changed, err := writeResolvedDropIn(paths)
		if err != nil || !changed {
			return false, err
		}
		return false, RunCommand("systemctl", "restart", "systemd-resolved")
	case "networkmanager":
		return writeNetworkManagerDropIn(paths)
	case "connman":
		return configureConnman(paths)
	case "dhclient":
		return false, configureDhclient(paths)
	case "resolvconf":
		return false, configureResolvconf(paths)
	default:
		return false, nil
	}
}

func reloadLinuxDNSManagerAfterConfigure(manager string) {
	switch manager {
	case "networkmanager":
		// A reload applies dns=none without dropping the connection.
		if err := RunCommand("systemctl", "reload", "NetworkManager"); err != nil {
			log.Printf("Warning: reload NetworkManager, restarting instead: %v", err)
			if err := RunCommand("systemctl", "restart", "NetworkManager"); err != nil {
				log.Printf("Warning: restart NetworkManager: %v", err)
			}
		}
	case "connman":
		if err := RunCommand("systemctl", "restart", "connman"); err != nil {
			log.Printf("Warning: restart connman: %v", err)
		}
	}
}

// writeFileIfChanged writes content and reports whether the file differed.
func writeFileIfChanged(path string, content []byte) (bool, error) {
	if data, err := os.ReadFile(path); err == nil && bytes.Equal(data, content) {
		return false, nil
	}
	return true, os.WriteFile(path, content, 0o644)
}

func detectLinuxDNSManager() string {
	if systemctlActive("systemd-resolved") {
		return "systemd-resolved"
	}
	if systemctlActive("NetworkManager") {
		return "networkmanager"
	}
	if systemctlActive("connman") {
		return "connman"
	}
	paths := linuxDNSPathsActive()
	for _, path := range paths.DhclientConfs {
		if fileExists(path) {
			return "dhclient"
		}
	}
	if fileExists("/etc/dhclient.d") {
		return "dhclient"
	}
	if _, err := exec.LookPath("resolvconf"); err == nil {
		return "resolvconf"
	}
	return "none"
}

func systemctlActive(unit string) bool {
	return RunCommand("systemctl", "is-active", "--quiet", unit) == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func backupResolvConf(paths linuxDNSPaths) error {
	if _, err := os.Stat(paths.ResolvUBlockDNSBak); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	info, err := os.Lstat(paths.ResolvConf)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(paths.ResolvConf)
		if err != nil {
			return err
		}
		backup := []byte("symlink:" + target + "\n")
		return os.WriteFile(paths.ResolvUBlockDNSBak, backup, 0o644)
	}

	data, err := os.ReadFile(paths.ResolvConf)
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ResolvUBlockDNSBak, data, 0o644)
}

func writeManagedResolvConf(paths linuxDNSPaths) error {
	content := "# " + managedResolvMarker + " — do not edit\nnameserver 127.0.0.1\n"

	info, err := os.Lstat(paths.ResolvConf)
	isSymlink := err == nil && info.Mode()&os.ModeSymlink != 0
	if !isSymlink {
		if data, err := os.ReadFile(paths.ResolvConf); err == nil && string(data) == content {
			return nil
		}
	}

	unlockResolvConf(paths)
	if isSymlink {
		if err := os.Remove(paths.ResolvConf); err != nil {
			return err
		}
	}

	if err := os.WriteFile(paths.ResolvConf, []byte(content), 0o644); err != nil {
		return err
	}

	if _, err := exec.LookPath("chattr"); err == nil {
		_ = RunCommand("chattr", "+i", paths.ResolvConf)
	}
	return nil
}

func writeResolvedDropIn(paths linuxDNSPaths) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(paths.ResolvedDropIn), 0o755); err != nil {
		return false, err
	}
	// Domains=~. routes every lookup to the proxy. Without it, programs that
	// resolve through systemd-resolved also ask the link's own DNS server and
	// blocked names leak through.
	content := "[Resolve]\nDNS=127.0.0.1\nDomains=~.\nDNSStubListener=no\n"
	return writeFileIfChanged(paths.ResolvedDropIn, []byte(content))
}

func writeNetworkManagerDropIn(paths linuxDNSPaths) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(paths.NMUBlockDNSConf), 0o755); err != nil {
		return false, err
	}
	return writeFileIfChanged(paths.NMUBlockDNSConf, []byte("[main]\ndns=none\n"))
}

// configureConnman reports whether main.conf changed.
func configureConnman(paths linuxDNSPaths) (bool, error) {
	content := "[General]\n" + managedConfigMarker + "\nDNSProxy=none\n"
	data, err := os.ReadFile(paths.ConnmanMainConf)
	if err != nil {
		if os.IsNotExist(err) {
			return true, os.WriteFile(paths.ConnmanMainConf, []byte(content), 0o644)
		}
		return false, err
	}
	if string(data) == "" {
		return true, os.WriteFile(paths.ConnmanMainConf, []byte(content), 0o644)
	}
	if containsLine(data, "DNSProxy=none") {
		return false, nil
	}
	if err := backupConnmanMainConf(paths); err != nil {
		return false, err
	}
	if containsLine(data, "[General]") {
		lines := splitLines(data)
		var out []string
		inserted := false
		for _, line := range lines {
			out = append(out, line)
			if !inserted && trimLine(line) == "[General]" {
				out = append(out, managedConfigMarker, "DNSProxy=none")
				inserted = true
			}
		}
		if !inserted {
			out = append(out, managedConfigMarker, "DNSProxy=none")
		}
		return true, os.WriteFile(paths.ConnmanMainConf, []byte(joinLines(out)), 0o644)
	}
	return true, os.WriteFile(paths.ConnmanMainConf, append(append(data, '\n'), []byte("[General]\n"+managedConfigMarker+"\nDNSProxy=none\n")...), 0o644)
}

func backupConnmanMainConf(paths linuxDNSPaths) error {
	if _, err := os.Stat(paths.ConnmanMainConfBak); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	data, err := os.ReadFile(paths.ConnmanMainConf)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(paths.ConnmanMainConfBak, data, 0o644)
}

func configureDhclient(paths linuxDNSPaths) error {
	for _, path := range paths.DhclientConfs {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if containsLine(data, "supersede domain-name-servers 127.0.0.1;") {
			continue
		}
		block := "\n" + managedConfigMarker + "\nsupersede domain-name-servers 127.0.0.1;\n"
		if err := os.WriteFile(path, append(data, block...), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func configureResolvconf(paths linuxDNSPaths) error {
	if err := os.MkdirAll("/etc/resolvconf/resolv.conf.d", 0o755); err != nil {
		return err
	}
	if err := backupResolvconfHead(paths); err != nil {
		return err
	}
	if err := os.WriteFile(paths.ResolvconfHead, []byte("nameserver 127.0.0.1\n"), 0o644); err != nil {
		return err
	}
	_ = RunCommand("resolvconf", "-u")
	return nil
}

func backupResolvconfHead(paths linuxDNSPaths) error {
	if _, err := os.Stat(paths.ResolvconfHeadBak); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	data, err := os.ReadFile(paths.ResolvconfHead)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if trimLine(string(data)) == "nameserver 127.0.0.1" {
		return nil
	}
	return os.WriteFile(paths.ResolvconfHeadBak, data, 0o644)
}
