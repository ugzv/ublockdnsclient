package core

import "os/exec"

func RunCommand(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func CommandOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func CommandCombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
