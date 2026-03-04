package app

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

func flushDNSCaches() error {
	type cmdSpec struct {
		name string
		args []string
	}

	var commands []cmdSpec
	switch runtime.GOOS {
	case "darwin":
		commands = []cmdSpec{
			{name: "dscacheutil", args: []string{"-flushcache"}},
			{name: "killall", args: []string{"-HUP", "mDNSResponder"}},
		}
	case "linux":
		commands = []cmdSpec{
			{name: "resolvectl", args: []string{"flush-caches"}},
			{name: "systemd-resolve", args: []string{"--flush-caches"}},
			{name: "service", args: []string{"nscd", "restart"}},
		}
	case "windows":
		commands = []cmdSpec{
			{name: "ipconfig", args: []string{"/flushdns"}},
		}
	default:
		return nil
	}

	var errs []string
	success := false
	for _, c := range commands {
		if err := runCommand(c.name, c.args...); err != nil {
			errs = append(errs, fmt.Sprintf("%s %s: %v", c.name, strings.Join(c.args, " "), err))
			continue
		}
		success = true
	}
	if success {
		return nil
	}
	if len(errs) == 0 {
		return errors.New("no cache flush command available")
	}
	return errors.New(strings.Join(errs, "; "))
}
