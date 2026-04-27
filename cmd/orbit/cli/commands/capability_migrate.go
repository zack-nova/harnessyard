package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type capabilityMigrateOutput struct {
	RepoRoot     string                     `json:"repo_root"`
	OrbitID      string                     `json:"orbit"`
	File         string                     `json:"file"`
	Migrated     bool                       `json:"migrated"`
	Capabilities orbitpkg.OrbitCapabilities `json:"capabilities"`
}

func NewCapabilityMigrateCommand() *cobra.Command {
	var orbitID string

	cmd := &cobra.Command{
		Use:   "migrate-v0-66",
		Short: "Migrate legacy capability entries into v0.66 capability truth",
		Long:  "Rewrite one hosted orbit definition from legacy command/skill entry capabilities into the canonical v0.66 path-scope and remote-URI model.",
		Example: "" +
			"  orbit capability migrate-v0-66 --orbit execute\n" +
			"  orbit capability migrate-v0-66 --orbit execute --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			resolvedOrbitID, err := resolveCapabilityMigrationOrbitID(cmd, repo, orbitID)
			if err != nil {
				return err
			}

			result, err := orbitpkg.MigrateHostedCapabilitySpecV066(repo.Root, resolvedOrbitID)
			if err != nil {
				return fmt.Errorf("migrate capability truth: %w", err)
			}

			output := capabilityMigrateOutput{
				RepoRoot:     repo.Root,
				OrbitID:      resolvedOrbitID,
				File:         result.File,
				Migrated:     result.Migrated,
				Capabilities: capabilityListValue(result.Spec.Capabilities),
			}
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			message := "capability truth already uses v0.66 shape"
			if result.Migrated {
				message = "migrated capability truth to v0.66"
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s for orbit %s at %s\n", message, resolvedOrbitID, result.File); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")

	return cmd
}

func resolveCapabilityMigrationOrbitID(cmd *cobra.Command, repo gitpkg.Repo, requestedOrbitID string) (string, error) {
	state, err := orbittemplate.LoadCurrentRepoState(cmd.Context(), repo.Root)
	if err != nil {
		return "", fmt.Errorf("load current repo state: %w", err)
	}

	explicitOrbitID := strings.TrimSpace(requestedOrbitID)
	if explicitOrbitID != "" {
		if err := ids.ValidateOrbitID(explicitOrbitID); err != nil {
			return "", fmt.Errorf("validate orbit id: %w", err)
		}
	}

	if !isAuthoringOrbitBranchKind(state.Kind) {
		return resolveAuthoredTruthOrbitID(cmd, repo, explicitOrbitID)
	}

	branchOrbitID := strings.TrimSpace(state.OrbitID)
	if branchOrbitID == "" {
		return "", fmt.Errorf("current %s branch does not declare orbit identity; pass --orbit after repairing the branch manifest", state.Kind)
	}
	if err := ids.ValidateOrbitID(branchOrbitID); err != nil {
		return "", fmt.Errorf("validate branch manifest orbit id: %w", err)
	}
	if explicitOrbitID != "" && explicitOrbitID != branchOrbitID {
		return "", fmt.Errorf(
			"current %s manifest hosts orbit %q; requested --orbit %q does not match",
			state.Kind,
			branchOrbitID,
			explicitOrbitID,
		)
	}

	if err := verifySingleHostedDefinitionForCapabilityMigration(cmd.Context(), repo.Root, state.Kind, branchOrbitID); err != nil {
		return "", err
	}

	return branchOrbitID, nil
}

func verifySingleHostedDefinitionForCapabilityMigration(ctx context.Context, repoRoot string, branchKind string, orbitID string) error {
	definitionPaths, err := hostedDefinitionCandidatePathsForCapabilityMigration(ctx, repoRoot)
	if err != nil {
		return err
	}
	if len(definitionPaths) != 1 {
		return fmt.Errorf("current %s branch must host exactly one orbit; found %d", branchKind, len(definitionPaths))
	}

	expectedPath, err := orbitpkg.HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return fmt.Errorf("build hosted orbit definition relative path: %w", err)
	}
	if definitionPaths[0] != expectedPath {
		return fmt.Errorf("current %s manifest hosts orbit %q but hosted orbit definition %s was not found", branchKind, orbitID, expectedPath)
	}

	return nil
}

func hostedDefinitionCandidatePathsForCapabilityMigration(ctx context.Context, repoRoot string) ([]string, error) {
	const hostedOrbitsRelativeDir = ".harness/orbits"

	seen := map[string]struct{}{}
	worktreeDir := orbitpkg.HostedOrbitsDir(repoRoot)
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read hosted orbit definitions: %w", err)
		}
	} else {
		for _, entry := range entries {
			if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".yaml" {
				continue
			}
			relativePath, err := ids.NormalizeRepoRelativePath(filepath.ToSlash(filepath.Join(hostedOrbitsRelativeDir, entry.Name())))
			if err != nil {
				return nil, fmt.Errorf("normalize hosted orbit definition path %q: %w", entry.Name(), err)
			}
			seen[relativePath] = struct{}{}
		}
	}

	headPaths, err := gitpkg.ListFilesAtRev(ctx, repoRoot, "HEAD", hostedOrbitsRelativeDir)
	if err != nil {
		return nil, fmt.Errorf("list hosted orbit definitions at HEAD: %w", err)
	}
	for _, relativePath := range headPaths {
		if strings.ToLower(filepath.Ext(relativePath)) != ".yaml" {
			continue
		}
		normalizedPath, err := ids.NormalizeRepoRelativePath(relativePath)
		if err != nil {
			return nil, fmt.Errorf("normalize hosted orbit definition path %q: %w", relativePath, err)
		}
		seen[normalizedPath] = struct{}{}
	}

	paths := make([]string, 0, len(seen))
	for relativePath := range seen {
		paths = append(paths, relativePath)
	}
	sort.Strings(paths)

	return paths, nil
}
