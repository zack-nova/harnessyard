package orbittemplate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ValidateRuntimeAgentsFile validates the runtime AGENTS.md marker contract when Orbit markers are present.
func ValidateRuntimeAgentsFile(repoRoot string) error {
	filename := filepath.Join(repoRoot, sharedFilePathAgents)
	//nolint:gosec // The runtime AGENTS path is fixed under the repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read runtime AGENTS.md: %w", err)
	}
	if _, err := ParseRuntimeAgentsDocument(data); err != nil {
		return fmt.Errorf("parse runtime AGENTS.md: %w", err)
	}

	return nil
}
