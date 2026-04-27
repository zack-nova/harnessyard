package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

func newAssignCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assign",
		Short: "Assign runtime orbit packages to a harness package",
		Long: "Assign runtime orbit packages to a harness package through the canonical hyard user surface.\n" +
			"These commands update runtime affiliation while keeping the orbit active in the\n" +
			"current runtime.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(newAssignOrbitCommand())

	return cmd
}

func newAssignOrbitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orbit <orbit-package>",
		Short: "Assign a runtime orbit package to a harness package",
		Long: "Assign a runtime orbit package to a harness package through the hyard user surface.\n" +
			"When the runtime has exactly one installed harness package, that package is selected\n" +
			"automatically. If multiple harness packages are installed, pass --harness explicitly.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			harnessID, err := cmd.Flags().GetString("harness")
			if err != nil {
				return fmt.Errorf("read --harness flag: %w", err)
			}
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read --json flag: %w", err)
			}

			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}
			harnessID, err = resolveAssignHarnessPackage(resolved.Repo.Root, resolved.Runtime, harnessID)
			if err != nil {
				return err
			}

			result, err := harnesspkg.AssignRuntimeMember(cmd.Context(), resolved.Repo.Root, args[0], harnessID, time.Now().UTC())
			if err != nil {
				return fmt.Errorf("assign runtime orbit: %w", err)
			}

			output := assignOutput{
				HarnessRoot:            resolved.Repo.Root,
				OrbitPackage:           args[0],
				OrbitID:                args[0],
				HarnessPackage:         result.HarnessID,
				HarnessID:              result.HarnessID,
				Source:                 result.Source,
				PreviousOwnerHarnessID: result.PreviousOwnerHarnessID,
				OwnerHarnessID:         result.HarnessID,
				ManifestPath:           result.ManifestPath,
				MemberCount:            len(result.Runtime.Members),
				Changed:                result.Changed,
			}
			if !result.Changed {
				output.ManifestPath = ""
			}

			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			if result.Changed {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "assigned orbit package %s to harness package %s in %s\n", args[0], result.HarnessID, resolved.Repo.Root)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "orbit package %s is already assigned to harness package %s in %s\n", args[0], result.HarnessID, resolved.Repo.Root)
			}
			if err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().String("harness", "", "Target harness package when the runtime has multiple installed harness packages")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")

	return cmd
}

func resolveAssignHarnessPackage(repoRoot string, runtimeFile harnesspkg.RuntimeFile, explicitHarnessPackage string) (string, error) {
	trimmed := strings.TrimSpace(explicitHarnessPackage)
	if trimmed != "" {
		return trimmed, nil
	}

	harnessPackages, err := harnesspkg.ListBundleRecordIDs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("list installed harness packages: %w", err)
	}
	candidates := make(map[string]struct{}, len(harnessPackages)+len(runtimeFile.Members))
	for _, harnessPackage := range harnessPackages {
		candidates[harnessPackage] = struct{}{}
	}
	for _, member := range runtimeFile.Members {
		owner := strings.TrimSpace(member.OwnerHarnessID)
		if owner == "" {
			continue
		}
		candidates[owner] = struct{}{}
	}

	switch len(candidates) {
	case 0:
		currentComposition := strings.TrimSpace(runtimeFile.Harness.ID)
		if currentComposition != "" {
			return currentComposition, nil
		}
		return "", fmt.Errorf("assign orbit requires --harness <harness-package> because no harness package composition was found")
	case 1:
		for harnessPackage := range candidates {
			return harnessPackage, nil
		}
	default:
		harnessPackages := make([]string, 0, len(candidates))
		for harnessPackage := range candidates {
			harnessPackages = append(harnessPackages, harnessPackage)
		}
		sort.Strings(harnessPackages)
		return "", fmt.Errorf(
			"assign orbit requires --harness <harness-package> because multiple harness packages are installed: %s",
			strings.Join(harnessPackages, ", "),
		)
	}
	return "", fmt.Errorf("assign orbit requires --harness <harness-package> because no harness package composition was found")
}

func newUnassignCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unassign",
		Short: "Remove runtime orbit affiliation from a harness",
		Long: "Remove runtime orbit affiliation from a harness through the canonical hyard user surface.\n" +
			"These commands update runtime affiliation while keeping the orbit active in the\n" +
			"current runtime.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(newUnassignOrbitCommand())

	return cmd
}

func newUnassignOrbitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orbit <orbit-id>",
		Short: "Return a runtime orbit to standalone affiliation",
		Long: "Return a runtime orbit to standalone affiliation through the hyard user surface.\n" +
			"This keeps the orbit active in the current runtime and is distinct from remove.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read --json flag: %w", err)
			}

			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			result, err := harnesspkg.UnassignRuntimeMember(cmd.Context(), resolved.Repo.Root, args[0], time.Now().UTC())
			if err != nil {
				return fmt.Errorf("unassign runtime orbit: %w", err)
			}

			output := unassignOutput{
				HarnessRoot:            resolved.Repo.Root,
				OrbitID:                args[0],
				SourceBefore:           result.SourceBefore,
				SourceAfter:            result.SourceAfter,
				PreviousOwnerHarnessID: result.PreviousOwnerHarnessID,
				OwnerHarnessID:         "",
				ManifestPath:           result.ManifestPath,
				MemberCount:            len(result.Runtime.Members),
				Changed:                result.Changed,
				RemovedPaths:           append([]string(nil), result.RemovedPaths...),
				RemovedAgentsBlock:     result.RemovedAgentsBlock,
				DeletedBundleRecord:    result.DeletedBundleRecord,
			}

			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			message := fmt.Sprintf(
				"returned orbit %s to standalone affiliation in %s",
				args[0],
				resolved.Repo.Root,
			)
			if result.SourceBefore == harnesspkg.MemberSourceInstallBundle {
				message = strings.TrimSuffix(message, ".") + " via detached bundle extract"
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), message); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")

	return cmd
}

type assignOutput struct {
	HarnessRoot            string `json:"harness_root"`
	OrbitPackage           string `json:"orbit_package"`
	OrbitID                string `json:"orbit_id"`
	HarnessPackage         string `json:"harness_package"`
	HarnessID              string `json:"harness_id"`
	Source                 string `json:"source"`
	PreviousOwnerHarnessID string `json:"previous_owner_harness_id"`
	OwnerHarnessID         string `json:"owner_harness_id"`
	ManifestPath           string `json:"manifest_path,omitempty"`
	MemberCount            int    `json:"member_count"`
	Changed                bool   `json:"changed"`
}

type unassignOutput struct {
	HarnessRoot            string   `json:"harness_root"`
	OrbitID                string   `json:"orbit_id"`
	SourceBefore           string   `json:"source_before"`
	SourceAfter            string   `json:"source_after"`
	PreviousOwnerHarnessID string   `json:"previous_owner_harness_id"`
	OwnerHarnessID         string   `json:"owner_harness_id"`
	ManifestPath           string   `json:"manifest_path,omitempty"`
	MemberCount            int      `json:"member_count"`
	Changed                bool     `json:"changed"`
	RemovedPaths           []string `json:"removed_paths,omitempty"`
	RemovedAgentsBlock     bool     `json:"removed_agents_block"`
	DeletedBundleRecord    bool     `json:"deleted_bundle_record"`
}

func mustMarkFlagHidden(cmd *cobra.Command, name string) {
	if err := cmd.Flags().MarkHidden(name); err != nil {
		panic(err)
	}
}
