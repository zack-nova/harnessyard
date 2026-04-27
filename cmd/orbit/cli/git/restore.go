package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// RestorePathspec restores the requested repo-relative paths from a revision.
func RestorePathspec(ctx context.Context, repoRoot string, revision string, paths []string) (err error) {
	pathspecFile, err := CreatePathspecFile(paths)
	if err != nil {
		return fmt.Errorf("create restore pathspec file: %w", err)
	}
	defer func() {
		cleanupErr := pathspecFile.Cleanup()
		if cleanupErr == nil {
			return
		}

		wrappedCleanupErr := fmt.Errorf("cleanup restore pathspec file: %w", cleanupErr)
		if err == nil {
			err = wrappedCleanupErr
			return
		}

		err = errors.Join(err, wrappedCleanupErr)
	}()

	args := []string{
		"restore",
		"--source=" + revision,
		"--worktree",
		"--staged",
		"--pathspec-from-file=" + pathspecFile.Path,
		"--pathspec-file-nul",
	}

	if _, err := runGit(ctx, repoRoot, args...); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return nil
}
