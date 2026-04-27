package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const headRevision = "HEAD"

// RevisionExists reports whether a revision is available in the repository.
func RevisionExists(ctx context.Context, repoRoot string, rev string) (bool, error) {
	if strings.TrimSpace(rev) == "" {
		return false, errors.New("revision must not be empty")
	}

	commandArgs := []string{"-C", repoRoot, "rev-parse", "--verify", "--quiet", rev}
	//nolint:gosec // Git is invoked with explicit argument lists from internal callers only.
	cmd := exec.CommandContext(ctx, "git", commandArgs...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}

		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			return false, fmt.Errorf("run git rev-parse --verify --quiet %s: %w: %s", rev, err, stderrText)
		}

		return false, fmt.Errorf("run git rev-parse --verify --quiet %s: %w", rev, err)
	}

	return true, nil
}

// ListFilesAtRev lists tracked files under the requested repo-relative prefix at a revision.
func ListFilesAtRev(ctx context.Context, repoRoot string, rev string, prefix string) ([]string, error) {
	normalizedPrefix, err := ids.NormalizeRepoRelativePath(prefix)
	if err != nil {
		return nil, fmt.Errorf("normalize revision prefix %q: %w", prefix, err)
	}

	exists, err := RevisionExists(ctx, repoRoot, rev)
	if err != nil {
		return nil, fmt.Errorf("check revision %q: %w", rev, err)
	}
	if !exists {
		return nil, nil
	}

	output, err := runGit(ctx, repoRoot, "ls-tree", "-r", "--name-only", "-z", rev, "--", normalizedPrefix)
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s -- %s: %w", rev, normalizedPrefix, err)
	}

	paths := parseNULTerminated(output)
	for index, value := range paths {
		normalizedPath, normalizeErr := ids.NormalizeRepoRelativePath(value)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize revision path %q: %w", value, normalizeErr)
		}
		paths[index] = normalizedPath
	}

	return paths, nil
}

// ListAllFilesAtRev lists all tracked files at a revision with stable normalization.
func ListAllFilesAtRev(ctx context.Context, repoRoot string, rev string) ([]string, error) {
	exists, err := RevisionExists(ctx, repoRoot, rev)
	if err != nil {
		return nil, fmt.Errorf("check revision %q: %w", rev, err)
	}
	if !exists {
		return nil, nil
	}

	output, err := runGit(ctx, repoRoot, "ls-tree", "-r", "--name-only", "-z", rev)
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s: %w", rev, err)
	}

	paths := parseNULTerminated(output)
	for index, value := range paths {
		normalizedPath, normalizeErr := ids.NormalizeRepoRelativePath(value)
		if normalizeErr != nil {
			return nil, fmt.Errorf("normalize revision path %q: %w", value, normalizeErr)
		}
		paths[index] = normalizedPath
	}

	return paths, nil
}

// PathExistsAtRev reports whether a repo-relative file exists at a revision.
func PathExistsAtRev(ctx context.Context, repoRoot string, rev string, path string) (bool, error) {
	normalizedPath, err := ids.NormalizeRepoRelativePath(path)
	if err != nil {
		return false, fmt.Errorf("normalize revision path %q: %w", path, err)
	}

	paths, err := ListFilesAtRev(ctx, repoRoot, rev, normalizedPath)
	if err != nil {
		return false, err
	}

	for _, candidate := range paths {
		if candidate == normalizedPath {
			return true, nil
		}
	}

	return false, nil
}

// ReadFileAtRev reads a tracked file from the requested revision.
func ReadFileAtRev(ctx context.Context, repoRoot string, rev string, path string) ([]byte, error) {
	normalizedPath, err := ids.NormalizeRepoRelativePath(path)
	if err != nil {
		return nil, fmt.Errorf("normalize revision path %q: %w", path, err)
	}

	output, err := runGit(ctx, repoRoot, "show", rev+":"+normalizedPath)
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", rev, normalizedPath, err)
	}

	return output, nil
}

// FileModeAtRev reads the Git tree mode for one tracked file at the requested revision.
func FileModeAtRev(ctx context.Context, repoRoot string, rev string, path string) (string, error) {
	normalizedPath, err := ids.NormalizeRepoRelativePath(path)
	if err != nil {
		return "", fmt.Errorf("normalize revision path %q: %w", path, err)
	}

	output, err := runGit(ctx, repoRoot, "ls-tree", rev, "--", normalizedPath)
	if err != nil {
		return "", fmt.Errorf("git ls-tree %s -- %s: %w", rev, normalizedPath, err)
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return "", fmt.Errorf("path %s not found at revision %s", normalizedPath, rev)
	}

	fields := strings.Fields(line)
	if len(fields) < 4 {
		return "", fmt.Errorf("parse ls-tree output for %s at %s", normalizedPath, rev)
	}

	mode, err := normalizeFileMode(fields[0])
	if err != nil {
		return "", fmt.Errorf("parse file mode for %s at %s: %w", normalizedPath, rev, err)
	}

	return mode, nil
}

