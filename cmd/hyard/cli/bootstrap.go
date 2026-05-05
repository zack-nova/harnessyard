package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type hyardBootstrapCompleteOutput struct {
	RepoRoot               string   `json:"repo_root"`
	Check                  bool     `json:"check"`
	Completed              bool     `json:"completed"`
	CompletedOrbits        []string `json:"completed_orbits,omitempty"`
	AlreadyCompletedOrbits []string `json:"already_completed_orbits,omitempty"`
	RemovedPaths           []string `json:"removed_paths,omitempty"`
	RemovedBootstrapBlocks []string `json:"removed_bootstrap_blocks,omitempty"`
	DeletedBootstrapFile   bool     `json:"deleted_bootstrap_file"`
	AutoLeftCurrentOrbit   bool     `json:"auto_left_current_orbit"`
}

type hyardBootstrapReopenOutput struct {
	RepoRoot                string   `json:"repo_root"`
	ReopenedOrbits          []string `json:"reopened_orbits,omitempty"`
	AlreadyPendingOrbits    []string `json:"already_pending_orbits,omitempty"`
	RestoredPaths           []string `json:"restored_paths,omitempty"`
	RestoredBootstrapBlocks []string `json:"restored_bootstrap_blocks,omitempty"`
	CreatedBootstrapFile    bool     `json:"created_bootstrap_file"`
}

type hyardBootstrapSetupOutput struct {
	RepoRoot         string `json:"repo_root"`
	Check            bool   `json:"check"`
	Framework        string `json:"framework"`
	ResolutionSource string `json:"resolution_source"`
	SkillName        string `json:"skill_name"`
	SkillRoot        string `json:"skill_root"`
	SkillPath        string `json:"skill_path"`
	Action           string `json:"action"`
	Changed          bool   `json:"changed"`
	Remove           bool   `json:"remove"`
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
		newBootstrapSetupCommand(),
	)

	return cmd
}

