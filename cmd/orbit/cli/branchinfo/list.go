package branchinfo

import (
	"context"
	"fmt"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

// ListedBranch captures the stable classification summary for one local branch.
type ListedBranch struct {
	Name           string         `json:"name"`
	Classification Classification `json:"classification"`
}

// ListLocalBranches enumerates and classifies local branches using the shared classifier.
func ListLocalBranches(ctx context.Context, repoRoot string) ([]ListedBranch, error) {
	branchNames, err := gitpkg.ListLocalBranches(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("enumerate local branches: %w", err)
	}

	listedBranches := make([]ListedBranch, 0, len(branchNames))
	for _, branchName := range branchNames {
		classification, err := ClassifyRevision(ctx, repoRoot, branchName)
		if err != nil {
			return nil, fmt.Errorf("classify branch %q: %w", branchName, err)
		}

		listedBranches = append(listedBranches, ListedBranch{
			Name:           branchName,
			Classification: classification,
		})
	}

	return listedBranches, nil
}
