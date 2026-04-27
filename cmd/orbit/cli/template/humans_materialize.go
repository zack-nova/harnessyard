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

const runtimeHumansRepoPath = "HUMANS.md"

// HumansMaterializeInput captures one authored human-guidance materialization request.
type HumansMaterializeInput struct {
	RepoRoot  string
	OrbitID   string
	Force     bool
	SeedEmpty bool
}

// HumansMaterializeResult reports the repo root HUMANS container touched by one successful materialize.
type HumansMaterializeResult struct {
	OrbitID    string
	HumansPath string
	Changed    bool
	Forced     bool
}

// MaterializeOrbitHumans renders one authored orbit human guidance into the repo root HUMANS container.
func MaterializeOrbitHumans(ctx context.Context, input HumansMaterializeInput) (HumansMaterializeResult, error) {
	if input.RepoRoot == "" {
		return HumansMaterializeResult{}, errors.New("repo root must not be empty")
	}
	revisionKind, err := resolveAllowedBriefRevisionKind(input.RepoRoot, "materialize")
	if err != nil {
		return HumansMaterializeResult{}, err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, input.RepoRoot, input.OrbitID)
	if err != nil {
		return HumansMaterializeResult{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}

	payload, hasTruth, err := materializedOrbitHumansPayload(spec, input.RepoRoot, revisionKind)
	if err != nil {
		return HumansMaterializeResult{}, err
	}
	if input.SeedEmpty && !hasExplicitOrbitHumansTemplate(spec) {
		payload = []byte{}
		hasTruth = true
	}
	if !hasTruth {
		return HumansMaterializeResult{}, fmt.Errorf("orbit %q does not define materializable human guidance", spec.ID)
	}

	wrappedBlock, err := WrapRuntimeAgentsBlock(input.OrbitID, payload)
	if err != nil {
		return HumansMaterializeResult{}, fmt.Errorf("wrap root HUMANS block: %w", err)
	}

	filename := filepath.Join(input.RepoRoot, filepath.FromSlash(runtimeHumansRepoPath))
	//nolint:gosec // The root HUMANS path is fixed under the repo root.
	existing, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := contractutil.AtomicWriteFileMode(filename, wrappedBlock, 0o644); err != nil {
				return HumansMaterializeResult{}, fmt.Errorf("write root HUMANS.md: %w", err)
			}
			return HumansMaterializeResult{
				OrbitID:    input.OrbitID,
				HumansPath: filename,
				Changed:    true,
				Forced:     input.Force,
			}, nil
		}
		return HumansMaterializeResult{}, fmt.Errorf("read root HUMANS.md: %w", err)
	}

	document, err := ParseRuntimeAgentsDocument(existing)
	if err != nil {
		return HumansMaterializeResult{}, fmt.Errorf("parse root HUMANS.md: %w", err)
	}

	if currentBlock, extractErr := extractRuntimeGuidanceBlock(document, input.OrbitID, "root HUMANS.md"); extractErr == nil {
		if !bytes.Equal(currentBlock, payload) && !input.Force {
			return HumansMaterializeResult{}, fmt.Errorf(
				"root HUMANS.md already contains drifted orbit block %q; rerun with --force to overwrite it",
				input.OrbitID,
			)
		}
	}

	merged, err := replaceOrAppendRuntimeGuidanceBlock(existing, input.OrbitID, wrappedBlock, "root HUMANS.md")
	if err != nil {
		return HumansMaterializeResult{}, fmt.Errorf("merge root HUMANS.md: %w", err)
	}
	if bytes.Equal(existing, merged) {
		return HumansMaterializeResult{
			OrbitID:    input.OrbitID,
			HumansPath: filename,
			Changed:    false,
			Forced:     input.Force,
		}, nil
	}
	if err := contractutil.AtomicWriteFileMode(filename, merged, 0o644); err != nil {
		return HumansMaterializeResult{}, fmt.Errorf("write root HUMANS.md: %w", err)
	}

	return HumansMaterializeResult{
		OrbitID:    input.OrbitID,
		HumansPath: filename,
		Changed:    true,
		Forced:     input.Force,
	}, nil
}

func materializedOrbitHumansPayload(spec orbitpkg.OrbitSpec, repoRoot string, revisionKind string) ([]byte, bool, error) {
	body := orbitHumansBody(spec)
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
		Path:    runtimeHumansRepoPath,
		Content: body,
	}}, renderedBindings)
	if err != nil {
		return nil, false, fmt.Errorf("render orbit humans guidance: %w", err)
	}

	return renderedFiles[0].Content, true, nil
}
