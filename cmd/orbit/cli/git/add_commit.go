package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// StageAllPathspec stages the requested repo-relative paths with git add -A.
// Use --sparse so new in-scope files can be added while the worktree is projected
// to a sparse set of tracked paths.
func StageAllPathspec(ctx context.Context, repoRoot string, paths []string) (err error) {
	pathspecFile, err := CreatePathspecFile(paths)
	if err != nil {
		return fmt.Errorf("create add pathspec file: %w", err)
	}
	defer func() {
		cleanupErr := pathspecFile.Cleanup()
		if cleanupErr == nil {
			return
		}

		wrappedCleanupErr := fmt.Errorf("cleanup add pathspec file: %w", cleanupErr)
		if err == nil {
			err = wrappedCleanupErr
			return
		}

		err = errors.Join(err, wrappedCleanupErr)
	}()

	args := []string{
		"add",
		"--sparse",
		"-A",
		"--pathspec-from-file=" + pathspecFile.Path,
		"--pathspec-file-nul",
	}

	if _, err := runGit(ctx, repoRoot, args...); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return nil
}

// ResetIndexPathspec restores the index entries for the requested repo-relative paths.
func ResetIndexPathspec(ctx context.Context, repoRoot string, paths []string) (err error) {
	if len(paths) == 0 {
		return nil
	}

	pathspecFile, err := CreatePathspecFile(paths)
	if err != nil {
		return fmt.Errorf("create reset pathspec file: %w", err)
	}
	defer func() {
		cleanupErr := pathspecFile.Cleanup()
		if cleanupErr == nil {
			return
		}

		wrappedCleanupErr := fmt.Errorf("cleanup reset pathspec file: %w", cleanupErr)
		if err == nil {
			err = wrappedCleanupErr
			return
		}

		err = errors.Join(err, wrappedCleanupErr)
	}()

	headExists, err := RevisionExists(ctx, repoRoot, "HEAD")
	if err != nil {
		return fmt.Errorf("check HEAD before index reset: %w", err)
	}

	args := []string{
		"reset",
		"-q",
		"HEAD",
		"--pathspec-from-file=" + pathspecFile.Path,
		"--pathspec-file-nul",
	}
	if !headExists {
		args = []string{
			"rm",
			"-q",
			"--cached",
			"--ignore-unmatch",
			"-r",
			"--pathspec-from-file=" + pathspecFile.Path,
			"--pathspec-file-nul",
		}
	}

	if _, err := runGit(ctx, repoRoot, args...); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return nil
}

// CommitPathspec creates a scoped Git commit from the requested repo-relative paths.
func CommitPathspec(ctx context.Context, repoRoot string, paths []string, message string) (err error) {
	pathspecFile, err := CreatePathspecFile(paths)
	if err != nil {
		return fmt.Errorf("create commit pathspec file: %w", err)
	}
	defer func() {
		cleanupErr := pathspecFile.Cleanup()
		if cleanupErr == nil {
			return
		}

		wrappedCleanupErr := fmt.Errorf("cleanup commit pathspec file: %w", cleanupErr)
		if err == nil {
			err = wrappedCleanupErr
			return
		}

		err = errors.Join(err, wrappedCleanupErr)
	}()

	args := []string{
		"commit",
		"-m",
		message,
		"--pathspec-from-file=" + pathspecFile.Path,
		"--pathspec-file-nul",
	}

	if _, err := runGit(ctx, repoRoot, args...); err != nil {
		return fmt.Errorf("git commit with scoped pathspec: %w", err)
	}

	return nil
}

// HeadCommit returns the current HEAD commit hash.
func HeadCommit(ctx context.Context, repoRoot string) (string, error) {
	output, err := runGit(ctx, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
