package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

func repoFromRuntimeInitCommand(cmd *cobra.Command, createCommand string) (gitpkg.Repo, error) {
	workingDir, err := workingDirFromCommand(cmd)
	if err != nil {
		return gitpkg.Repo{}, err
	}

	repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
	if err == nil {
		return repo, nil
	}
	if !gitpkg.IsNotGitRepositoryError(err) {
		return gitpkg.Repo{}, fmt.Errorf("discover git repository: %w", err)
	}

	createTarget := "."
	return gitpkg.Repo{}, fmt.Errorf(
		"current directory is not a Git repository\n"+
			"to start a new harness runtime repo here, run:\n"+
			"  %s %s",
		createCommand,
		createTarget,
	)
}
