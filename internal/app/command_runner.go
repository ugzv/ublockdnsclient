package app

import "os/exec"

func runCommand(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func commandOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func commandCombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
