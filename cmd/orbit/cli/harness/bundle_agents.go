package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// ApplyBundleAgentsPayload appends or replaces one harness bundle block in runtime AGENTS.md.
func ApplyBundleAgentsPayload(repoRoot string, harnessID string, payload []byte) error {
	wrappedBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindHarness, harnessID, payload)
	if err != nil {
		return fmt.Errorf("wrap bundle AGENTS payload: %w", err)
	}

	filename := filepath.Join(repoRoot, rootAgentsPath)
	//nolint:gosec // The runtime AGENTS path is fixed under the repo root.
	existing, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := contractutil.AtomicWriteFileMode(filename, wrappedBlock, 0o644); err != nil {
				return fmt.Errorf("write runtime AGENTS.md: %w", err)
			}
			return nil
		}
		return fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	merged, err := orbittemplate.ReplaceOrAppendRuntimeAgentsOwnerBlockData(existing, orbittemplate.OwnerKindHarness, harnessID, wrappedBlock)
	if err != nil {
		return fmt.Errorf("merge runtime AGENTS.md: %w", err)
	}
	if err := contractutil.AtomicWriteFileMode(filename, merged, 0o644); err != nil {
		return fmt.Errorf("write runtime AGENTS.md: %w", err)
	}

	return nil
}

// RemoveBundleAgentsPayload removes one harness bundle block from runtime AGENTS.md.
func RemoveBundleAgentsPayload(repoRoot string, harnessID string) error {
	var record *BundleRecord
	if loaded, err := LoadBundleRecord(repoRoot, harnessID); err == nil {
		record = &loaded
	}
	return removeBundleAgentsPayload(repoRoot, harnessID, record)
}

// RemoveBundleAgentsPayloadForRecord removes one harness bundle block using an already-loaded
// bundle record for ownership checks.
func RemoveBundleAgentsPayloadForRecord(repoRoot string, record BundleRecord) error {
	if record.HarnessID == "" {
		return fmt.Errorf("bundle record harness_id must not be empty")
	}
	return removeBundleAgentsPayload(repoRoot, record.HarnessID, &record)
}

func removeBundleAgentsPayload(repoRoot string, harnessID string, record *BundleRecord) error {
	filename := filepath.Join(repoRoot, rootAgentsPath)
	//nolint:gosec // The runtime AGENTS path is fixed under the repo root.
	existing, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	updated, removed, err := orbittemplate.RemoveRuntimeAgentsOwnerBlockData(existing, orbittemplate.OwnerKindHarness, harnessID)
	if err != nil {
		return fmt.Errorf("remove runtime AGENTS block: %w", err)
	}
	if !removed {
		if record != nil &&
			record.RootAgentsDigest != "" &&
			contentDigest(existing) == record.RootAgentsDigest {
			if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("delete runtime AGENTS.md: %w", err)
			}
		}
		return nil
	}
	if len(updated) == 0 {
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete runtime AGENTS.md: %w", err)
		}
		return nil
	}
	if err := contractutil.AtomicWriteFileMode(filename, updated, 0o644); err != nil {
		return fmt.Errorf("write runtime AGENTS.md: %w", err)
	}

	return nil
}
