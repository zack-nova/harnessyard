package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// StatusEntry is a parsed entry from git status --porcelain=v1 -z -uall.
type StatusEntry struct {
	Path    string
	Code    string
	Tracked bool
	Staged  bool
}

// WorktreeStatus returns parsed porcelain status entries for the repository worktree.
func WorktreeStatus(ctx context.Context, repoRoot string) ([]StatusEntry, error) {
	output, err := runGit(ctx, repoRoot, "status", "--porcelain=v1", "-z", "-uall")
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain=v1 -z -uall: %w", err)
	}

	entries, err := parsePorcelainV1Z(output)
	if err != nil {
		return nil, fmt.Errorf("parse porcelain status: %w", err)
	}

	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Path == entries[right].Path {
			return entries[left].Code < entries[right].Code
		}

		return entries[left].Path < entries[right].Path
	})

	return entries, nil
}

// WorktreeMatchesIndex reports whether the current worktree bytes already match
// the index. This is useful for unborn repositories where staged additions form
// the tracked baseline even though porcelain status still reports them as added.
func WorktreeMatchesIndex(ctx context.Context, repoRoot string) (bool, error) {
	// #nosec G204 -- repoRoot is the resolved repository root and is passed as a distinct git argument.
	command := exec.CommandContext(ctx, "git", "-C", repoRoot, "diff", "--quiet", "--exit-code", "--")
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("git diff --quiet --exit-code --: %w", err)
	}

	return true, nil
}

func parsePorcelainV1Z(output []byte) ([]StatusEntry, error) {
	if len(output) == 0 {
		return []StatusEntry{}, nil
	}

	parts := bytes.Split(output, []byte{0})
	entries := make([]StatusEntry, 0, len(parts))

	for index := 0; index < len(parts); index++ {
		part := parts[index]
		if len(part) == 0 {
			continue
		}
		if len(part) < 4 {
			return nil, fmt.Errorf("invalid porcelain entry %q", string(part))
		}

		status := part[:2]
		pathValue := string(part[3:])
		if isRenameOrCopy(status) {
			index++
			if index >= len(parts) || len(parts[index]) == 0 {
				return nil, errors.New("missing rename destination in porcelain output")
			}
			pathValue = string(parts[index])
		}

		normalizedPath, err := ids.NormalizeRepoRelativePath(pathValue)
		if err != nil {
			return nil, fmt.Errorf("normalize status path %q: %w", pathValue, err)
		}

		code := summarizePorcelainCode(status)
		entries = append(entries, StatusEntry{
			Path:    normalizedPath,
			Code:    code,
			Tracked: code != "??",
			Staged:  status[0] != ' ' && status[0] != '?',
		})
	}

	return entries, nil
}

func summarizePorcelainCode(status []byte) string {
	if len(status) != 2 {
		return strings.TrimSpace(string(status))
	}
	if status[0] == '?' && status[1] == '?' {
		return "??"
	}

	for _, value := range status {
		switch value {
		case 'R', 'C':
			return "R"
		case 'D':
			return "D"
		case 'A':
			return "A"
		case 'M':
			return "M"
		case 'U':
			return "U"
		}
	}

	return strings.TrimSpace(string(status))
}

func isRenameOrCopy(status []byte) bool {
	if len(status) != 2 {
		return false
	}

	return status[0] == 'R' || status[1] == 'R' || status[0] == 'C' || status[1] == 'C'
}
