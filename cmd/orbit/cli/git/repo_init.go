package git

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// EnsureRepoRoot makes sure targetPath is itself a Git repo root, initializing one when needed.
func EnsureRepoRoot(ctx context.Context, targetPath string) (bool, error) {
	repoRoot, err := RepoRoot(ctx, targetPath)
	switch {
	case err == nil && ComparablePath(repoRoot) == ComparablePath(targetPath):
		return false, nil
	case err == nil:
		// target exists inside another repository; create an independent repo here.
	case err != nil:
		// not a repo yet; initialize below.
	}

	//nolint:gosec // Git is invoked with explicit argument lists from an internal command implementation.
	cmd := exec.CommandContext(ctx, "git", "init", targetPath)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		return false, fmt.Errorf("git init %s: %w: %s", targetPath, runErr, string(output))
	}

	return true, nil
}

// ComparablePath normalizes one filesystem path for root-equivalence checks.
func ComparablePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved)
	}

	return filepath.Clean(path)
}
