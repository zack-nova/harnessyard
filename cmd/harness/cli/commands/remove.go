package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

const (
	removeModeRuntimeCleanup     = "runtime_cleanup"
	removeModeTemplateFullRemove = "template_full_remove"
)

type removeOutput struct {
	HarnessRoot           string   `json:"harness_root"`
	OrbitID               string   `json:"orbit_id"`
	RevisionKind          string   `json:"revision_kind"`
	RemoveMode            string   `json:"remove_mode,omitempty"`
	ManifestPath          string   `json:"manifest_path"`
	TemplatePath          string   `json:"template_path,omitempty"`
	MemberCount           int      `json:"member_count"`
	RemovedPaths          []string `json:"removed_paths,omitempty"`
	RemovedAgentsBlock    bool     `json:"removed_agents_block"`
	AutoLeftCurrentOrbit  bool     `json:"auto_left_current_orbit"`
	DetachedInstallRecord bool     `json:"detached_install_record"`
	ZeroMemberTemplate    bool     `json:"zero_member_template"`
}

// NewRemoveCommand creates the harness remove command.
func NewRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <orbit-id>",
		Short: "Remove one orbit from the current harness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveManifestRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			output := removeOutput{
				HarnessRoot:  resolved.Repo.Root,
				OrbitID:      args[0],
				RevisionKind: resolved.Manifest.Kind,
			}

			switch resolved.Manifest.Kind {
			case harnesspkg.ManifestKindRuntime:
				result, err := harnesspkg.RemoveRuntimeMember(cmd.Context(), resolved.Repo, args[0], time.Now().UTC())
				if err != nil {
					return fmt.Errorf("remove harness member: %w", err)
				}
				output.RemoveMode = removeModeRuntimeCleanup
				output.ManifestPath = result.ManifestPath
				output.MemberCount = len(result.Runtime.Members)
				output.RemovedPaths = result.RemovedPaths
				output.RemovedAgentsBlock = result.RemovedAgentsBlock
				output.AutoLeftCurrentOrbit = result.AutoLeftCurrentOrbit
				output.DetachedInstallRecord = result.DetachedInstallRecord
			case harnesspkg.ManifestKindHarnessTemplate:
				result, err := harnesspkg.RemoveTemplateMember(cmd.Context(), resolved.Repo.Root, args[0])
				if err != nil {
					return fmt.Errorf("remove harness member: %w", err)
				}
				output.RemoveMode = removeModeTemplateFullRemove
				output.ManifestPath = result.ManifestPath
				output.TemplatePath = result.TemplatePath
				output.MemberCount = len(result.TemplateManifest.Members)
				output.RemovedPaths = result.RemovedPaths
				output.RemovedAgentsBlock = result.RemovedAgentsBlock
				output.ZeroMemberTemplate = result.ZeroMemberTemplate
			default:
				return fmt.Errorf("remove harness member: harness remove is not supported for manifest kind %q", resolved.Manifest.Kind)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "removed orbit %s from harness %s\n", args[0], resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
