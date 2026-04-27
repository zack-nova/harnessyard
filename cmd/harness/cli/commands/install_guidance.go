package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type runtimeGuidanceComposer func(context.Context, harnesspkg.ComposeRuntimeGuidanceInput) (harnesspkg.ComposeRuntimeGuidanceResult, error)

type installScopedGuidanceOutcome struct {
	WrittenPaths []string
	Warnings     []string
}

// Test-only hook for deterministic scoped guidance failure injection after install commit.
var composeRuntimeGuidanceForInstall runtimeGuidanceComposer = harnesspkg.ComposeRuntimeGuidance

func installPreviewGuidancePaths(repoRoot string, preview orbittemplate.TemplateApplyPreview) []string {
	meta := preview.Source.Spec.Meta
	if meta == nil && !orbittemplate.HasOrbitAgentsBody(preview.Source.Spec) {
		return nil
	}

	paths := make([]string, 0, 3)
	if orbittemplate.HasOrbitAgentsBody(preview.Source.Spec) {
		paths = append(paths, "AGENTS.md")
	}
	if meta != nil && strings.TrimSpace(meta.HumansTemplate) != "" {
		paths = append(paths, "HUMANS.md")
	}
	if meta != nil && strings.TrimSpace(meta.BootstrapTemplate) != "" && installPreviewBootstrapPending(context.Background(), repoRoot, preview.Source.Manifest.Template.OrbitID) {
		paths = append(paths, "BOOTSTRAP.md")
	}

	return dedupeSortedStrings(paths)
}

func installPreviewBootstrapPending(ctx context.Context, repoRoot string, orbitID string) bool {
	repo, err := gitpkg.DiscoverRepo(ctx, repoRoot)
	if err != nil {
		return true
	}
	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return true
	}
	snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
	if err != nil {
		return true
	}
	return snapshot.Bootstrap == nil || !snapshot.Bootstrap.Completed
}

func composeInstallScopedGuidance(
	ctx context.Context,
	repoRoot string,
	orbitIDs []string,
	force bool,
	composer runtimeGuidanceComposer,
) installScopedGuidanceOutcome {
	if len(orbitIDs) == 0 {
		return installScopedGuidanceOutcome{}
	}

	tx, err := harnesspkg.BeginInstallTransaction(ctx, repoRoot, []string{"AGENTS.md", "HUMANS.md", "BOOTSTRAP.md"})
	if err != nil {
		return installScopedGuidanceOutcome{
			Warnings: []string{fmt.Sprintf(
				"install succeeded, but scoped guidance compose could not start: %v. Fix the issue and run `hyard guide sync --target all`.",
				err,
			)},
		}
	}

	result, err := composer(ctx, harnesspkg.ComposeRuntimeGuidanceInput{
		RepoRoot: repoRoot,
		Force:    force,
		Target:   harnesspkg.GuidanceTargetAll,
		OrbitIDs: orbitIDs,
	})
	if err != nil {
		rollbackErr := tx.Rollback()
		return installScopedGuidanceOutcome{
			Warnings: []string{formatInstallScopedGuidanceWarning(err, rollbackErr)},
		}
	}
	tx.Commit()

	writtenPaths := make([]string, 0, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		if artifact.ChangedCount == 0 {
			continue
		}
		writtenPaths = append(writtenPaths, installRepoRelativePath(repoRoot, artifact.Path))
	}

	return installScopedGuidanceOutcome{
		WrittenPaths: dedupeSortedStrings(writtenPaths),
	}
}

func formatInstallScopedGuidanceWarning(composeErr error, rollbackErr error) string {
	if rollbackErr == nil {
		return fmt.Sprintf(
			"install succeeded, but scoped guidance compose was rolled back: %v. Fix the issue and run `hyard guide sync --target all`.",
			composeErr,
		)
	}

	return fmt.Sprintf(
		"install succeeded, but scoped guidance compose failed (%v) and rollback also failed (%v). Guidance artifacts may be in an unknown state; run `hyard check --json` and then `hyard guide sync --target all`.",
		composeErr,
		rollbackErr,
	)
}

func appendInstallGuidancePaths(
	repoRoot string,
	preview orbittemplate.TemplateApplyPreview,
	result *orbittemplate.TemplateApplyResult,
	writtenGuidancePaths []string,
) {
	if result == nil || len(writtenGuidancePaths) == 0 {
		return
	}

	writtenSet := make(map[string]struct{}, len(writtenGuidancePaths))
	for _, path := range writtenGuidancePaths {
		writtenSet[path] = struct{}{}
	}

	for _, path := range installPreviewGuidancePaths(repoRoot, preview) {
		if _, ok := writtenSet[path]; !ok {
			continue
		}
		result.WrittenPaths = append(result.WrittenPaths, path)
	}
	result.WrittenPaths = dedupeSortedStrings(result.WrittenPaths)
}

func installRepoRelativePath(repoRoot string, path string) string {
	relativePath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relativePath)
}

func dedupeSortedStrings(values []string) []string {
	sort.Strings(values)
	if len(values) == 0 {
		return values
	}

	deduped := values[:0]
	for _, value := range values {
		if value == "" {
			continue
		}
		if len(deduped) > 0 && deduped[len(deduped)-1] == value {
			continue
		}
		deduped = append(deduped, value)
	}
	return deduped
}
