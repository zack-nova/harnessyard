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

const runtimeBootstrapRepoPath = "BOOTSTRAP.md"

// BootstrapMaterializeInput captures one authored bootstrap-guidance materialization request.
type BootstrapMaterializeInput struct {
	RepoRoot  string
	OrbitID   string
	Force     bool
	SeedEmpty bool
}

// BootstrapMaterializeResult reports the repo root BOOTSTRAP container touched by one successful materialize.
type BootstrapMaterializeResult struct {
	OrbitID       string
	BootstrapPath string
	Changed       bool
	Forced        bool
}

// MaterializeOrbitBootstrap renders one authored orbit bootstrap guidance into the repo root BOOTSTRAP container.
func MaterializeOrbitBootstrap(ctx context.Context, input BootstrapMaterializeInput) (BootstrapMaterializeResult, error) {
	if input.RepoRoot == "" {
		return BootstrapMaterializeResult{}, errors.New("repo root must not be empty")
	}
	revisionKind, err := resolveAllowedBriefRevisionKind(input.RepoRoot, "materialize")
	if err != nil {
		return BootstrapMaterializeResult{}, err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, input.RepoRoot, input.OrbitID)
	if err != nil {
		return BootstrapMaterializeResult{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}
	bootstrapStatus, err := InspectBootstrapOrbitForRevision(ctx, input.RepoRoot, "", input.OrbitID, revisionKind)
	if err != nil {
		return BootstrapMaterializeResult{}, err
	}
	if PlanBootstrapGuidanceMaterialize(bootstrapStatus).ReasonCode == "bootstrap_completed" {
		return BootstrapMaterializeResult{}, fmt.Errorf(
			"bootstrap guidance for orbit %q is closed because bootstrap is already completed in this runtime",
			input.OrbitID,
		)
	}

	payload, hasTruth, err := materializedOrbitBootstrapPayload(spec, input.RepoRoot, revisionKind)
	if err != nil {
		return BootstrapMaterializeResult{}, err
	}
	if input.SeedEmpty && !hasExplicitOrbitBootstrapTemplate(spec) {
		payload = []byte{}
		hasTruth = true
	}
	if !hasTruth {
		return BootstrapMaterializeResult{}, fmt.Errorf("orbit %q does not define materializable bootstrap guidance", spec.ID)
	}

	wrappedBlock, err := WrapRuntimeAgentsBlock(input.OrbitID, payload)
	if err != nil {
		return BootstrapMaterializeResult{}, fmt.Errorf("wrap root BOOTSTRAP block: %w", err)
	}

	filename := filepath.Join(input.RepoRoot, filepath.FromSlash(runtimeBootstrapRepoPath))
	//nolint:gosec // The root BOOTSTRAP path is fixed under the repo root.
	existing, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := contractutil.AtomicWriteFileMode(filename, wrappedBlock, 0o644); err != nil {
				return BootstrapMaterializeResult{}, fmt.Errorf("write root BOOTSTRAP.md: %w", err)
			}
			return BootstrapMaterializeResult{
				OrbitID:       input.OrbitID,
				BootstrapPath: filename,
				Changed:       true,
				Forced:        input.Force,
			}, nil
		}
		return BootstrapMaterializeResult{}, fmt.Errorf("read root BOOTSTRAP.md: %w", err)
	}

	document, err := ParseRuntimeAgentsDocument(existing)
	if err != nil {
		return BootstrapMaterializeResult{}, fmt.Errorf("parse root BOOTSTRAP.md: %w", err)
	}

	if currentBlock, extractErr := extractRuntimeGuidanceBlock(document, input.OrbitID, "root BOOTSTRAP.md"); extractErr == nil {
		if !bytes.Equal(currentBlock, payload) && !input.Force {
			return BootstrapMaterializeResult{}, fmt.Errorf(
				"root BOOTSTRAP.md already contains drifted orbit block %q; rerun with --force to overwrite it",
				input.OrbitID,
			)
		}
	}

	merged, err := replaceOrAppendRuntimeGuidanceBlock(existing, input.OrbitID, wrappedBlock, "root BOOTSTRAP.md")
	if err != nil {
		return BootstrapMaterializeResult{}, fmt.Errorf("merge root BOOTSTRAP.md: %w", err)
	}
	if bytes.Equal(existing, merged) {
		return BootstrapMaterializeResult{
			OrbitID:       input.OrbitID,
			BootstrapPath: filename,
			Changed:       false,
			Forced:        input.Force,
		}, nil
	}
	if err := contractutil.AtomicWriteFileMode(filename, merged, 0o644); err != nil {
		return BootstrapMaterializeResult{}, fmt.Errorf("write root BOOTSTRAP.md: %w", err)
	}

	return BootstrapMaterializeResult{
		OrbitID:       input.OrbitID,
		BootstrapPath: filename,
		Changed:       true,
		Forced:        input.Force,
	}, nil
}

func materializedOrbitBootstrapPayload(spec orbitpkg.OrbitSpec, repoRoot string, revisionKind string) ([]byte, bool, error) {
	body := orbitBootstrapBody(spec)
	if len(body) == 0 {
		return nil, false, nil
	}

	renderedBindings := map[string]string{}
	if revisionKind == "runtime" {
		runtimeBindings, err := loadOptionalRuntimeVars(repoRoot)
		if err != nil {
			return nil, false, err
		}
		renderedBindings = renderBindingValues(runtimeBindings)
	}

	renderedFiles, err := RenderTemplateFilesAllowingUnresolved([]CandidateFile{{
		Path:    runtimeBootstrapRepoPath,
		Content: body,
	}}, renderedBindings)
	if err != nil {
		return nil, false, fmt.Errorf("render orbit bootstrap guidance: %w", err)
	}

	return renderedFiles[0].Content, true, nil
}
