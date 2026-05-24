package core

import "os/exec"

var runCommandFunc = func(name string, args ...string) error {
	return exec.Command(name, args...).Run()
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
