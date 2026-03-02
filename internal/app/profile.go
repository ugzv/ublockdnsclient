package app

import (
	"fmt"
	"regexp"
	"strings"
)

var profileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func ValidateProfileID(profileID string) error {
	id := strings.TrimSpace(profileID)
	if id == "" {
		return fmt.Errorf("profile id is required")
	}
	if strings.HasPrefix(id, "-") {
		return fmt.Errorf("invalid profile id %q: profile id cannot start with '-'", profileID)
	}
	if !profileIDPattern.MatchString(id) {
		return fmt.Errorf("invalid profile id %q: only letters, digits, '-' and '_' are allowed", profileID)
	}
	return nil
}
