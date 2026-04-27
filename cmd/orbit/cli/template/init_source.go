package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

// InitSourceResult summarizes one source-branch initialization run.
type InitSourceResult struct {
	RepoRoot           string
	SourceManifestPath string
	SourceBranch       string
	PublishOrbitID     string
	Changed            bool
}

// InitSourceBranch initializes one single-orbit source branch marker in the current branch.
func InitSourceBranch(ctx context.Context, repoRoot string) (InitSourceResult, error) {
	return InitSourceBranchWithInput(ctx, repoRoot, InitSourceInput{})
}

// InitSourceBranchWithInput initializes one single-orbit source branch marker in the current branch.
func InitSourceBranchWithInput(ctx context.Context, repoRoot string, input InitSourceInput) (InitSourceResult, error) {
	currentBranch, err := resolveCurrentBranch(ctx, repoRoot, "source init")
	if err != nil {
		return InitSourceResult{}, err
	}
	if err := ensureCompatibleAuthoringRevision(repoRoot, SourceKind, "source init"); err != nil {
		return InitSourceResult{}, err
	}

	templateManifestExists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, "HEAD", manifestRelativePath)
	if err != nil {
		return InitSourceResult{}, fmt.Errorf("check %s at HEAD: %w", manifestRelativePath, err)
	}

	paths, err := gitpkg.ListAllFilesAtRev(ctx, repoRoot, "HEAD")
	if err != nil {
		return InitSourceResult{}, fmt.Errorf("list tracked files at HEAD: %w", err)
	}
	for _, path := range paths {
		if strings.HasPrefix(path, ".harness/") && !isAllowedSourceBranchHarnessPath(path) {
			return InitSourceResult{}, fmt.Errorf("source branch must not contain %s", path)
		}
	}

	selection, err := resolveAuthoringOrbitSelection(ctx, repoRoot, input.OrbitID, input.Name, input.Description, input.WithSpec)
	if err != nil {
		return InitSourceResult{}, err
	}
	selection.RemovedLegacyTemplate = templateManifestExists

	manifest := SourceManifest{
		SchemaVersion: sourceSchemaVersion,
		Kind:          SourceKind,
		SourceBranch:  currentBranch,
		Publish: &SourcePublishConfig{
			OrbitID: selection.Definition.ID,
		},
	}
	result := InitSourceResult{
		RepoRoot:           repoRoot,
		SourceManifestPath: SourceManifestPath(repoRoot),
		SourceBranch:       currentBranch,
		PublishOrbitID:     selection.Definition.ID,
	}

	existing, err := LoadSourceManifest(repoRoot)
	switch {
	case err == nil:
		if reflect.DeepEqual(existing, manifest) {
			if err := writeInitialSpecDocIfRequested(repoRoot, selection.Definition.ID, input.WithSpec, selection.CreatedHostedOrbit); err != nil {
				return InitSourceResult{}, err
			}
			if err := materializeInitialGuidanceIfCreated(ctx, repoRoot, selection.Definition.ID, selection.CreatedHostedOrbit); err != nil {
				return InitSourceResult{}, err
			}
			if err := removeLegacyOrbitArtifacts(repoRoot, selection); err != nil {
				return InitSourceResult{}, err
			}
			result.Changed = selection.CreatedHostedOrbit || selection.MigratedLegacyOrbit || selection.RemovedLegacyTemplate
			return result, nil
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return InitSourceResult{}, fmt.Errorf("load %s: %w", sourceManifestRelativePath, err)
	}

	writtenPath, err := WriteSourceManifest(repoRoot, manifest)
	if err != nil {
		return InitSourceResult{}, fmt.Errorf("write %s: %w", sourceManifestRelativePath, err)
	}
	result.SourceManifestPath = writtenPath
	result.Changed = true
	if err := writeInitialSpecDocIfRequested(repoRoot, selection.Definition.ID, input.WithSpec, selection.CreatedHostedOrbit); err != nil {
		return InitSourceResult{}, err
	}
	if err := materializeInitialGuidanceIfCreated(ctx, repoRoot, selection.Definition.ID, selection.CreatedHostedOrbit); err != nil {
		return InitSourceResult{}, err
	}

	if err := removeLegacyOrbitArtifacts(repoRoot, selection); err != nil {
		return InitSourceResult{}, err
	}

	return result, nil
}

func removeLegacyTemplateManifest(repoRoot string) error {
	filename := filepath.Join(repoRoot, filepath.FromSlash(manifestRelativePath))
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", manifestRelativePath, err)
	}

	return nil
}
