package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

type currentOutput struct {
	RepoRoot    string                      `json:"repo_root"`
	Current     *statepkg.CurrentOrbitState `json:"current"`
	Stale       bool                        `json:"stale"`
	StaleReason string                      `json:"stale_reason,omitempty"`
	Warning     string                      `json:"warning,omitempty"`
}

// NewCurrentCommand creates the orbit current command.
func NewCurrentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show the current orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			store, err := statepkg.NewFSStore(repo.GitDir)
			if err != nil {
				return fmt.Errorf("create state store: %w", err)
			}

			current, err := store.ReadCurrentOrbit()
			if err != nil && !errors.Is(err, statepkg.ErrCurrentOrbitNotFound) {
				return fmt.Errorf("read current orbit: %w", err)
			}

			var currentPtr *statepkg.CurrentOrbitState
			if err == nil {
				currentPtr = &current
			}

			stale := false
			staleReason := ""
			warning := ""
			if currentPtr != nil {
				config, err := loadValidatedRepositoryConfig(cmd.Context(), repo.Root)
				switch {
				case errors.Is(err, errOrbitNotInitialized):
					stale = true
					staleReason = "missing_configuration"
					warning = "stale current orbit state: orbit configuration is missing"
				case err != nil:
					return err
				default:
					definition, found := config.OrbitByID(currentPtr.Orbit)
					if !found {
						stale = true
						staleReason = "missing_orbit"
						warning = fmt.Sprintf("stale current orbit state: orbit %q is missing", currentPtr.Orbit)
					} else {
						trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), repo.Root)
						if err != nil {
							return fmt.Errorf("load tracked files: %w", err)
						}
						_, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(
							cmd.Context(),
							repo.Root,
							config,
							definition.ID,
							trackedFiles,
						)
						if err != nil {
							return fmt.Errorf("load current orbit runtime plan: %w", err)
						}
						staleness, err := viewpkg.DetectCurrentRuntimeLedgerPlanStaleness(store, currentPtr.Orbit, plan.PlanHash)
						if err != nil {
							return fmt.Errorf("detect current runtime ledger staleness: %w", err)
						}
						if staleness.Stale {
							stale = true
							staleReason = staleness.Reason
							warning = "stale current orbit state: " + staleness.Detail
						}
					}
				}
			}

			output := currentOutput{
				RepoRoot:    repo.Root,
				Current:     currentPtr,
				Stale:       stale,
				StaleReason: staleReason,
				Warning:     warning,
			}

			jsonOutput, flagErr := wantJSON(cmd)
			if flagErr != nil {
				return flagErr
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if currentPtr == nil {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "no current orbit")
			} else if stale {
				if _, err = fmt.Fprintln(cmd.OutOrStdout(), currentPtr.Orbit); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warning)
			} else {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), currentPtr.Orbit)
			}
			if err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}
