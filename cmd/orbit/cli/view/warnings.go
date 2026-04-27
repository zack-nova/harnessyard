package view

import (
	"fmt"
	"time"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// WriteWarnings stores the latest warning summary for orbit-aware commands.
func WriteWarnings(store statepkg.FSStore, currentOrbit string, messages []string) error {
	snapshot := statepkg.WarningSnapshot{
		CurrentOrbit: currentOrbit,
		Messages:     append([]string(nil), messages...),
		CreatedAt:    time.Now().UTC(),
	}

	if err := store.WriteWarnings(snapshot); err != nil {
		return fmt.Errorf("write warning snapshot: %w", err)
	}

	return nil
}
