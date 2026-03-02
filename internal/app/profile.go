package app

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var profileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func NormalizeProfileIDInput(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", fmt.Errorf("profile id is required")
	}

	id := raw
	if maybeURL, err := url.Parse(raw); err == nil &&
		(maybeURL.Scheme == "http" || maybeURL.Scheme == "https") &&
		maybeURL.Host != "" {
		trimmedPath := strings.Trim(maybeURL.Path, "/")
		if trimmedPath == "" {
			return "", fmt.Errorf("invalid profile URL %q: missing profile id path segment", input)
		}
		parts := strings.Split(trimmedPath, "/")
		id = parts[len(parts)-1]
	}

	if err := ValidateProfileID(id); err != nil {
		return "", err
	}
	return id, nil
}

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
