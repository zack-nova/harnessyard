package orbittemplate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// BriefMaterializeInput captures one authored-brief materialization request.
type BriefMaterializeInput struct {
	RepoRoot  string
	OrbitID   string
	Force     bool
	SeedEmpty bool
}

// BriefMaterializeResult reports the repo root AGENTS container touched by one successful materialize.
type BriefMaterializeResult struct {
	OrbitID    string
	AgentsPath string
	Changed    bool
	Forced     bool
}

// MaterializeOrbitBrief renders one authored orbit brief into the repo root AGENTS container.
func MaterializeOrbitBrief(ctx context.Context, input BriefMaterializeInput) (BriefMaterializeResult, error) {
	if input.RepoRoot == "" {
		return BriefMaterializeResult{}, errors.New("repo root must not be empty")
	}
	revisionKind, err := resolveAllowedBriefRevisionKind(input.RepoRoot, "materialize")
	if err != nil {
		return BriefMaterializeResult{}, err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, input.RepoRoot, input.OrbitID)
	if err != nil {
		return BriefMaterializeResult{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}

	payload, hasTruth, err := materializedOrbitBriefPayload(spec, input.RepoRoot, revisionKind)
	if err != nil {
		return BriefMaterializeResult{}, err
	}
	if input.SeedEmpty && !hasExplicitOrbitAgentsTemplate(spec) {
		payload = []byte{}
		hasTruth = true
	}
	if !hasTruth {
		return BriefMaterializeResult{}, fmt.Errorf("orbit %q does not define a materializable brief", spec.ID)
	}

	wrappedBlock, err := WrapRuntimeAgentsBlock(input.OrbitID, payload)
	if err != nil {
		return BriefMaterializeResult{}, fmt.Errorf("wrap root AGENTS block: %w", err)
	}

	filename := filepath.Join(input.RepoRoot, filepath.FromSlash(runtimeAgentsRepoPath))
	//nolint:gosec // The root AGENTS path is fixed under the repo root.
	existing, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := contractutil.AtomicWriteFileMode(filename, wrappedBlock, 0o644); err != nil {
				return BriefMaterializeResult{}, fmt.Errorf("write root AGENTS.md: %w", err)
			}
			return BriefMaterializeResult{
				OrbitID:    input.OrbitID,
				AgentsPath: filename,
				Changed:    true,
				Forced:     input.Force,
			}, nil
		}
		return BriefMaterializeResult{}, fmt.Errorf("read root AGENTS.md: %w", err)
	}

	document, err := ParseRuntimeAgentsDocument(existing)
	if err != nil {
		return BriefMaterializeResult{}, fmt.Errorf("parse root AGENTS.md: %w", err)
	}

	if currentBlock, extractErr := extractRuntimeAgentsBlock(document, input.OrbitID); extractErr == nil {
		if !bytes.Equal(currentBlock, payload) && !input.Force {
			return BriefMaterializeResult{}, fmt.Errorf(
				"root AGENTS.md already contains drifted orbit block %q; run `orbit brief backfill --orbit %s` or rerun with --force to overwrite it",
				input.OrbitID,
				input.OrbitID,
			)
		}
	}

	merged, err := replaceOrAppendRuntimeAgentsBlock(existing, input.OrbitID, wrappedBlock)
	if err != nil {
		return BriefMaterializeResult{}, fmt.Errorf("merge root AGENTS.md: %w", err)
	}
	if bytes.Equal(existing, merged) {
		return BriefMaterializeResult{
			OrbitID:    input.OrbitID,
			AgentsPath: filename,
			Changed:    false,
			Forced:     input.Force,
		}, nil
	}
	if err := contractutil.AtomicWriteFileMode(filename, merged, 0o644); err != nil {
		return BriefMaterializeResult{}, fmt.Errorf("write root AGENTS.md: %w", err)
	}

	return BriefMaterializeResult{
		OrbitID:    input.OrbitID,
		AgentsPath: filename,
		Changed:    true,
		Forced:     input.Force,
	}, nil
}

func materializedOrbitBriefPayload(spec orbitpkg.OrbitSpec, repoRoot string, revisionKind string) ([]byte, bool, error) {
	body := orbitAgentsBody(spec)
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
		Path:    runtimeAgentsRepoPath,
		Content: body,
	}}, renderedBindings)
	if err != nil {
		return nil, false, fmt.Errorf("render orbit brief: %w", err)
	}

	return renderedFiles[0].Content, true, nil
}

func renderBindingValues(values map[string]bindings.VariableBinding) map[string]string {
	rendered := make(map[string]string, len(values))
	for name, binding := range values {
		rendered[name] = binding.Value
	}

	return rendered
}
