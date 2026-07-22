package core

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

var runCommandFunc = func(name string, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return nil
	}
	desc := strings.Join(append([]string{name}, args...), " ")
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return fmt.Errorf("%s: %w: %s", desc, err, msg)
	}
	return fmt.Errorf("%s: %w", desc, err)
}

func RunCommand(name string, args ...string) error {
	return runCommandFunc(name, args...)
}

func CommandOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func CommandCombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// SwapCommandRunner overrides command execution for tests and returns a restore func.
func SwapCommandRunner(run func(string, ...string) error) func() {
	oldRun := runCommandFunc
	if run != nil {
		runCommandFunc = run
	}
	return func() {
		runCommandFunc = oldRun
	}
}
