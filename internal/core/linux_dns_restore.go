//go:build linux

package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func restorePlatformInstallArtifacts() error {
	paths := linuxDNSPathsActive()
	var errs []error

	appendErr := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	appendErr(unlockResolvConf(paths))

	restoredFromUBlock, err := restoreResolvConfFromUBlockDNSBackup(paths)
	appendErr(err)

	removedResolved, err := removeManagedFile(paths.ResolvedDropIn)
	appendErr(err)
	removedNM, err := removeManagedFile(paths.NMUBlockDNSConf)
	appendErr(err)

	connmanChanged, err := restoreConnmanConfig(paths)
	appendErr(err)
	appendErr(restoreDhclientConfig(paths))
	appendErr(restoreResolvconfHead(paths))

	if restoredFromUBlock {
		_ = os.Remove(paths.ResolvNextDNSBak)
	} else {
		appendErr(cleanupManagedResolvConfIfNeeded(paths))
	}

	appendErr(restartLinuxDNSManagers(restartLinuxDNSOptions{
		restartResolved:      removedResolved,
		reloadNetworkManager: removedNM,
		restartConnman:       connmanChanged,
	}))

	return errors.Join(errs...)
}

type restartLinuxDNSOptions struct {
	restartResolved      bool
	reloadNetworkManager bool
	restartConnman       bool
}

func unlockResolvConf(paths linuxDNSPaths) error {
	if _, err := exec.LookPath("chattr"); err != nil {
		return nil
	}
	return RunCommand("chattr", "-i", paths.ResolvConf)
}

func restoreResolvConfFromUBlockDNSBackup(paths linuxDNSPaths) (bool, error) {
	if _, err := os.Stat(paths.ResolvUBlockDNSBak); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	data, err := os.ReadFile(paths.ResolvUBlockDNSBak)
	if err != nil {
		return false, err
	}

	_ = os.Remove(paths.ResolvConf)
	if bytes.HasPrefix(data, []byte("symlink:")) {
		target := strings.TrimSpace(strings.TrimPrefix(string(data), "symlink:"))
		if target == "" {
			return false, fmt.Errorf("invalid symlink backup")
		}
		if err := os.Symlink(target, paths.ResolvConf); err != nil {
			return false, err
		}
	} else if err := os.WriteFile(paths.ResolvConf, data, 0o644); err != nil {
		return false, err
	}
	_ = os.Remove(paths.ResolvUBlockDNSBak)
	return true, nil
}

func cleanupManagedResolvConfIfNeeded(paths linuxDNSPaths) error {
	data, err := os.ReadFile(paths.ResolvConf)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !strings.Contains(string(data), managedResolvMarker) {
		return nil
	}
	_ = unlockResolvConf(paths)
	if err := os.Remove(paths.ResolvConf); err != nil && !os.IsNotExist(err) {
		return err
	}
	return restoreStandardResolvConf(paths)
}

func restoreStandardResolvConf(paths linuxDNSPaths) error {
	const stubTarget = "/run/systemd/resolve/stub-resolv.conf"
	if !systemctlActive("systemd-resolved") {
		return nil
	}
	if _, err := os.Stat(stubTarget); err != nil {
		return nil
	}
	if _, err := os.Lstat(paths.ResolvConf); err == nil {
		return nil
	}
	return os.Symlink(stubTarget, paths.ResolvConf)
}

func removeManagedFile(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, os.Remove(path)
}

func restoreConnmanConfig(paths linuxDNSPaths) (bool, error) {
	if _, err := os.Stat(paths.ConnmanMainConfBak); err == nil {
		data, err := os.ReadFile(paths.ConnmanMainConfBak)
		if err != nil {
			return false, err
		}
		if len(data) == 0 {
			if err := os.Remove(paths.ConnmanMainConf); err != nil && !os.IsNotExist(err) {
				return false, err
			}
		} else if err := os.WriteFile(paths.ConnmanMainConf, data, 0o644); err != nil {
			return false, err
		}
		_ = os.Remove(paths.ConnmanMainConfBak)
		return true, nil
	} else if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	data, err := os.ReadFile(paths.ConnmanMainConf)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !containsLine(data, managedConfigMarker) {
		if string(data) == "[General]\nDNSProxy=none\n" {
			return true, os.Remove(paths.ConnmanMainConf)
		}
		return false, nil
	}

	lines := splitLines(data)
	var out []string
	changed := false
	skipDNSProxy := false
	for _, line := range lines {
		trimmed := trimLine(line)
		if trimmed == managedConfigMarker {
			skipDNSProxy = true
			changed = true
			continue
		}
		if skipDNSProxy && trimmed == "DNSProxy=none" {
			skipDNSProxy = false
			continue
		}
		skipDNSProxy = false
		out = append(out, line)
	}
	if !changed {
		return false, nil
	}
	content := joinLines(out)
	if content == "" {
		return true, os.Remove(paths.ConnmanMainConf)
	}
	return true, os.WriteFile(paths.ConnmanMainConf, []byte(content), 0o644)
}

func restoreDhclientConfig(paths linuxDNSPaths) error {
	const supersedeLine = "supersede domain-name-servers 127.0.0.1;"

	for _, path := range paths.DhclientConfs {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !containsLine(data, managedConfigMarker) {
			continue
		}

		var out []string
		skipSupersede := false
		for _, line := range splitLines(data) {
			trimmed := trimLine(line)
			if trimmed == managedConfigMarker {
				skipSupersede = true
				continue
			}
			if skipSupersede && trimmed == supersedeLine {
				skipSupersede = false
				continue
			}
			skipSupersede = false
			out = append(out, line)
		}
		if err := os.WriteFile(path, []byte(joinLines(out)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func restoreResolvconfHead(paths linuxDNSPaths) error {
	if _, err := os.Stat(paths.ResolvconfHeadBak); err == nil {
		data, err := os.ReadFile(paths.ResolvconfHeadBak)
		if err != nil {
			return err
		}
		if err := os.WriteFile(paths.ResolvconfHead, data, 0o644); err != nil {
			return err
		}
		_ = os.Remove(paths.ResolvconfHeadBak)
		_ = RunCommand("resolvconf", "-u")
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
	if trimLine(string(data)) != "nameserver 127.0.0.1" {
		return nil
	}
	if err := os.Remove(paths.ResolvconfHead); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = RunCommand("resolvconf", "-u")
	return nil
}

func restartLinuxDNSManagers(opts restartLinuxDNSOptions) error {
	var errs []error
	if opts.restartResolved {
		if err := RunCommand("systemctl", "restart", "systemd-resolved"); err != nil {
			errs = append(errs, err)
		}
	}
	if opts.reloadNetworkManager {
		if err := RunCommand("systemctl", "reload", "NetworkManager"); err != nil {
			errs = append(errs, err)
		}
	}
	if opts.restartConnman {
		if err := RunCommand("systemctl", "restart", "connman"); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
