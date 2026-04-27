package ids

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

const maxOrbitIDLength = 64

var (
	orbitIDPattern      = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]*[a-z0-9])?$`)
	windowsDrivePattern = regexp.MustCompile(`^[A-Za-z]:/`)
)

// ValidateOrbitID checks whether an orbit identifier is safe for filenames and refs.
func ValidateOrbitID(id string) error {
	switch {
	case id == "":
		return errors.New("orbit id must not be empty")
	case len(id) > maxOrbitIDLength:
		return fmt.Errorf("orbit id must be at most %d characters", maxOrbitIDLength)
	case strings.TrimSpace(id) != id:
		return errors.New("orbit id must not contain leading or trailing whitespace")
	case !orbitIDPattern.MatchString(id):
		return errors.New("orbit id must use lowercase letters, digits, hyphens, or underscores, and must start and end with an alphanumeric character")
	default:
		return nil
	}
}

// NormalizeRepoRelativePath cleans a repository-relative path and rejects escapes.
func NormalizeRepoRelativePath(value string) (string, error) {
	if value == "" {
		return "", errors.New("path must not be empty")
	}
	if strings.ContainsRune(value, '\x00') {
		return "", errors.New("path must not contain NUL bytes")
	}

	normalized := strings.ReplaceAll(value, `\`, `/`)
	if strings.TrimSpace(normalized) == "" {
		return "", errors.New("path must not be blank")
	}
	if path.IsAbs(normalized) || windowsDrivePattern.MatchString(normalized) {
		return "", errors.New("path must be repository-relative")
	}

	cleaned := path.Clean(normalized)
	switch cleaned {
	case "", ".":
		return "", errors.New("path must not resolve to repository root")
	case "..":
		return "", errors.New("path must not escape repository root")
	}
	if strings.HasPrefix(cleaned, "../") {
		return "", errors.New("path must not escape repository root")
	}
	if strings.HasPrefix(cleaned, "/") {
		return "", errors.New("path must be repository-relative")
	}

	return cleaned, nil
}
