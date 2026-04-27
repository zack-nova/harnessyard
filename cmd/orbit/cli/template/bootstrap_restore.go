package orbittemplate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// BootstrapArtifactRestoreInput captures one explicit runtime BOOTSTRAP artifact restore request.
type BootstrapArtifactRestoreInput struct {
	RepoRoot string
	OrbitID  string
}

// BootstrapArtifactRestoreResult reports one successful runtime BOOTSTRAP artifact restore pass.
type BootstrapArtifactRestoreResult struct {
	OrbitID       string
	BootstrapPath string
	Changed       bool
	Created       bool
}

// RestoreOrbitBootstrapArtifact restores one orbit-owned BOOTSTRAP.md block from authored truth
// without consulting the runtime completion gate. Callers should perform their own reopen gating first.
func RestoreOrbitBootstrapArtifact(ctx context.Context, input BootstrapArtifactRestoreInput) (BootstrapArtifactRestoreResult, error) {
	if input.RepoRoot == "" {
		return BootstrapArtifactRestoreResult{}, errors.New("repo root must not be empty")
	}

	revisionKind, err := resolveAllowedBriefRevisionKind(input.RepoRoot, "materialize")
	if err != nil {
		return BootstrapArtifactRestoreResult{}, err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, input.RepoRoot, input.OrbitID)
	if err != nil {
		return BootstrapArtifactRestoreResult{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}

	payload, hasTruth, err := materializedOrbitBootstrapPayload(spec, input.RepoRoot, revisionKind)
	if err != nil {
		return BootstrapArtifactRestoreResult{}, err
	}
	if !hasTruth {
		return BootstrapArtifactRestoreResult{
			OrbitID:       input.OrbitID,
			BootstrapPath: filepath.Join(input.RepoRoot, filepath.FromSlash(runtimeBootstrapRepoPath)),
		}, nil
	}

	wrappedBlock, err := WrapRuntimeAgentsBlock(input.OrbitID, payload)
	if err != nil {
		return BootstrapArtifactRestoreResult{}, fmt.Errorf("wrap root BOOTSTRAP block: %w", err)
	}

	filename := filepath.Join(input.RepoRoot, filepath.FromSlash(runtimeBootstrapRepoPath))
	//nolint:gosec // The root BOOTSTRAP path is fixed under the repo root.
	existing, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := contractutil.AtomicWriteFileMode(filename, wrappedBlock, 0o644); err != nil {
				return BootstrapArtifactRestoreResult{}, fmt.Errorf("write root BOOTSTRAP.md: %w", err)
			}
			return BootstrapArtifactRestoreResult{
				OrbitID:       input.OrbitID,
				BootstrapPath: filename,
				Changed:       true,
				Created:       true,
			}, nil
		}
		return BootstrapArtifactRestoreResult{}, fmt.Errorf("read root BOOTSTRAP.md: %w", err)
	}

	document, err := ParseRuntimeAgentsDocument(existing)
	if err != nil {
		return BootstrapArtifactRestoreResult{}, fmt.Errorf("parse root BOOTSTRAP.md: %w", err)
	}
	if currentBlock, extractErr := extractRuntimeGuidanceBlock(document, input.OrbitID, "root BOOTSTRAP.md"); extractErr == nil {
		if !bytes.Equal(currentBlock, payload) {
			return BootstrapArtifactRestoreResult{}, fmt.Errorf(
				"root BOOTSTRAP.md already contains drifted orbit block %q; resolve it before reopening bootstrap surface",
				input.OrbitID,
			)
		}
		return BootstrapArtifactRestoreResult{
			OrbitID:       input.OrbitID,
			BootstrapPath: filename,
			Changed:       false,
			Created:       false,
		}, nil
	}

	merged, err := replaceOrAppendRuntimeGuidanceBlock(existing, input.OrbitID, wrappedBlock, "root BOOTSTRAP.md")
	if err != nil {
		return BootstrapArtifactRestoreResult{}, fmt.Errorf("merge root BOOTSTRAP.md: %w", err)
	}
	if bytes.Equal(existing, merged) {
		return BootstrapArtifactRestoreResult{
			OrbitID:       input.OrbitID,
			BootstrapPath: filename,
			Changed:       false,
			Created:       false,
		}, nil
	}
	if err := contractutil.AtomicWriteFileMode(filename, merged, 0o644); err != nil {
		return BootstrapArtifactRestoreResult{}, fmt.Errorf("write root BOOTSTRAP.md: %w", err)
	}

	return BootstrapArtifactRestoreResult{
		OrbitID:       input.OrbitID,
		BootstrapPath: filename,
		Changed:       true,
		Created:       false,
	}, nil
}
