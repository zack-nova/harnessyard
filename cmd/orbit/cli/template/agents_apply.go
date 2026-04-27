package orbittemplate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

func renderSharedAgentsPayloadWithOptions(source LocalTemplateSource, bindings map[string]string, options renderTemplateOptions) (CandidateFile, bool, error) {
	body := orbitAgentsBody(source.Spec)
	if len(body) == 0 {
		return CandidateFile{}, false, nil
	}

	rendered, err := renderTemplateFilesWithOptions([]CandidateFile{{
		Path:    sharedFilePathAgents,
		Content: body,
		Mode:    gitpkg.FileModeRegular,
	}}, bindings, options)
	if err != nil {
		return CandidateFile{}, false, err
	}

	return rendered[0], true, nil
}

func analyzeSharedAgentsApply(
	repoRoot string,
	orbitID string,
	statusByPath map[string]gitpkg.StatusEntry,
) ([]ApplyConflict, []string, error) {
	filename := filepath.Join(repoRoot, sharedFilePathAgents)
	//nolint:gosec // The runtime AGENTS path is fixed under the repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	document, err := ParseRuntimeAgentsDocument(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse runtime AGENTS.md: %w", err)
	}

	conflicts := make([]ApplyConflict, 0, 1)
	if status, ok := statusByPath[sharedFilePathAgents]; ok {
		conflicts = append(conflicts, ApplyConflict{
			Path:    sharedFilePathAgents,
			Message: fmt.Sprintf("target path has uncommitted worktree status %s", status.Code),
		})
	}

	for _, segment := range document.Segments {
		if segment.Kind == AgentsRuntimeSegmentBlock && segment.OrbitID == orbitID {
			return conflicts, []string{
				fmt.Sprintf(`runtime AGENTS.md already contains orbit block %q; apply will replace it in place`, orbitID),
			}, nil
		}
	}

	return conflicts, nil, nil
}

func applySharedAgentsPayload(repoRoot string, orbitID string, payload []byte) error {
	wrappedBlock, err := WrapRuntimeAgentsBlock(orbitID, payload)
	if err != nil {
		return fmt.Errorf("wrap shared AGENTS payload: %w", err)
	}

	filename := filepath.Join(repoRoot, sharedFilePathAgents)
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

	merged, err := replaceOrAppendRuntimeAgentsBlock(existing, orbitID, wrappedBlock)
	if err != nil {
		return fmt.Errorf("merge runtime AGENTS.md: %w", err)
	}
	if err := contractutil.AtomicWriteFileMode(filename, merged, 0o644); err != nil {
		return fmt.Errorf("write runtime AGENTS.md: %w", err)
	}

	return nil
}

func removeRuntimeAgentsBlock(repoRoot string, orbitID string) error {
	filename := filepath.Join(repoRoot, sharedFilePathAgents)
	//nolint:gosec // The runtime AGENTS path is fixed under the repo root.
	existing, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	updated, removed, err := RemoveRuntimeGuidanceBlockData(existing, orbitID, "runtime AGENTS.md")
	if err != nil {
		return fmt.Errorf("remove runtime AGENTS block: %w", err)
	}
	if !removed {
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

func replaceOrAppendRuntimeAgentsBlock(existing []byte, orbitID string, wrappedBlock []byte) ([]byte, error) {
	if _, err := ParseRuntimeAgentsDocument(existing); err != nil {
		return nil, fmt.Errorf("parse runtime AGENTS.md: %w", err)
	}

	lines := splitLinesPreserveNewline(existing)
	var output bytes.Buffer
	replacing := false
	replaced := false

	for _, line := range lines {
		markerKind, markerOrbitID, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return nil, err
		}

		if replacing {
			if isMarker && markerKind == "end" && markerOrbitID == orbitID {
				output.Write(wrappedBlock)
				replacing = false
				replaced = true
			}
			continue
		}

		if isMarker && markerKind == "begin" && markerOrbitID == orbitID {
			replacing = true
			continue
		}

		output.Write(line)
	}

	if replacing {
		return nil, fmt.Errorf("unterminated orbit block for %q", orbitID)
	}
	if replaced {
		return output.Bytes(), nil
	}

	merged := append([]byte(nil), existing...)
	if len(merged) > 0 {
		if merged[len(merged)-1] != '\n' {
			merged = append(merged, '\n')
		}
		if !bytes.HasSuffix(merged, []byte("\n\n")) {
			merged = append(merged, '\n')
		}
	}
	merged = append(merged, wrappedBlock...)

	return merged, nil
}

// ReplaceOrAppendRuntimeAgentsBlockData replaces one existing runtime AGENTS block in place
// or appends a new block when the identity is not present.
func ReplaceOrAppendRuntimeAgentsBlockData(existing []byte, blockID string, wrappedBlock []byte) ([]byte, error) {
	return replaceOrAppendRuntimeAgentsBlock(existing, blockID, wrappedBlock)
}

// RemoveRuntimeAgentsBlockData removes one runtime AGENTS block by identity from raw document data.
func RemoveRuntimeAgentsBlockData(existing []byte, blockID string) ([]byte, bool, error) {
	return RemoveRuntimeGuidanceBlockData(existing, blockID, "runtime AGENTS.md")
}
