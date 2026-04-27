package scoped

import (
	"fmt"
	"strings"
)

// BuildCommitMessage appends the Orbit trailer when configured.
func BuildCommitMessage(message string, orbitID string, appendTrailer bool) string {
	trimmed := strings.TrimSpace(message)
	if !appendTrailer {
		return trimmed
	}

	return trimmed + "\n\nOrbit: " + orbitID
}

// BuildRestoreCommitMessage builds the documented restore commit message and trailers.
func BuildRestoreCommitMessage(orbitID string, revision string) string {
	return fmt.Sprintf(
		"restore %s orbit to %s\n\nOrbit: %s\nOrbit-Restore-From: %s",
		orbitID,
		revision,
		orbitID,
		revision,
	)
}
