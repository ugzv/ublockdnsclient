//go:build linux

package core

import "strings"

func trimLine(line string) string {
	return strings.TrimSpace(line)
}

func containsLine(data []byte, want string) bool {
	for _, line := range splitLines(data) {
		if trimLine(line) == want {
			return true
		}
	}
	return false
}

func splitLines(data []byte) []string {
	text := string(data)
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