// ResolveRevision returns the commit hash for a revision expression.
func ResolveRevision(ctx context.Context, repoRoot string, rev string) (string, error) {
	trimmedRevision := strings.TrimSpace(rev)
	if trimmedRevision == "" {
		return "", errors.New("revision must not be empty")
	}

	output, err := runGit(ctx, repoRoot, "rev-parse", trimmedRevision)
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", trimmedRevision, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// ReadTrackedFileWorktreeOrHEAD reads a tracked file from the worktree when visible,
// and falls back to HEAD when the file is currently hidden by sparse-checkout.
func ReadTrackedFileWorktreeOrHEAD(ctx context.Context, repoRoot string, path string) ([]byte, error) {
	normalizedPath, err := ids.NormalizeRepoRelativePath(path)
	if err != nil {
		return nil, fmt.Errorf("normalize tracked path %q: %w", path, err)
	}

	worktreePath := filepath.Join(repoRoot, filepath.FromSlash(normalizedPath))
	//nolint:gosec // The destination path is repo-local and derived from a validated repo-relative path.
	data, worktreeErr := os.ReadFile(worktreePath)
	if worktreeErr == nil {
		return data, nil
	}
	if !errors.Is(worktreeErr, os.ErrNotExist) {
		return nil, fmt.Errorf("read worktree file %s: %w", worktreePath, worktreeErr)
	}

	exists, err := PathExistsAtRev(ctx, repoRoot, headRevision, normalizedPath)
	if err != nil {
		return nil, fmt.Errorf("check hidden control file %s at HEAD: %w", normalizedPath, err)
	}
	if !exists {
		return nil, fmt.Errorf("read worktree file %s: %w", worktreePath, worktreeErr)
	}

	skipped, err := PathIsSkipWorktree(ctx, repoRoot, normalizedPath)
	if err != nil {
		return nil, fmt.Errorf("check skip-worktree for %s: %w", normalizedPath, err)
	}
	if !skipped {
		return nil, fmt.Errorf("read worktree file %s: %w", worktreePath, worktreeErr)
	}

	data, err = ReadFileAtRev(ctx, repoRoot, headRevision, normalizedPath)
	if err != nil {
		return nil, fmt.Errorf("read hidden control file %s from HEAD: %w", normalizedPath, err)
	}

	return data, nil
}

// TrackedFileModeWorktreeOrHEAD reads a tracked file mode from the visible worktree
// and falls back to HEAD when the file is currently hidden by sparse-checkout.
func TrackedFileModeWorktreeOrHEAD(ctx context.Context, repoRoot string, path string) (string, error) {
	normalizedPath, err := ids.NormalizeRepoRelativePath(path)
	if err != nil {
		return "", fmt.Errorf("normalize tracked path %q: %w", path, err)
	}

	worktreePath := filepath.Join(repoRoot, filepath.FromSlash(normalizedPath))
	info, statErr := os.Stat(worktreePath)
	if statErr == nil {
		mode, err := fileModeFromWorktreeInfo(info)
		if err != nil {
			return "", fmt.Errorf("stat worktree file %s: %w", worktreePath, err)
		}

		return mode, nil
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("stat worktree file %s: %w", worktreePath, statErr)
	}

	exists, err := PathExistsAtRev(ctx, repoRoot, headRevision, normalizedPath)
	if err != nil {
		return "", fmt.Errorf("check hidden control file %s at HEAD: %w", normalizedPath, err)
	}
	if !exists {
		return "", fmt.Errorf("stat worktree file %s: %w", worktreePath, statErr)
	}

	skipped, err := PathIsSkipWorktree(ctx, repoRoot, normalizedPath)
	if err != nil {
		return "", fmt.Errorf("check skip-worktree for %s: %w", normalizedPath, err)
	}
	if !skipped {
		return "", fmt.Errorf("stat worktree file %s: %w", worktreePath, statErr)
	}

	mode, err := FileModeAtRev(ctx, repoRoot, headRevision, normalizedPath)
	if err != nil {
		return "", fmt.Errorf("read hidden control file mode %s from HEAD: %w", normalizedPath, err)
	}

	return mode, nil
}

// ReadFileWorktreeOrHEAD reads a control-plane file from the worktree when visible,
// and falls back to HEAD when the file is currently hidden by sparse-checkout.
func ReadFileWorktreeOrHEAD(ctx context.Context, repoRoot string, path string) ([]byte, error) {
	return ReadTrackedFileWorktreeOrHEAD(ctx, repoRoot, path)
}

// PathIsSkipWorktree reports whether a tracked path is currently hidden from the worktree projection.
func PathIsSkipWorktree(ctx context.Context, repoRoot string, path string) (bool, error) {
	normalizedPath, err := ids.NormalizeRepoRelativePath(path)
	if err != nil {
		return false, fmt.Errorf("normalize path %q: %w", path, err)
	}

	output, err := runGit(ctx, repoRoot, "ls-files", "-t", "--", normalizedPath)
	if err != nil {
		return false, fmt.Errorf("git ls-files -t -- %s: %w", normalizedPath, err)
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return false, nil
	}

	return strings.HasPrefix(line, "S "), nil
}
