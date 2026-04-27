package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"gopkg.in/yaml.v3"
)

// ZeroGitCommitID is the sentinel written into portable template provenance
// when the current repo has not produced a first commit yet.
const ZeroGitCommitID = "0000000000000000000000000000000000000000"

// CurrentRepoState captures the current-worktree identity needed by authoring
// and portable-template commands.
type CurrentRepoState struct {
	Kind          string
	OrbitID       string
	HarnessID     string
	CurrentBranch string
	Detached      bool
	HeadExists    bool
}

type currentRepoStateManifest struct {
	Kind     string                           `yaml:"kind"`
	Source   *currentRepoStateSourceMetadata  `yaml:"source"`
	Runtime  *currentRepoStateRuntimeMetadata `yaml:"runtime"`
	Template *currentRepoStateTemplate        `yaml:"template"`
}

type currentRepoStateSourceMetadata struct {
	Package ids.PackageIdentity `yaml:"package"`
	OrbitID string              `yaml:"orbit_id"`
}

type currentRepoStateRuntimeMetadata struct {
	Package ids.PackageIdentity `yaml:"package"`
	ID      string              `yaml:"id"`
}

type currentRepoStateTemplate struct {
	Package   ids.PackageIdentity `yaml:"package"`
	OrbitID   string              `yaml:"orbit_id"`
	HarnessID string              `yaml:"harness_id"`
}

// LoadCurrentRepoState resolves the current worktree manifest identity,
// current symbolic branch, detached status, and whether HEAD exists.
func LoadCurrentRepoState(ctx context.Context, repoRoot string) (CurrentRepoState, error) {
	state := CurrentRepoState{Kind: "plain"}

	manifestData, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, branchManifestPath)
	switch {
	case err == nil:
		if err := applyCurrentRepoStateManifest(&state, manifestData); err != nil {
			// CurrentRepoState is intentionally identity-oriented rather than
			// validation-oriented. Invalid manifest bytes should not prevent
			// callers from still observing detached/head-exists status.
			state = CurrentRepoState{Kind: "plain"}
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return CurrentRepoState{}, fmt.Errorf("read %s: %w", branchManifestPath, err)
	}

	currentBranch, err := gitpkg.CurrentBranch(ctx, repoRoot)
	if err != nil {
		return CurrentRepoState{}, fmt.Errorf("resolve current branch: %w", err)
	}
	if strings.TrimSpace(currentBranch) == "HEAD" {
		state.Detached = true
	} else {
		state.CurrentBranch = strings.TrimSpace(currentBranch)
	}

	headExists, err := gitpkg.RevisionExists(ctx, repoRoot, "HEAD")
	if err != nil {
		return CurrentRepoState{}, fmt.Errorf("check HEAD revision: %w", err)
	}
	state.HeadExists = headExists

	return state, nil
}

// RequireCurrentBranch ensures the current repo state is attached to a
// symbolic branch. Unborn branches are allowed.
func RequireCurrentBranch(state CurrentRepoState, operation string) (string, error) {
	if state.Detached || strings.TrimSpace(state.CurrentBranch) == "" {
		return "", fmt.Errorf("%s requires a current branch; detached HEAD is not supported", operation)
	}

	return state.CurrentBranch, nil
}

// CurrentCommitOrZero resolves HEAD when it exists, or returns the zero commit
// sentinel for unborn repositories.
func CurrentCommitOrZero(ctx context.Context, repoRoot string) (string, error) {
	exists, err := gitpkg.RevisionExists(ctx, repoRoot, "HEAD")
	if err != nil {
		return "", fmt.Errorf("check HEAD revision: %w", err)
	}
	if !exists {
		return ZeroGitCommitID, nil
	}

	commit, err := gitpkg.HeadCommit(ctx, repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve HEAD commit: %w", err)
	}

	return strings.TrimSpace(commit), nil
}

func applyCurrentRepoStateManifest(state *CurrentRepoState, data []byte) error {
	var manifest currentRepoStateManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode branch manifest state: %w", err)
	}

	state.Kind = strings.TrimSpace(manifest.Kind)
	if state.Kind == "" {
		state.Kind = "plain"
	}

	switch state.Kind {
	case "source":
		if manifest.Source != nil {
			state.OrbitID = strings.TrimSpace(packageNameOrFallback(manifest.Source.Package, manifest.Source.OrbitID))
		}
	case "orbit_template":
		if manifest.Template != nil {
			state.OrbitID = strings.TrimSpace(packageNameOrFallback(manifest.Template.Package, manifest.Template.OrbitID))
		}
	case "runtime":
		if manifest.Runtime != nil {
			state.HarnessID = strings.TrimSpace(packageNameOrFallback(manifest.Runtime.Package, manifest.Runtime.ID))
		}
	case "harness_template":
		if manifest.Template != nil {
			state.HarnessID = strings.TrimSpace(packageNameOrFallback(manifest.Template.Package, manifest.Template.HarnessID))
		}
	}

	return nil
}

func packageNameOrFallback(identity ids.PackageIdentity, fallback string) string {
	if strings.TrimSpace(identity.Name) != "" {
		return identity.Name
	}
	return fallback
}
