package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type hyardBootstrapCompleteOutput struct {
	RepoRoot                string   `json:"repo_root"`
	Check                   bool     `json:"check"`
	Completed               bool     `json:"completed"`
	CompletedOrbits         []string `json:"completed_orbits,omitempty"`
	AlreadyCompletedOrbits  []string `json:"already_completed_orbits,omitempty"`
	RemovedPaths            []string `json:"removed_paths,omitempty"`
	RemovedRepoBlocks       []string `json:"removed_repo_blocks,omitempty"`
	RemovedBootstrapBlocks  []string `json:"removed_bootstrap_blocks,omitempty"`
	DeletedBootstrapFile    bool     `json:"deleted_bootstrap_file"`
	DeletedAgentsFile       bool     `json:"deleted_agents_file"`
	RuntimeDeletedBootstrap bool     `json:"runtime_deleted_bootstrap_file"`
	AutoLeftCurrentOrbit    bool     `json:"auto_left_current_orbit"`
}

type hyardBootstrapReopenOutput struct {
	RepoRoot                string   `json:"repo_root"`
	ReopenedOrbits          []string `json:"reopened_orbits,omitempty"`
	AlreadyPendingOrbits    []string `json:"already_pending_orbits,omitempty"`
	RestoredPaths           []string `json:"restored_paths,omitempty"`
	RestoredRepoBlocks      []string `json:"restored_repo_blocks,omitempty"`
	RestoredBootstrapBlocks []string `json:"restored_bootstrap_blocks,omitempty"`
	CreatedBootstrapFile    bool     `json:"created_bootstrap_file"`
	CreatedAgentsFile       bool     `json:"created_agents_file"`
}

func newBootstrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Manage repository bootstrap lifecycle",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		newBootstrapCompleteCommand(),
		newBootstrapReopenCommand(),
	)

	return cmd
}

func newBootstrapCompleteCommand() *cobra.Command {
	var check bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "complete",
		Short: "Close repository bootstrap after initialization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}

			if check {
				output, err := buildHyardBootstrapCompleteOutput(cmd, check)
				if err != nil {
					return err
				}
				if jsonOutput {
					return emitHyardJSON(cmd, output)
				}
				return printHyardBootstrapCompleteOutput(cmd, output)
			}
			if !yes {
				return fmt.Errorf("hyard bootstrap complete requires --yes")
			}

			output, err := runHyardBootstrapComplete(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}
			return printHyardBootstrapCompleteOutput(cmd, output)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Preview repository bootstrap closeout without modifying files")
	cmd.Flags().BoolVar(&yes, "yes", false, "Close repository bootstrap without interactive confirmation")
	addHyardJSONFlag(cmd)

	return cmd
}

func newBootstrapReopenCommand() *cobra.Command {
	var restoreSurface bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "reopen",
		Short: "Reopen repository bootstrap",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, err := runHyardBootstrapReopen(cmd, restoreSurface)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}
			return printHyardBootstrapReopenOutput(cmd, output)
		},
	}
	cmd.Flags().BoolVar(&restoreSurface, "restore-surface", false, "Also restore orbit bootstrap blocks and bootstrap-lane runtime files when possible")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output machine-readable JSON")

	return cmd
}

func runHyardBootstrapComplete(cmd *cobra.Command) (hyardBootstrapCompleteOutput, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return hyardBootstrapCompleteOutput{}, err
	}
	resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
	if err != nil {
		return hyardBootstrapCompleteOutput{}, fmt.Errorf("resolve harness root: %w", err)
	}

	if _, err := harnesspkg.PlanRepoBootstrapCloseout(resolved.Repo.Root); err != nil {
		return hyardBootstrapCompleteOutput{}, fmt.Errorf("plan repo bootstrap closeout: %w", err)
	}

	runtimeResult, err := harnesspkg.CompleteRuntimeBootstrap(cmd.Context(), resolved.Repo, harnesspkg.BootstrapCompleteInput{
		All:                         true,
		Now:                         time.Now().UTC(),
		AllowDirtyBootstrapArtifact: true,
	})
	if err != nil {
		return hyardBootstrapCompleteOutput{}, fmt.Errorf("complete runtime bootstrap: %w", err)
	}
	repoResult, err := harnesspkg.ApplyRepoBootstrapCloseout(resolved.Repo.Root)
	if err != nil {
		return hyardBootstrapCompleteOutput{}, fmt.Errorf("close repo bootstrap guidance: %w", err)
	}

	return hyardBootstrapCompleteOutput{
		RepoRoot:                resolved.Repo.Root,
		Check:                   false,
		Completed:               true,
		CompletedOrbits:         append([]string(nil), runtimeResult.CompletedOrbits...),
		AlreadyCompletedOrbits:  append([]string(nil), runtimeResult.AlreadyCompletedOrbits...),
		RemovedPaths:            uniqueSortedStrings(append(append([]string(nil), runtimeResult.RemovedPaths...), repoResult.RemovedPaths...)),
		RemovedRepoBlocks:       append([]string(nil), repoResult.RemovedBlocks...),
		RemovedBootstrapBlocks:  append([]string(nil), runtimeResult.RemovedBootstrapBlocks...),
		DeletedBootstrapFile:    repoResult.DeletedBootstrapFile,
		DeletedAgentsFile:       repoResult.DeletedAgentsFile,
		RuntimeDeletedBootstrap: runtimeResult.DeletedBootstrapFile,
		AutoLeftCurrentOrbit:    runtimeResult.AutoLeftCurrentOrbit,
	}, nil
}

