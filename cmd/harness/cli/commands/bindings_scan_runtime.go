package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type bindingsScanRuntimePathJSON struct {
	Path      string   `json:"path"`
	Variables []string `json:"variables"`
}

type bindingsScanRuntimeOrbitJSON struct {
	OrbitID                   string                        `json:"orbit_id"`
	PathCount                 int                           `json:"path_count"`
	PlaceholderCount          int                           `json:"placeholder_count"`
	VariableNamespaces        map[string]string             `json:"variable_namespaces,omitempty"`
	ObservedRuntimeUnresolved []string                      `json:"observed_runtime_unresolved"`
	WroteInstall              bool                          `json:"wrote_install"`
	Paths                     []bindingsScanRuntimePathJSON `json:"paths"`
}

type bindingsScanRuntimeJSON struct {
	HarnessRoot      string                         `json:"harness_root"`
	HarnessID        string                         `json:"harness_id"`
	OrbitCount       int                            `json:"orbit_count"`
	OrbitIDs         []string                       `json:"orbit_ids"`
	PlaceholderCount int                            `json:"placeholder_count"`
	WroteInstall     bool                           `json:"wrote_install"`
	ReadinessReason  string                         `json:"readiness_reason"`
	Orbits           []bindingsScanRuntimeOrbitJSON `json:"orbits"`
}

// NewBindingsScanRuntimeCommand creates the harness bindings scan-runtime command.
func NewBindingsScanRuntimeCommand() *cobra.Command {
	var orbitID string
	var all bool
	var writeInstall bool

	cmd := &cobra.Command{
		Use:   "scan-runtime",
		Short: "Scan current runtime markdown and AGENTS blocks for unresolved placeholders",
		Long: "Scan current runtime markdown and the current orbit's AGENTS.md block for remaining $var_name placeholders.\n" +
			"This command operates on current runtime observations rather than authored variable declarations.",
		Example: "" +
			"  harness bindings scan-runtime --orbit docs\n" +
			"  harness bindings scan-runtime --all --json\n" +
			"  harness bindings scan-runtime --orbit docs --write-install\n",
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

			result, err := harnesspkg.ScanRuntimeBindings(cmd.Context(), harnesspkg.ScanRuntimeBindingsInput{
				RepoRoot:     resolved.Repo.Root,
				OrbitID:      orbitID,
				All:          all,
				WriteInstall: writeInstall,
			})
			if err != nil {
				return fmt.Errorf("scan runtime bindings: %w", err)
			}

			payload := bindingsScanRuntimePayload(resolved.Repo.Root, result)
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
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "placeholder_count: %d\n", payload.PlaceholderCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote_install: %t\n", payload.WroteInstall); err != nil {
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
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit: %s (placeholders: %d)\n", orbit.OrbitID, orbit.PlaceholderCount); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				for _, pathResult := range orbit.Paths {
					if _, err := fmt.Fprintf(
						cmd.OutOrStdout(),
						"  - %s: %s\n",
						pathResult.Path,
						strings.Join(formatScanRuntimeVariables(pathResult.Variables, orbit.VariableNamespaces), ", "),
					); err != nil {
						return fmt.Errorf("write command output: %w", err)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&orbitID, "orbit", "", "Scan one install-backed orbit")
	cmd.Flags().BoolVar(&all, "all", false, "Scan all install-backed orbits in the current runtime")
	cmd.Flags().BoolVar(&writeInstall, "write-install", false, "Persist observed runtime placeholders back into install provenance")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}

func bindingsScanRuntimePayload(repoRoot string, result harnesspkg.ScanRuntimeBindingsResult) bindingsScanRuntimeJSON {
	payload := bindingsScanRuntimeJSON{
		HarnessRoot:     repoRoot,
		HarnessID:       result.HarnessID,
		OrbitCount:      len(result.Orbits),
		OrbitIDs:        make([]string, 0, len(result.Orbits)),
		WroteInstall:    result.WroteInstall,
		ReadinessReason: string(harnesspkg.ReadinessReasonRuntimePlaceholdersObserved),
		Orbits:          make([]bindingsScanRuntimeOrbitJSON, 0, len(result.Orbits)),
	}

	for _, orbit := range result.Orbits {
		payload.OrbitIDs = append(payload.OrbitIDs, orbit.OrbitID)
		payload.PlaceholderCount += orbit.PlaceholderCount

		paths := make([]bindingsScanRuntimePathJSON, 0, len(orbit.Paths))
		for _, pathResult := range orbit.Paths {
			paths = append(paths, bindingsScanRuntimePathJSON{
				Path:      pathResult.Path,
				Variables: append([]string(nil), pathResult.Variables...),
			})
		}

		payload.Orbits = append(payload.Orbits, bindingsScanRuntimeOrbitJSON{
			OrbitID:                   orbit.OrbitID,
			PathCount:                 orbit.PathCount,
			PlaceholderCount:          orbit.PlaceholderCount,
			VariableNamespaces:        cloneBindingsScanRuntimeStringMap(orbit.VariableNamespaces),
			ObservedRuntimeUnresolved: append([]string(nil), orbit.ObservedRuntimeUnresolved...),
			WroteInstall:              orbit.WroteInstall,
			Paths:                     paths,
		})
	}

	return payload
}

func formatScanRuntimeVariables(variables []string, namespaces map[string]string) []string {
	formatted := make([]string, 0, len(variables))
	for _, variable := range variables {
		if namespace := namespaces[variable]; namespace != "" {
			formatted = append(formatted, fmt.Sprintf("%s(namespace=%s)", variable, namespace))
			continue
		}
		formatted = append(formatted, variable)
	}
	return formatted
}

func cloneBindingsScanRuntimeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
