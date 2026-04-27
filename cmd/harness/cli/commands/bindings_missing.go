package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type bindingsMissingVariableJSON struct {
	Name                      string `json:"name"`
	Namespace                 string `json:"namespace,omitempty"`
	Description               string `json:"description,omitempty"`
	Required                  bool   `json:"required"`
	HasValue                  bool   `json:"has_value"`
	ObservedRuntimeUnresolved bool   `json:"observed_runtime_unresolved"`
	Missing                   bool   `json:"missing"`
}

type bindingsMissingOrbitJSON struct {
	OrbitID         string                        `json:"orbit_id"`
	SnapshotMissing bool                          `json:"snapshot_missing,omitempty"`
	DeclaredCount   int                           `json:"declared_count"`
	MissingCount    int                           `json:"missing_count"`
	Variables       []bindingsMissingVariableJSON `json:"variables"`
}

type bindingsMissingJSON struct {
	HarnessRoot     string                     `json:"harness_root"`
	HarnessID       string                     `json:"harness_id"`
	OrbitCount      int                        `json:"orbit_count"`
	OrbitIDs        []string                   `json:"orbit_ids"`
	VariableCount   int                        `json:"variable_count"`
	MissingCount    int                        `json:"missing_count"`
	ReadinessReason string                     `json:"readiness_reason"`
	Orbits          []bindingsMissingOrbitJSON `json:"orbits"`
}

// NewBindingsMissingCommand creates the harness bindings missing command.
func NewBindingsMissingCommand() *cobra.Command {
	var orbitID string
	var all bool

	cmd := &cobra.Command{
		Use:   "missing",
		Short: "Inspect which declared install bindings are still missing from current runtime vars",
		Long: "Inspect which declared install bindings are still missing from the current .harness/vars.yaml file.\n" +
			"This command only targets install-backed orbits and distinguishes declared contract gaps from runtime placeholder observations.",
		Example: "" +
			"  harness bindings missing --orbit docs\n" +
			"  harness bindings missing --all --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			result, err := harnesspkg.InspectMissingBindings(cmd.Context(), harnesspkg.MissingBindingsInput{
				RepoRoot: resolved.Repo.Root,
				OrbitID:  orbitID,
				All:      all,
			})
			if err != nil {
				return fmt.Errorf("inspect missing bindings: %w", err)
			}

			payload := bindingsMissingPayload(resolved.Repo.Root, result)
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), payload)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_root: %s\n", payload.HarnessRoot); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", payload.HarnessID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_count: %d\n", payload.OrbitCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "variable_count: %d\n", payload.VariableCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "missing_count: %d\n", payload.MissingCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "readiness_reason: %s\n", payload.ReadinessReason); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if len(payload.OrbitIDs) == 0 {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "orbits: none")
				if err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				return nil
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbits: %s\n", strings.Join(payload.OrbitIDs, ", ")); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, orbit := range payload.Orbits {
				if orbit.SnapshotMissing {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit: %s (snapshot: missing)\n", orbit.OrbitID); err != nil {
						return fmt.Errorf("write command output: %w", err)
					}
					continue
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit: %s (missing: %d/%d)\n", orbit.OrbitID, orbit.MissingCount, orbit.DeclaredCount); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				for _, variable := range orbit.Variables {
					state := "present"
					if variable.Missing {
						state = "missing"
					}
					required := "optional"
					if variable.Required {
						required = "required"
					}
					namespace := ""
					if variable.Namespace != "" {
						namespace = fmt.Sprintf(", namespace=%s", variable.Namespace)
					}
					observed := ""
					if variable.ObservedRuntimeUnresolved {
						observed = ", observed_runtime_unresolved"
					}
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s [%s, %s%s%s]\n", variable.Name, required, state, namespace, observed); err != nil {
						return fmt.Errorf("write command output: %w", err)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&orbitID, "orbit", "", "Inspect one install-backed orbit")
	cmd.Flags().BoolVar(&all, "all", false, "Inspect all install-backed orbits in the current runtime")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}

func bindingsMissingPayload(repoRoot string, result harnesspkg.MissingBindingsResult) bindingsMissingJSON {
	payload := bindingsMissingJSON{
		HarnessRoot:     repoRoot,
		HarnessID:       result.HarnessID,
		OrbitCount:      len(result.Orbits),
		OrbitIDs:        make([]string, 0, len(result.Orbits)),
		ReadinessReason: string(harnesspkg.ReadinessReasonUnresolvedRequiredBindings),
		Orbits:          make([]bindingsMissingOrbitJSON, 0, len(result.Orbits)),
	}

	for _, orbit := range result.Orbits {
		payload.OrbitIDs = append(payload.OrbitIDs, orbit.OrbitID)
		payload.VariableCount += orbit.DeclaredCount
		payload.MissingCount += orbit.MissingCount

		variables := make([]bindingsMissingVariableJSON, 0, len(orbit.Variables))
		for _, variable := range orbit.Variables {
			variables = append(variables, bindingsMissingVariableJSON{
				Name:                      variable.Name,
				Namespace:                 variable.Namespace,
				Description:               variable.Description,
				Required:                  variable.Required,
				HasValue:                  variable.HasValue,
				ObservedRuntimeUnresolved: variable.ObservedRuntimeUnresolved,
				Missing:                   variable.Missing,
			})
		}

		payload.Orbits = append(payload.Orbits, bindingsMissingOrbitJSON{
			OrbitID:         orbit.OrbitID,
			SnapshotMissing: orbit.SnapshotMissing,
			DeclaredCount:   orbit.DeclaredCount,
			MissingCount:    orbit.MissingCount,
			Variables:       variables,
		})
	}

	return payload
}