func newBootstrapSetupCommand() *cobra.Command {
	var framework string
	var check bool
	var force bool
	var remove bool

	cmd := &cobra.Command{
		Use:   "setup [framework]",
		Short: "Set up repository bootstrap agent skill",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			resolvedFramework, err := resolveHyardBootstrapSetupFrameworkArgument(args, framework)
			if err != nil {
				return err
			}
			input, err := hyardBootstrapSetupInput(cmd, resolvedFramework, force, remove)
			if err != nil {
				return err
			}

			var plan harnesspkg.BootstrapAgentSkillSetupPlan
			if check {
				plan, err = harnesspkg.PlanBootstrapAgentSkillSetup(input)
			} else {
				plan, err = harnesspkg.ApplyBootstrapAgentSkillSetup(input)
			}
			if err != nil {
				return err
			}
			output := hyardBootstrapSetupOutputFromPlan(plan, check)
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			return printHyardBootstrapSetupOutput(cmd, output)
		},
	}
	cmd.Flags().StringVar(&framework, "framework", "", "Target framework: codex, claude, or openclaw")
	cmd.Flags().BoolVar(&check, "check", false, "Preview bootstrap skill setup without writing files")
	cmd.Flags().BoolVar(&force, "force", false, "Replace or remove an edited bootstrap skill")
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove the bootstrap skill for the target framework")
	addHyardJSONFlag(cmd)

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

	runtimeResult, err := harnesspkg.CompleteRuntimeBootstrap(cmd.Context(), resolved.Repo, harnesspkg.BootstrapCompleteInput{
		All:                         true,
		Now:                         time.Now().UTC(),
		AllowDirtyBootstrapArtifact: true,
	})
	if err != nil {
		return hyardBootstrapCompleteOutput{}, fmt.Errorf("complete runtime bootstrap: %w", err)
	}

	return hyardBootstrapCompleteOutput{
		RepoRoot:               resolved.Repo.Root,
		Check:                  false,
		Completed:              true,
		CompletedOrbits:        append([]string(nil), runtimeResult.CompletedOrbits...),
		AlreadyCompletedOrbits: append([]string(nil), runtimeResult.AlreadyCompletedOrbits...),
		RemovedPaths:           append([]string(nil), runtimeResult.RemovedPaths...),
		RemovedBootstrapBlocks: append([]string(nil), runtimeResult.RemovedBootstrapBlocks...),
		DeletedBootstrapFile:   runtimeResult.DeletedBootstrapFile,
		AutoLeftCurrentOrbit:   runtimeResult.AutoLeftCurrentOrbit,
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

	runtimeResult, err := harnesspkg.ReopenRuntimeBootstrap(cmd.Context(), resolved.Repo, harnesspkg.BootstrapReopenInput{
		All:            true,
		RestoreSurface: restoreSurface,
		Now:            time.Now().UTC(),
	})
	if err != nil {
		return hyardBootstrapReopenOutput{}, fmt.Errorf("reopen runtime bootstrap: %w", err)
	}

	return hyardBootstrapReopenOutput{
		RepoRoot:                resolved.Repo.Root,
		ReopenedOrbits:          append([]string(nil), runtimeResult.ReopenedOrbits...),
		AlreadyPendingOrbits:    append([]string(nil), runtimeResult.AlreadyPendingOrbits...),
		RestoredPaths:           append([]string(nil), runtimeResult.RestoredPaths...),
		RestoredBootstrapBlocks: append([]string(nil), runtimeResult.RestoredBootstrapBlocks...),
		CreatedBootstrapFile:    runtimeResult.CreatedBootstrapFile,
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
	return hyardBootstrapCompleteOutput{
		RepoRoot:               resolved.Repo.Root,
		Check:                  check,
		Completed:              false,
		CompletedOrbits:        append([]string(nil), runtimePlan.CompletedOrbits...),
		AlreadyCompletedOrbits: append([]string(nil), runtimePlan.AlreadyCompletedOrbits...),
		RemovedPaths:           append([]string(nil), runtimePlan.RemovedPaths...),
		RemovedBootstrapBlocks: append([]string(nil), runtimePlan.RemovedBootstrapBlocks...),
		DeletedBootstrapFile:   runtimePlan.DeletedBootstrapFile,
		AutoLeftCurrentOrbit:   runtimePlan.AutoLeftCurrentOrbit,
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

func resolveHyardBootstrapSetupFrameworkArgument(args []string, frameworkFlag string) (string, error) {
	argFramework := ""
	if len(args) > 0 {
		argFramework = args[0]
	}
	if argFramework == "" {
		return frameworkFlag, nil
	}
	if frameworkFlag == "" {
		return argFramework, nil
	}
	argNormalized, argOK := harnesspkg.NormalizeFrameworkID(argFramework)
	flagNormalized, flagOK := harnesspkg.NormalizeFrameworkID(frameworkFlag)
	if !argOK || !flagOK {
		return "", fmt.Errorf("framework argument and --framework must both be supported framework ids")
	}
	if argNormalized != flagNormalized {
		return "", fmt.Errorf("framework argument %q conflicts with --framework %q", argFramework, frameworkFlag)
	}

	return argNormalized, nil
}

func hyardBootstrapSetupInput(cmd *cobra.Command, framework string, force bool, remove bool) (harnesspkg.BootstrapAgentSkillSetupInput, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return harnesspkg.BootstrapAgentSkillSetupInput{}, err
	}
	resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
	if err != nil {
		return harnesspkg.BootstrapAgentSkillSetupInput{}, fmt.Errorf("resolve harness root: %w", err)
	}

	return harnesspkg.BootstrapAgentSkillSetupInput{
		RepoRoot:  resolved.Repo.Root,
		GitDir:    resolved.Repo.GitDir,
		Framework: framework,
		Force:     force,
		Remove:    remove,
	}, nil
}

func hyardBootstrapSetupOutputFromPlan(plan harnesspkg.BootstrapAgentSkillSetupPlan, check bool) hyardBootstrapSetupOutput {
	return hyardBootstrapSetupOutput{
		RepoRoot:         plan.RepoRoot,
		Check:            check,
		Framework:        plan.Framework,
		ResolutionSource: string(plan.ResolutionSource),
		SkillName:        plan.SkillName,
		SkillRoot:        plan.SkillRoot,
		SkillPath:        plan.SkillPath,
		Action:           plan.Action,
		Changed:          plan.Changed,
		Remove:           plan.Remove,
	}
}

func printHyardBootstrapSetupOutput(cmd *cobra.Command, output hyardBootstrapSetupOutput) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "bootstrap skill %s for %s at %s\n", output.Action, output.Framework, output.SkillPath); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "repo_root: %s\n", output.RepoRoot); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	return nil
}
