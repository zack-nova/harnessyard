package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

func repoFromAuthoringCommand(
	cmd *cobra.Command,
	repoLabel string,
	createCommand string,
	orbitID string,
	name string,
	description string,
) (gitpkg.Repo, error) {
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

	return gitpkg.Repo{}, fmt.Errorf(
		"current directory is not a Git repository\n"+
			"to start a new %s repo here, run:\n"+
			"  %s",
		repoLabel,
		buildAuthoringCreateSuggestion(createCommand, orbitID, name, description),
	)
}

func buildAuthoringCreateSuggestion(createCommand string, orbitID string, name string, description string) string {
	parts := []string{createCommand, "."}
	if strings.TrimSpace(orbitID) != "" {
		parts = append(parts, "--orbit", orbitID)
	} else {
		parts = append(parts, "--orbit", "<orbit-id>")
	}
	if strings.TrimSpace(name) != "" {
		parts = append(parts, "--name", fmt.Sprintf("%q", name))
	}
	if strings.TrimSpace(description) != "" {
		parts = append(parts, "--description", fmt.Sprintf("%q", description))
	}

	return strings.Join(parts, " ")
}
