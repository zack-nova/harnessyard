package git

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// InitNoConeSparseCheckout enables sparse-checkout in no-cone mode.
func InitNoConeSparseCheckout(ctx context.Context, repoRoot string) error {
	if _, err := runGit(ctx, repoRoot, "sparse-checkout", "init", "--no-cone"); err != nil {
		return fmt.Errorf("git sparse-checkout init --no-cone: %w", err)
	}

	return nil
}

// SetSparseCheckoutPaths applies an explicit resolved path list to sparse-checkout.
func SetSparseCheckoutPaths(ctx context.Context, repoRoot string, paths []string) error {
	normalizedPaths := make([]string, 0, len(paths))
	for _, pathValue := range paths {
		normalizedPath, err := ids.NormalizeRepoRelativePath(pathValue)
		if err != nil {
			return fmt.Errorf("normalize sparse-checkout path %q: %w", pathValue, err)
		}
		normalizedPaths = append(normalizedPaths, normalizedPath)
	}

	sort.Strings(normalizedPaths)

	input := []byte(strings.Join(normalizedPaths, "\n"))
	if len(normalizedPaths) > 0 {
		input = append(input, '\n')
	}

	if _, err := runGitInput(ctx, repoRoot, input, "sparse-checkout", "set", "--stdin"); err != nil {
		return fmt.Errorf("git sparse-checkout set --stdin: %w", err)
	}

	return nil
}

// DisableSparseCheckout restores the full tracked worktree view.
func DisableSparseCheckout(ctx context.Context, repoRoot string) error {
	if _, err := runGit(ctx, repoRoot, "sparse-checkout", "disable"); err != nil {
		return fmt.Errorf("git sparse-checkout disable: %w", err)
	}

	return nil
}

// SparseCheckoutEnabled reports whether the current worktree is projected via
// sparse-checkout.
func SparseCheckoutEnabled(ctx context.Context, repoRoot string) (bool, error) {
	output, err := runGit(ctx, repoRoot, "config", "--bool", "--default", "false", "core.sparseCheckout")
	if err != nil {
		return false, fmt.Errorf("git config --bool --default false core.sparseCheckout: %w", err)
	}

	switch strings.TrimSpace(string(output)) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, fmt.Errorf("unexpected core.sparseCheckout value %q", strings.TrimSpace(string(output)))
	}
}
