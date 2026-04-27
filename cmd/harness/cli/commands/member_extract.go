package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type memberExtractOutput struct {
	HarnessRoot         string   `json:"harness_root"`
	OrbitID             string   `json:"orbit_id"`
	RevisionKind        string   `json:"revision_kind"`
	ExtractMode         string   `json:"extract_mode"`
	ManifestPath        string   `json:"manifest_path"`
	InstallRecordPath   string   `json:"install_record_path,omitempty"`
	MemberCount         int      `json:"member_count"`
	TargetBranch        string   `json:"target_branch,omitempty"`
	TemplateRef         string   `json:"template_ref,omitempty"`
	TemplateCommit      string   `json:"template_commit,omitempty"`
	WrittenPaths        []string `json:"written_paths,omitempty"`
	RemovedPaths        []string `json:"removed_paths,omitempty"`
	RemovedAgentsBlock  bool     `json:"removed_agents_block"`
	DeletedBundleRecord bool     `json:"deleted_bundle_record"`
}

// NewMemberExtractCommand creates the harness member extract command.
func NewMemberExtractCommand() *cobra.Command {
	var orbitID string
	var detached bool
	var targetBranch string
	var reuseOrigin bool

	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Extract one runtime member out of its current harness affiliation",
		Long: "Extract one runtime member from the current runtime.\n" +
			"The current implementation supports the v0.72 detached, --to, and --reuse-origin lanes for bundle-backed members.\n" +
			"Detached extract clears owner_harness_id without guessing new install provenance,\n" +
			"while the install_orbit lanes first save the current runtime orbit to an explicit template branch.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if orbitID == "" {
				return fmt.Errorf("required flag %q not set", "orbit")
			}
			modeCount := 0
			if detached {
				modeCount++
			}
			if targetBranch != "" {
				modeCount++
			}
			if reuseOrigin {
				modeCount++
			}
			if modeCount != 1 {
				return fmt.Errorf("member extract requires exactly one of --detached, --to, or --reuse-origin")
			}

			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveManifestRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}
			if resolved.Manifest.Kind != harnesspkg.ManifestKindRuntime {
				return fmt.Errorf("member extract is only supported for manifest kind %q", harnesspkg.ManifestKindRuntime)
			}

			output := memberExtractOutput{
				HarnessRoot:  resolved.Repo.Root,
				OrbitID:      orbitID,
				RevisionKind: resolved.Manifest.Kind,
			}

			switch {
			case detached:
				result, err := harnesspkg.ExtractRuntimeMemberDetached(cmd.Context(), resolved.Repo.Root, orbitID, time.Now().UTC())
				if err != nil {
					return fmt.Errorf("extract harness member: %w", err)
				}
				output.ExtractMode = "detached"
				output.ManifestPath = result.ManifestPath
				output.MemberCount = len(result.Runtime.Members)
				output.WrittenPaths = result.WrittenPaths
				output.RemovedPaths = result.RemovedPaths
				output.RemovedAgentsBlock = result.RemovedAgentsBlock
				output.DeletedBundleRecord = result.DeletedBundleRecord
			case targetBranch != "":
				result, err := harnesspkg.ExtractRuntimeMemberToInstall(cmd.Context(), resolved.Repo.Root, orbitID, targetBranch, time.Now().UTC())
				if err != nil {
					return fmt.Errorf("extract harness member: %w", err)
				}
				output.ExtractMode = "to"
				output.ManifestPath = result.ManifestPath
				output.InstallRecordPath = result.InstallRecordPath
				output.MemberCount = len(result.Runtime.Members)
				output.TargetBranch = result.TargetBranch
				output.TemplateRef = result.TemplateRef
				output.TemplateCommit = result.TemplateCommit
				output.WrittenPaths = result.WrittenPaths
				output.RemovedPaths = result.RemovedPaths
				output.RemovedAgentsBlock = result.RemovedAgentsBlock
				output.DeletedBundleRecord = result.DeletedBundleRecord
			case reuseOrigin:
				result, err := harnesspkg.ExtractRuntimeMemberReuseOrigin(cmd.Context(), resolved.Repo.Root, orbitID, time.Now().UTC())
				if err != nil {
					return fmt.Errorf("extract harness member: %w", err)
				}
				output.ExtractMode = "reuse_origin"
				output.ManifestPath = result.ManifestPath
				output.InstallRecordPath = result.InstallRecordPath
				output.MemberCount = len(result.Runtime.Members)
				output.TargetBranch = result.TargetBranch
				output.TemplateRef = result.TemplateRef
				output.TemplateCommit = result.TemplateCommit
				output.WrittenPaths = result.WrittenPaths
				output.RemovedPaths = result.RemovedPaths
				output.RemovedAgentsBlock = result.RemovedAgentsBlock
				output.DeletedBundleRecord = result.DeletedBundleRecord
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			message := fmt.Sprintf("extracted orbit %s from harness affiliation in %s\n", orbitID, resolved.Repo.Root)
			if output.TargetBranch != "" {
				message = fmt.Sprintf(
					"extracted orbit %s from harness affiliation in %s\nsaved template branch: %s\n",
					orbitID,
					resolved.Repo.Root,
					output.TargetBranch,
				)
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), message); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&orbitID, "orbit", "", "Runtime orbit id to extract")
	cmd.Flags().BoolVar(&detached, "detached", false, "Extract as a detached standalone member without creating a new install provenance")
	cmd.Flags().StringVar(&targetBranch, "to", "", "Extract by first saving the member into the target orbit template branch")
	cmd.Flags().BoolVar(&reuseOrigin, "reuse-origin", false, "Extract by reusing the member's stored last_standalone_origin target branch")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
