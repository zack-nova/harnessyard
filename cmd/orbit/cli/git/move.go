package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// MovePath relocates one repo-relative path with git mv.
func MovePath(ctx context.Context, repoRoot string, sourcePath string, destinationPath string) error {
	normalizedSource, err := ids.NormalizeRepoRelativePath(sourcePath)
	if err != nil {
		return fmt.Errorf("normalize source path %q: %w", sourcePath, err)
	}
	normalizedDestination, err := ids.NormalizeRepoRelativePath(destinationPath)
	if err != nil {
		return fmt.Errorf("normalize destination path %q: %w", destinationPath, err)
	}

	absoluteDestination := filepath.Join(repoRoot, filepath.FromSlash(normalizedDestination))
	if err := os.MkdirAll(filepath.Dir(absoluteDestination), 0o750); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", normalizedDestination, err)
	}

	if _, err := runGit(ctx, repoRoot, "mv", "--", normalizedSource, normalizedDestination); err != nil {
		return fmt.Errorf("git mv %q -> %q: %w", normalizedSource, normalizedDestination, err)
	}

	return nil
}
