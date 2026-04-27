package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

type filesOutput struct {
	RepoRoot string   `json:"repo_root"`
	Orbit    string   `json:"orbit"`
	Files    []string `json:"files"`
}

// NewFilesCommand creates the orbit files command.
func NewFilesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files [orbit-id]",
		Short: "List the resolved files for an orbit",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}
			requestedOrbitID := ""
			if len(args) > 0 {
				requestedOrbitID = args[0]
			}
			orbitID, err := resolveAuthoredTruthOrbitID(cmd, repo, requestedOrbitID)
			if err != nil {
				return err
			}

			config, err := loadValidatedRepositoryConfig(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}

			definition, err := definitionByID(config, orbitID)
			if err != nil {
				return err
			}

			trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), repo.Root)
			if err != nil {
				return fmt.Errorf("load tracked files: %w", err)
			}

			spec, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(cmd.Context(), repo.Root, config, definition.ID, trackedFiles)
			if err != nil {
				return fmt.Errorf("resolve projection plan: %w", err)
			}

			store, err := statepkg.NewFSStore(repo.GitDir)
			if err != nil {
				return fmt.Errorf("create state store: %w", err)
			}
			if err := store.WriteProjectionCache(definition.ID, plan.ProjectionPaths); err != nil {
				return fmt.Errorf("write projection cache: %w", err)
			}
			inventorySnapshot, err := orbitpkg.BuildFileInventorySnapshot(config, spec, plan, time.Now().UTC())
			if err != nil {
				return fmt.Errorf("build file inventory snapshot: %w", err)
			}
			if err := store.WriteFileInventorySnapshot(inventorySnapshot); err != nil {
				return fmt.Errorf("write file inventory snapshot: %w", err)
			}

			output := filesOutput{
				RepoRoot: repo.Root,
				Orbit:    definition.ID,
				Files:    plan.ProjectionPaths,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			for _, resolvedPath := range plan.ProjectionPaths {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), resolvedPath); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}