func runHyardBootstrapReopen(cmd *cobra.Command, restoreSurface bool) (hyardBootstrapReopenOutput, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return hyardBootstrapReopenOutput{}, err
	}
	resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
	if err != nil {
		return hyardBootstrapReopenOutput{}, fmt.Errorf("resolve harness root: %w", err)
	}

	if _, err := harnesspkg.PlanRepoBootstrapReopen(resolved.Repo.Root); err != nil {
		return hyardBootstrapReopenOutput{}, fmt.Errorf("plan repo bootstrap reopen: %w", err)
	}

	runtimeResult, err := harnesspkg.ReopenRuntimeBootstrap(cmd.Context(), resolved.Repo, harnesspkg.BootstrapReopenInput{
		All:            true,
		RestoreSurface: restoreSurface,
		Now:            time.Now().UTC(),
	})
	if err != nil {
		return hyardBootstrapReopenOutput{}, fmt.Errorf("reopen runtime bootstrap: %w", err)
	}
	repoResult, err := harnesspkg.ApplyRepoBootstrapReopen(resolved.Repo.Root)
	if err != nil {
		return hyardBootstrapReopenOutput{}, fmt.Errorf("restore repo bootstrap guidance: %w", err)
	}

	return hyardBootstrapReopenOutput{
		RepoRoot:                resolved.Repo.Root,
		ReopenedOrbits:          append([]string(nil), runtimeResult.ReopenedOrbits...),
		AlreadyPendingOrbits:    append([]string(nil), runtimeResult.AlreadyPendingOrbits...),
		RestoredPaths:           uniqueSortedStrings(append(append([]string(nil), runtimeResult.RestoredPaths...), repoResult.RestoredPaths...)),
		RestoredRepoBlocks:      append([]string(nil), repoResult.RestoredBlocks...),
		RestoredBootstrapBlocks: append([]string(nil), runtimeResult.RestoredBootstrapBlocks...),
		CreatedBootstrapFile:    runtimeResult.CreatedBootstrapFile || repoResult.CreatedBootstrapFile,
		CreatedAgentsFile:       repoResult.CreatedAgentsFile,
	}, nil
}

func buildHyardBootstrapCompleteOutput(cmd *cobra.Command, check bool) (hyardBootstrapCompleteOutput, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return hyardBootstrapCompleteOutput{}, err
	}
	resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
	if err != nil {
		return hyardBootstrapCompleteOutput{}, fmt.Errorf("resolve harness root: %w", err)
	}

	runtimePlan, err := harnesspkg.PlanRuntimeBootstrapCompletion(cmd.Context(), resolved.Repo, harnesspkg.BootstrapCompleteInput{
		All:                         true,
		Now:                         time.Now().UTC(),
		AllowDirtyBootstrapArtifact: true,
	})
	if err != nil {
		return hyardBootstrapCompleteOutput{}, fmt.Errorf("plan runtime bootstrap completion: %w", err)
	}
	repoPlan, err := harnesspkg.PlanRepoBootstrapCloseout(resolved.Repo.Root)
	if err != nil {
		return hyardBootstrapCompleteOutput{}, fmt.Errorf("plan repo bootstrap closeout: %w", err)
	}

	return hyardBootstrapCompleteOutput{
		RepoRoot:               resolved.Repo.Root,
		Check:                  check,
		Completed:              false,
		CompletedOrbits:        append([]string(nil), runtimePlan.CompletedOrbits...),
		AlreadyCompletedOrbits: append([]string(nil), runtimePlan.AlreadyCompletedOrbits...),
		RemovedPaths: uniqueSortedStrings(append(
			append([]string(nil), runtimePlan.RemovedPaths...),
			repoPlan.RemovedPaths...,
		)),
		RemovedRepoBlocks:       append([]string(nil), repoPlan.RemovedBlocks...),
		RemovedBootstrapBlocks:  append([]string(nil), runtimePlan.RemovedBootstrapBlocks...),
		DeletedBootstrapFile:    repoPlan.DeletedBootstrapFile,
		DeletedAgentsFile:       repoPlan.DeletedAgentsFile,
		RuntimeDeletedBootstrap: runtimePlan.DeletedBootstrapFile,
		AutoLeftCurrentOrbit:    runtimePlan.AutoLeftCurrentOrbit,
	}, nil
}

func printHyardBootstrapCompleteOutput(cmd *cobra.Command, output hyardBootstrapCompleteOutput) error {
	action := "checked repository bootstrap closeout"
	if output.Completed {
		action = "completed repository bootstrap"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s at %s\n", action, output.RepoRoot); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, orbitID := range output.CompletedOrbits {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "pending_orbit: %s\n", orbitID); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, path := range output.RemovedPaths {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "remove_path: %s\n", path); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func printHyardBootstrapReopenOutput(cmd *cobra.Command, output hyardBootstrapReopenOutput) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "reopened repository bootstrap at %s\n", output.RepoRoot); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, orbitID := range output.ReopenedOrbits {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "reopened_orbit: %s\n", orbitID); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, path := range output.RestoredPaths {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "restored_path: %s\n", path); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	unique := make([]string, 0, len(seen))
	for value := range seen {
		unique = append(unique, value)
	}
	sort.Strings(unique)

	return unique
}
