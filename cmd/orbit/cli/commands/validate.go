package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type validateOrbitOutput struct {
	ID         string   `json:"id"`
	SourcePath string   `json:"source_path"`
	ScopeCount int      `json:"scope_count"`
	Warnings   []string `json:"warnings"`
}

type validateOutput struct {
	RepoRoot string                `json:"repo_root"`
	Valid    bool                  `json:"valid"`
	Orbits   []validateOrbitOutput `json:"orbits"`
}

// NewValidateCommand creates the orbit validate command.
func NewValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate Orbit configuration and scope resolution",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			config, err := loadValidatedVisibleOrbitConfig(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}
			if err := orbittemplate.ValidateRuntimeAgentsFile(repo.Root); err != nil {
				return fmt.Errorf("validate runtime AGENTS.md: %w", err)
			}

			trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), repo.Root)
			if err != nil {
				return fmt.Errorf("load tracked files: %w", err)
			}

			output := validateOutput{
				RepoRoot: repo.Root,
				Valid:    true,
				Orbits:   make([]validateOrbitOutput, 0, len(config.Orbits)),
			}

			for _, definition := range config.Orbits {
				spec, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(cmd.Context(), repo.Root, config, definition.ID, trackedFiles)
				if err != nil {
					return fmt.Errorf("resolve projection scope for orbit %q: %w", definition.ID, err)
				}
				if err := orbitpkg.PreflightResolvedCapabilities(repo.Root, spec, trackedFiles, plan.ExportPaths); err != nil {
					return fmt.Errorf("resolve projection scope for orbit %q: %w", definition.ID, err)
				}

				result := validateOrbitOutput{
					ID:         definition.ID,
					SourcePath: spec.SourcePath,
					ScopeCount: len(plan.ProjectionPaths),
				}
				if len(plan.SubjectPaths)+len(plan.RulePaths)+len(plan.ProcessPaths) == 0 {
					result.Warnings = append(result.Warnings, "projection scope is empty")
				}

				output.Orbits = append(output.Orbits, result)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if len(output.Orbits) == 0 {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "valid: 0 orbit(s)")
				if err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				return nil
			}

			hasWarnings := false
			for _, result := range output.Orbits {
				if len(result.Warnings) > 0 {
					hasWarnings = true
					break
				}
			}

			if hasWarnings {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "valid with warnings: %d orbit(s)\n", len(output.Orbits))
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "valid: %d orbit(s)\n", len(output.Orbits))
			}
			if err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			for _, result := range output.Orbits {
				line := fmt.Sprintf("ok %s %d file(s)", result.ID, result.ScopeCount)
				if len(result.Warnings) > 0 {
					line = fmt.Sprintf("warn %s %s", result.ID, result.Warnings[0])
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}
