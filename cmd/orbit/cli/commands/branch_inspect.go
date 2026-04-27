package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/branchinfo"
)

type branchInspectOutput struct {
	Revision   string                `json:"revision"`
	Inspection branchinfo.Inspection `json:"inspection"`
}

// NewBranchInspectCommand creates the orbit branch inspect command.
func NewBranchInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <branch-or-revision>",
		Short: "Inspect one branch or revision in more detail",
		Long: "Inspect one branch or revision in more detail, including template manifest metadata for\n" +
			"template branches and runtime control-plane / install-record summaries for runtime branches.\n" +
			"Manifest membership is reported separately from authored OrbitSpec definition members;\n" +
			"the legacy member_count/member_ids fields are manifest-scoped compatibility aliases.",
		Example: "" +
			"  orbit branch inspect orbit-template/docs\n" +
			"  orbit branch inspect main --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			inspection, err := branchinfo.InspectRevision(cmd.Context(), repo.Root, args[0])
			if err != nil {
				return fmt.Errorf("inspect revision %q: %w", args[0], err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), branchInspectOutput{
					Revision:   args[0],
					Inspection: inspection,
				})
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"kind: %s\n",
				inspection.Classification.Kind,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if inspection.Classification.TemplateKind != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "template_kind: %s\n", inspection.Classification.TemplateKind); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "reason: %s\n", inspection.Classification.Reason); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if inspection.Classification.Kind == branchinfo.KindPlain {
				return nil
			}
			if inspection.SourceBranch != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_branch: %s\n", inspection.SourceBranch); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if inspection.PublishOrbitID != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "publish_orbit_id: %s\n", inspection.PublishOrbitID); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if inspection.HarnessID != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", inspection.HarnessID); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if inspection.IncludesRootAgents != nil {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "includes_root_agents: %t\n", *inspection.IncludesRootAgents); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "manifest_member_count: %d\n", inspection.ManifestMemberCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "manifest_member_ids: %s\n", formatStringSlice(inspection.ManifestMemberIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count_scope: %s\n", inspection.MemberCountScope); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count: %d\n", inspection.MemberCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_ids: %s\n", formatStringSlice(inspection.MemberIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "definition_count: %d\n", inspection.DefinitionCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "definition_ids: %s\n", formatStringSlice(inspection.DefinitionIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "definition_member_count: %d\n", inspection.DefinitionMemberCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "definition_members: %s\n", formatDefinitionMembers(inspection.DefinitionMembers)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "install_count: %d\n", inspection.InstallCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "install_ids: %s\n", formatStringSlice(inspection.InstallIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "detached_install_count: %d\n", inspection.DetachedInstallCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "detached_install_ids: %s\n", formatStringSlice(inspection.DetachedInstallIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "invalid_install_count: %d\n", inspection.InvalidInstallCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "invalid_install_ids: %s\n", formatStringSlice(inspection.InvalidInstallIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)

	return cmd
}

func formatStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}

	return "[" + strings.Join(values, " ") + "]"
}

func formatDefinitionMembers(values []branchinfo.DefinitionMemberSummary) string {
	if len(values) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, value.ID+":"+formatStringSlice(value.MemberIDs))
	}

	return "[" + strings.Join(parts, " ") + "]"
}
