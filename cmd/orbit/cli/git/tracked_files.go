package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// TrackedFiles returns the repository-relative tracked files from git ls-files -z.
func TrackedFiles(ctx context.Context, repoRoot string) ([]string, error) {
	output, err := runGit(ctx, repoRoot, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("git ls-files -z: %w", err)
	}

	parts := parseNULTerminated(output)
	trackedFiles := make([]string, 0, len(parts))

	for _, part := range parts {
		normalized, err := ids.NormalizeRepoRelativePath(part)
		if err != nil {
			return nil, fmt.Errorf("normalize tracked file %q: %w", part, err)
		}

		trackedFiles = append(trackedFiles, normalized)
	}

	sort.Strings(trackedFiles)

	return trackedFiles, nil
}

// WorktreeFiles returns repository-relative files visible to Git in the current
// worktree: tracked files plus untracked files that are not ignored.
func WorktreeFiles(ctx context.Context, repoRoot string) ([]string, error) {
	output, err := runGit(ctx, repoRoot, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("git ls-files -z --cached --others --exclude-standard: %w", err)
	}

	parts := parseNULTerminated(output)
	files := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		normalized, err := ids.NormalizeRepoRelativePath(part)
		if err != nil {
			return nil, fmt.Errorf("normalize worktree file %q: %w", part, err)
		}

		filename := filepath.Join(repoRoot, filepath.FromSlash(normalized))
		if _, err := os.Lstat(filename); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat worktree file %q: %w", normalized, err)
		}

		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		files = append(files, normalized)
	}

	sort.Strings(files)

	return files, nil
}

func parseNULTerminated(data []byte) []string {
	parts := bytes.Split(data, []byte{0})
	values := make([]string, 0, len(parts))

	for _, part := range parts {
		if len(part) == 0 {
			continue
		}

		values = append(values, string(part))
	}

	return values
}
