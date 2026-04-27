package harness

import (
	"context"
	"errors"
	"fmt"
	"os"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

// ResolvedRoot captures the harness repo root plus the validated control-plane documents loaded from it.
type ResolvedRoot struct {
	Repo     gitpkg.Repo
	Manifest ManifestFile
	Runtime  RuntimeFile
}

// ResolvedManifestRoot captures the harness repo root plus the validated manifest loaded from it.
type ResolvedManifestRoot struct {
	Repo     gitpkg.Repo
	Manifest ManifestFile
}

// ResolveRoot discovers the git repo root from any working directory and validates .harness/manifest.yaml at that root.
func ResolveRoot(ctx context.Context, workingDir string) (ResolvedRoot, error) {
	resolved, err := ResolveManifestRoot(ctx, workingDir)
	if err != nil {
		return ResolvedRoot{}, err
	}
	manifest := resolved.Manifest
	if manifest.Kind != ManifestKindRuntime {
		return ResolvedRoot{}, fmt.Errorf("harness root must contain a runtime manifest at %s", resolved.Repo.Root)
	}

	runtime, err := RuntimeFileFromManifestFile(resolved.Manifest)
	if err != nil {
		return ResolvedRoot{}, fmt.Errorf("convert harness manifest to runtime view: %w", err)
	}

	return ResolvedRoot{
		Repo:     resolved.Repo,
		Manifest: resolved.Manifest,
		Runtime:  runtime,
	}, nil
}

// ResolveManifestRoot discovers the git repo root from any working directory and validates .harness/manifest.yaml.
func ResolveManifestRoot(ctx context.Context, workingDir string) (ResolvedManifestRoot, error) {
	repo, err := gitpkg.DiscoverRepo(ctx, workingDir)
	if err != nil {
		return ResolvedManifestRoot{}, fmt.Errorf("discover git repository: %w", err)
	}

	manifest, err := LoadManifestFile(repo.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ResolvedManifestRoot{}, fmt.Errorf("harness manifest is not initialized at %s", repo.Root)
		}
		return ResolvedManifestRoot{}, fmt.Errorf("load harness manifest: %w", err)
	}

	return ResolvedManifestRoot{
		Repo:     repo,
		Manifest: manifest,
	}, nil
}
