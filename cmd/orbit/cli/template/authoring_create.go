package orbittemplate

import (
	"context"
	"fmt"
	"os"
	"strings"
)

import gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"

// CreateSourceInput captures one-step source authoring repo bootstrap input.
type CreateSourceInput struct {
	TargetPath  string
	OrbitID     string
	Name        string
	Description string
	WithSpec    bool
}

// CreateSourceResult summarizes one-step source authoring repo bootstrap.
type CreateSourceResult struct {
	RepoRoot           string
	SourceManifestPath string
	SourceBranch       string
	PublishOrbitID     string
	GitInitialized     bool
	Changed            bool
}

// CreateTemplateInput captures one-step orbit_template authoring repo bootstrap input.
type CreateTemplateInput struct {
	TargetPath  string
	OrbitID     string
	Name        string
	Description string
	WithSpec    bool
}

// CreateTemplateResult summarizes one-step orbit_template repo bootstrap.
type CreateTemplateResult struct {
	RepoRoot       string
	ManifestPath   string
	CurrentBranch  string
	OrbitID        string
	GitInitialized bool
	Changed        bool
}

// CreateSourceRepo bootstraps a Git repo root and initializes it as a source authoring repo.
func CreateSourceRepo(ctx context.Context, input CreateSourceInput) (CreateSourceResult, error) {
	repoRoot, gitInitialized, err := ensureAuthoringRepoRoot(ctx, input.TargetPath)
	if err != nil {
		return CreateSourceResult{}, err
	}
	if err := requireExplicitOrbitForNewAuthoringRepo(gitInitialized, input.OrbitID); err != nil {
		return CreateSourceResult{}, err
	}
	if err := ensureCompatibleAuthoringRevision(repoRoot, SourceKind, "source init"); err != nil {
		return CreateSourceResult{}, err
	}

	result, err := InitSourceBranchWithInput(ctx, repoRoot, InitSourceInput{
		OrbitID:     input.OrbitID,
		Name:        input.Name,
		Description: input.Description,
		WithSpec:    input.WithSpec,
	})
	if err != nil {
		return CreateSourceResult{}, err
	}

	return CreateSourceResult{
		RepoRoot:           result.RepoRoot,
		SourceManifestPath: result.SourceManifestPath,
		SourceBranch:       result.SourceBranch,
		PublishOrbitID:     result.PublishOrbitID,
		GitInitialized:     gitInitialized,
		Changed:            result.Changed,
	}, nil
}

// CreateTemplateRepo bootstraps a Git repo root and initializes it as an orbit_template authoring repo.
func CreateTemplateRepo(ctx context.Context, input CreateTemplateInput) (CreateTemplateResult, error) {
	repoRoot, gitInitialized, err := ensureAuthoringRepoRoot(ctx, input.TargetPath)
	if err != nil {
		return CreateTemplateResult{}, err
	}
	if err := requireExplicitOrbitForNewAuthoringRepo(gitInitialized, input.OrbitID); err != nil {
		return CreateTemplateResult{}, err
	}
	if err := ensureCompatibleAuthoringRevision(repoRoot, "orbit_template", "template init"); err != nil {
		return CreateTemplateResult{}, err
	}

	result, err := InitTemplateBranch(ctx, repoRoot, InitTemplateInput{
		OrbitID:     input.OrbitID,
		Name:        input.Name,
		Description: input.Description,
		WithSpec:    input.WithSpec,
	})
	if err != nil {
		return CreateTemplateResult{}, err
	}

	return CreateTemplateResult{
		RepoRoot:       result.RepoRoot,
		ManifestPath:   result.ManifestPath,
		CurrentBranch:  result.CurrentBranch,
		OrbitID:        result.OrbitID,
		GitInitialized: gitInitialized,
		Changed:        result.Changed,
	}, nil
}

func ensureAuthoringRepoRoot(ctx context.Context, targetPath string) (string, bool, error) {
	if err := os.MkdirAll(targetPath, 0o750); err != nil {
		return "", false, fmt.Errorf("create target directory %s: %w", targetPath, err)
	}

	gitInitialized, err := gitpkg.EnsureRepoRoot(ctx, targetPath)
	if err != nil {
		return "", false, fmt.Errorf("ensure git repo root: %w", err)
	}

	repo, err := gitpkg.DiscoverRepo(ctx, targetPath)
	if err != nil {
		return "", false, fmt.Errorf("discover git repository: %w", err)
	}
	if gitpkg.ComparablePath(repo.Root) != gitpkg.ComparablePath(targetPath) {
		return "", false, fmt.Errorf("expected authoring root %s to be a git repo root, got %s", targetPath, repo.Root)
	}

	return repo.Root, gitInitialized, nil
}

func requireExplicitOrbitForNewAuthoringRepo(gitInitialized bool, orbitID string) error {
	if !gitInitialized || strings.TrimSpace(orbitID) != "" {
		return nil
	}

	return fmt.Errorf("authoring create requires --orbit when target is not already a Git repository")
}
