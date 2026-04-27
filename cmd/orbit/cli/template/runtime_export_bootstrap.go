package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"gopkg.in/yaml.v3"
)

// RuntimeExportBootstrapFilterInput describes one runtime export filtering pass.
type RuntimeExportBootstrapFilterInput struct {
	RepoRoot                  string
	OrbitID                   string
	Spec                      orbitpkg.OrbitSpec
	ExportPaths               []string
	IncludeCompletedBootstrap bool
}

// RuntimeExportBootstrapFilterResult captures the filtered export scope plus warnings.
type RuntimeExportBootstrapFilterResult struct {
	ExportPaths           []string
	SkippedBootstrapPaths []string
	Warnings              []string
}

// FilterCompletedBootstrapExportPaths removes bootstrap member paths from runtime export once
// bootstrap completion has closed that runtime surface.
func FilterCompletedBootstrapExportPaths(
	ctx context.Context,
	input RuntimeExportBootstrapFilterInput,
) (RuntimeExportBootstrapFilterResult, error) {
	result := RuntimeExportBootstrapFilterResult{
		ExportPaths: append([]string(nil), input.ExportPaths...),
	}
	if input.RepoRoot == "" {
		return RuntimeExportBootstrapFilterResult{}, fmt.Errorf("repo root must not be empty")
	}

	bootstrapMembers := bootstrapExportMembers(input.Spec)
	if len(bootstrapMembers) == 0 {
		return result, nil
	}

	revisionKind, err := currentRuntimeExportRevisionKind(ctx, input.RepoRoot)
	if err != nil {
		return RuntimeExportBootstrapFilterResult{}, fmt.Errorf("classify current revision: %w", err)
	}
	repo, err := gitpkg.DiscoverRepo(ctx, input.RepoRoot)
	if err != nil {
		return RuntimeExportBootstrapFilterResult{}, fmt.Errorf("discover repository git dir: %w", err)
	}

	status, err := InspectBootstrapOrbitForRevision(ctx, input.RepoRoot, repo.GitDir, input.OrbitID, revisionKind)
	if err != nil {
		return RuntimeExportBootstrapFilterResult{}, fmt.Errorf("inspect bootstrap state for orbit %q: %w", input.OrbitID, err)
	}
	plan := PlanBootstrapRuntimeExport(status, input.IncludeCompletedBootstrap)
	if plan.Action != BootstrapActionSkip && (status.CompletionState != BootstrapCompletionStateCompleted || !input.IncludeCompletedBootstrap) {
		return result, nil
	}

	filtered := make([]string, 0, len(input.ExportPaths))
	skipped := make([]string, 0)
	for _, exportPath := range input.ExportPaths {
		matched := false
		for _, member := range bootstrapMembers {
			memberMatch, err := orbitpkg.MemberMatchesPath(member, exportPath)
			if err != nil {
				return RuntimeExportBootstrapFilterResult{}, fmt.Errorf(
					"match bootstrap member path %q for orbit %q: %w",
					exportPath,
					input.OrbitID,
					err,
				)
			}
			if memberMatch {
				matched = true
				break
			}
		}
		if matched {
			if status.CompletionState == BootstrapCompletionStateCompleted && input.IncludeCompletedBootstrap {
				if err := ensureRuntimeExportPathReadable(ctx, input.RepoRoot, exportPath); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						skipped = append(skipped, exportPath)
						continue
					}
					return RuntimeExportBootstrapFilterResult{}, fmt.Errorf(
						"validate completed bootstrap export path %q for orbit %q: %w",
						exportPath,
						input.OrbitID,
						err,
					)
				}
			} else {
				skipped = append(skipped, exportPath)
				continue
			}
		}
		filtered = append(filtered, exportPath)
	}
	if len(skipped) == 0 {
		return result, nil
	}

	sort.Strings(skipped)
	result.ExportPaths = filtered
	result.SkippedBootstrapPaths = skipped
	if status.CompletionState == BootstrapCompletionStateCompleted && input.IncludeCompletedBootstrap {
		result.Warnings = []string{
			fmt.Sprintf(
				`skip missing completed-bootstrap export paths for orbit %q: %s`,
				input.OrbitID,
				strings.Join(skipped, ", "),
			),
		}
	} else {
		result.Warnings = []string{
			fmt.Sprintf(
				`skip bootstrap export paths for orbit %q because bootstrap is already completed in this runtime: %s`,
				input.OrbitID,
				strings.Join(skipped, ", "),
			),
		}
	}

	return result, nil
}

func ensureRuntimeExportPathReadable(ctx context.Context, repoRoot string, exportPath string) error {
	if _, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(ctx, repoRoot, exportPath); err != nil {
		return fmt.Errorf("read tracked file %q: %w", exportPath, err)
	}
	if _, err := gitpkg.TrackedFileModeWorktreeOrHEAD(ctx, repoRoot, exportPath); err != nil {
		return fmt.Errorf("read tracked file mode %q: %w", exportPath, err)
	}

	return nil
}

func bootstrapExportMembers(spec orbitpkg.OrbitSpec) []orbitpkg.OrbitMember {
	members := make([]orbitpkg.OrbitMember, 0)
	for _, member := range spec.Members {
		if member.Lane == orbitpkg.OrbitMemberLaneBootstrap {
			members = append(members, member)
		}
	}

	return members
}

func currentRuntimeExportRevisionKind(ctx context.Context, repoRoot string) (string, error) {
	data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, ".harness/manifest.yaml")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read .harness/manifest.yaml: %w", err)
	}

	var manifest struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("parse .harness/manifest.yaml: %w", err)
	}

	return strings.TrimSpace(manifest.Kind), nil
}
