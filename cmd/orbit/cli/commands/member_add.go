package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type memberAddOutput struct {
	RepoRoot string               `json:"repo_root"`
	File     string               `json:"file"`
	Member   orbitpkg.OrbitMember `json:"member"`
	Warnings []string             `json:"warnings,omitempty"`
}

// NewMemberAddCommand creates the orbit member add command.
func NewMemberAddCommand() *cobra.Command {
	var orbitID string
	var memberName string
	var legacyMemberKey string
	var memberDescription string
	var rawRole string
	var lane string
	var include []string
	var exclude []string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add one hosted orbit member",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			resolvedOrbitID, err := resolveAuthoredTruthOrbitID(cmd, repo, orbitID)
			if err != nil {
				return err
			}

			config, err := loadValidatedAuthoringRepositoryConfig(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}
			definition, err := definitionByID(config, resolvedOrbitID)
			if err != nil {
				return err
			}

			role, err := orbitpkg.ParseOrbitMemberRole(rawRole)
			if err != nil {
				return fmt.Errorf("parse orbit member role: %w", err)
			}
			memberIdentity, err := resolveMemberIdentityInput(memberName, legacyMemberKey)
			if err != nil {
				return err
			}

			spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repo.Root, definition.ID)
			if err != nil {
				return fmt.Errorf("load orbit spec: %w", err)
			}
			spec, err = orbitpkg.EnsureHostedMemberSchema(spec)
			if err != nil {
				return fmt.Errorf("upgrade hosted orbit spec: %w", err)
			}

			for _, member := range spec.Members {
				if member.Name == memberIdentity || member.Key == memberIdentity {
					return fmt.Errorf("member %q already exists in orbit %q", memberIdentity, definition.ID)
				}
			}

			filteredInclude, skippedIncludes, err := orbitpkg.SplitCapabilityOwnedMemberIncludes(spec, include, exclude)
			if err != nil {
				return fmt.Errorf("check capability-owned member paths: %w", err)
			}
			warnings := memberCapabilityWarnings(definition.ID, skippedIncludes)
			if len(filteredInclude) == 0 {
				return fmt.Errorf(
					"no member paths left after removing capability-owned paths; inspect with hyard orbit capability list --orbit %s --resolve",
					definition.ID,
				)
			}

			member := orbitpkg.OrbitMember{
				Name:        memberIdentity,
				Description: memberDescription,
				Role:        role,
				Lane:        lane,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: filteredInclude,
					Exclude: append([]string(nil), exclude...),
				},
			}
			spec.Members = append(spec.Members, member)

			filename, err := orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
			if err != nil {
				return fmt.Errorf("write orbit definition: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), memberAddOutput{
					RepoRoot: repo.Root,
					File:     filename,
					Member:   member,
					Warnings: warnings,
				})
			}

			if err := printWarnings(cmd, warnings); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "added member %s to orbit %s at %s\n", memberIdentity, definition.ID, filename); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringVar(&memberName, "name", "", "Unique member name")
	cmd.Flags().StringVar(&legacyMemberKey, "key", "", "Deprecated legacy member key")
	cmd.Flags().StringVar(&memberDescription, "description", "", "Optional member description")
	cmd.Flags().StringVar(&rawRole, "role", "", "Member role: subject, rule, or process")
	cmd.Flags().StringVar(&lane, "lane", "", "Optional member lifecycle lane")
	cmd.Flags().StringArrayVar(&include, "include", nil, "Add one include path glob")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "Add one exclude path glob")
	mustMarkFlagRequired(cmd, "role")
	mustMarkFlagRequired(cmd, "include")

	return cmd
}

func memberCapabilityWarnings(orbitID string, skippedIncludes []orbitpkg.CapabilityOwnedMemberInclude) []string {
	warnings := make([]string, 0, len(skippedIncludes))
	for _, skipped := range skippedIncludes {
		warnings = append(warnings, fmt.Sprintf(
			`skipped member include %q: path is managed by %s; inspect with hyard orbit capability list --orbit %s --resolve`,
			skipped.Include,
			skipped.CapabilityField,
			orbitID,
		))
	}

	return warnings
}
