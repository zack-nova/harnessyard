package git

import (
	"context"
	"fmt"
	"strings"
)

// ListLocalBranches returns local branch names in stable refname order.
func ListLocalBranches(ctx context.Context, repoRoot string) ([]string, error) {
	output, err := runGit(
		ctx,
		repoRoot,
		"for-each-ref",
		"--format=%(refname:short)",
		"--sort=refname",
		"refs/heads",
	)
	if err != nil {
		return nil, fmt.Errorf("git for-each-ref refs/heads: %w", err)
	}

	trimmedOutput := strings.TrimSpace(string(output))
	if trimmedOutput == "" {
		return nil, nil
	}

	branches := strings.Split(trimmedOutput, "\n")
	for index, branch := range branches {
		branches[index] = strings.TrimSpace(branch)
	}

	return branches, nil
}
