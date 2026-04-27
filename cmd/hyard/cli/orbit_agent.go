package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type orbitAgentInspectOutput struct {
	RepoRoot string                            `json:"repo_root"`
	OrbitID  string                            `json:"orbit"`
	File     string                            `json:"file"`
	Hooks    []orbitpkg.ResolvedAgentAddonHook `json:"hooks,omitempty"`
}

func newOrbitAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Inspect agent add-ons declared by hosted orbit authored truth",
		Long: "Inspect package-scoped agent add-ons declared by hosted orbit authored truth.\n" +
			"This command is read-only; native agent activation remains under `hyard agent`.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newOrbitAgentInspectCommand(),
	)

	return cmd
}

func newOrbitAgentInspectCommand() *cobra.Command {
	var orbitID string

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect agent add-ons for an orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, definition, spec, config, err := loadHyardOrbitAgentSpec(cmd, orbitID)
			if err != nil {
				return err
			}
			trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), repo.Root)
			if err != nil {
				return fmt.Errorf("load tracked files: %w", err)
			}
			plan, err := orbitpkg.ResolveProjectionPlan(config, spec, trackedFiles)
			if err != nil {
				return fmt.Errorf("resolve projection plan: %w", err)
			}
			hooks, err := orbitpkg.ResolveAgentAddonHooks(spec, trackedFiles, plan.ExportPaths)
			if err != nil {
				return fmt.Errorf("resolve agent add-ons: %w", err)
			}

			output := orbitAgentInspectOutput{
				RepoRoot: repo.Root,
				OrbitID:  definition.ID,
				File:     spec.SourcePath,
				Hooks:    hooks,
			}
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit: %s\n", output.OrbitID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if len(output.Hooks) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "agent hooks: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				return nil
			}
			for _, hook := range output.Hooks {
				targets := agentAddonHookTargetList(hook.Targets)
				if targets == "" {
					targets = "all"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent hook: %s event=%s handler=%s targets=%s activation=not_applied\n", hook.DisplayID, hook.EventKind, hook.HandlerPath, targets); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	addHyardJSONFlag(cmd)

	return cmd
}

func loadHyardOrbitAgentSpec(cmd *cobra.Command, requestedOrbitID string) (gitpkg.Repo, orbitpkg.Definition, orbitpkg.OrbitSpec, orbitpkg.RepositoryConfig, error) {
	repo, definition, spec, err := loadHyardOrbitSkillSpec(cmd, requestedOrbitID)
	if err != nil {
		message := strings.Replace(err.Error(), "orbit skill command", "orbit agent command", 1)
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, orbitpkg.RepositoryConfig{}, fmt.Errorf("%s", message)
	}
	config, err := orbitpkg.LoadHostedRepositoryConfig(cmd.Context(), repo.Root)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, orbitpkg.RepositoryConfig{}, fmt.Errorf("load hosted repository config: %w", err)
	}

	return repo, definition, spec, config, nil
}

func agentAddonHookTargetList(targets map[string]bool) string {
	values := make([]string, 0, len(targets))
	for target, enabled := range targets {
		if !enabled {
			continue
		}
		values = append(values, target)
	}
	sort.Strings(values)

	return strings.Join(values, ",")
}
