package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// BundleMemberShrinkPlan captures one validated bundle shrink/recomposition mutation.
type BundleMemberShrinkPlan struct {
	ExistingRecord        BundleRecord
	NextRecord            *BundleRecord
	RemovedMemberIDs      []string
	DeletePaths           []string
	RemoveRootAgentsBlock bool
	DeleteBundleRecord    bool
}

// BuildBundleMemberShrinkPlan validates and prepares one bundle shrink for the removed members.
func BuildBundleMemberShrinkPlan(
	ctx context.Context,
	repoRoot string,
	record BundleRecord,
	removedMemberIDs []string,
) (BundleMemberShrinkPlan, error) {
	if err := ValidateBundleRecord(record); err != nil {
		return BundleMemberShrinkPlan{}, fmt.Errorf("validate bundle record: %w", err)
	}
	if len(removedMemberIDs) == 0 {
		return BundleMemberShrinkPlan{}, fmt.Errorf("removed member set must not be empty")
	}

	source, err := resolveInstalledBundleSource(ctx, repoRoot, record)
	if err != nil {
		return BundleMemberShrinkPlan{}, fmt.Errorf("resolve installed bundle source: %w", err)
	}
	validation, err := validateTemplateMemberSnapshots(source)
	if err != nil {
		return BundleMemberShrinkPlan{}, err
	}

	recordMembers := make(map[string]struct{}, len(record.MemberIDs))
	for _, memberID := range record.MemberIDs {
		recordMembers[memberID] = struct{}{}
	}

	removedSet := make(map[string]struct{}, len(removedMemberIDs))
	for _, memberID := range removedMemberIDs {
		if _, ok := recordMembers[memberID]; !ok {
			return BundleMemberShrinkPlan{}, fmt.Errorf("bundle %q does not own member %q", record.HarnessID, memberID)
		}
		if _, seen := removedSet[memberID]; seen {
			continue
		}
		removedSet[memberID] = struct{}{}
	}

	deletePaths := make(map[string]struct{})
	for _, memberID := range sortedStringSetKeys(removedSet) {
		snapshot, ok := source.MemberSnapshots[memberID]
		if !ok {
			return BundleMemberShrinkPlan{}, fmt.Errorf("template member snapshot for %q is required", memberID)
		}

		definitionPath, err := orbitpkg.HostedDefinitionRelativePath(memberID)
		if err != nil {
			return BundleMemberShrinkPlan{}, fmt.Errorf("build hosted definition path for %q: %w", memberID, err)
		}
		deletePaths[definitionPath] = struct{}{}
		for _, path := range snapshot.Snapshot.ExportedPaths {
			if path == rootAgentsPath {
				continue
			}
			if !allBundlePathContributorsRemoved(validation.pathContributors[path], removedSet) {
				return BundleMemberShrinkPlan{}, fmt.Errorf(
					"bundle-backed member %q cannot be removed because shared payload path %q would become ambiguous",
					memberID,
					path,
				)
			}
			deletePaths[path] = struct{}{}
		}
	}

	deleteList := make([]string, 0, len(deletePaths))
	for path := range deletePaths {
		if err := verifyBundleOwnedPathOwnership(repoRoot, record, path); err != nil {
			return BundleMemberShrinkPlan{}, err
		}
		deleteList = append(deleteList, path)
	}
	sort.Strings(deleteList)

	surviving := make([]string, 0, len(record.MemberIDs)-len(removedSet))
	for _, memberID := range record.MemberIDs {
		if _, removed := removedSet[memberID]; removed {
			continue
		}
		surviving = append(surviving, memberID)
	}

	plan := BundleMemberShrinkPlan{
		ExistingRecord:   record,
		RemovedMemberIDs: sortedStringSetKeys(removedSet),
		DeletePaths:      deleteList,
	}
	if len(surviving) == 0 {
		if record.IncludesRootAgents {
			if err := verifyBundleAgentsBlockOwnership(repoRoot, record); err != nil {
				return BundleMemberShrinkPlan{}, err
			}
			plan.RemoveRootAgentsBlock = true
		}
		plan.DeleteBundleRecord = true
		return plan, nil
	}

	nextRecord := shrinkBundleRecord(record, source, surviving, deletePaths)
	plan.NextRecord = &nextRecord
	return plan, nil
}

func allBundlePathContributorsRemoved(contributors []string, removedSet map[string]struct{}) bool {
	if len(contributors) == 0 {
		return true
	}
	for _, contributor := range contributors {
		if _, removed := removedSet[contributor]; !removed {
			return false
		}
	}

	return true
}

// FilterBundleMemberShrinkPlanDeletePaths removes any delete paths that will be re-owned by another install unit.
func FilterBundleMemberShrinkPlanDeletePaths(
	plan BundleMemberShrinkPlan,
	preservedPaths map[string]struct{},
) BundleMemberShrinkPlan {
	if len(plan.DeletePaths) == 0 || len(preservedPaths) == 0 {
		return plan
	}

	filtered := make([]string, 0, len(plan.DeletePaths))
	for _, path := range plan.DeletePaths {
		if _, keep := preservedPaths[path]; keep {
			continue
		}
		filtered = append(filtered, path)
	}
	plan.DeletePaths = filtered
	return plan
}

// ApplyBundleMemberShrinkPlan applies one previously validated bundle shrink plan.
func ApplyBundleMemberShrinkPlan(repoRoot string, plan BundleMemberShrinkPlan) ([]string, error) {
	removedPaths, err := applyBundleOwnedCleanup(repoRoot, plan.ExistingRecord, bundleOwnedCleanupPlan{
		DeletePaths:           append([]string(nil), plan.DeletePaths...),
		RemoveRootAgentsBlock: plan.RemoveRootAgentsBlock,
	})
	if err != nil {
		return nil, err
	}

	if plan.DeleteBundleRecord {
		bundlePath, err := BundleRecordPath(repoRoot, plan.ExistingRecord.HarnessID)
		if err != nil {
			return nil, fmt.Errorf("build bundle record path: %w", err)
		}
		if err := os.Remove(bundlePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("delete bundle record %s: %w", plan.ExistingRecord.HarnessID, err)
		}
		removedPaths = append(removedPaths, mustRepoRelativePath(repoRoot, bundlePath))
		sort.Strings(removedPaths)
		return removedPaths, nil
	}

	if plan.NextRecord == nil {
		return nil, fmt.Errorf("shrink plan is missing next bundle record")
	}
	bundlePath, err := WriteBundleRecord(repoRoot, *plan.NextRecord)
	if err != nil {
		return nil, fmt.Errorf("write shrunken bundle record: %w", err)
	}
	removedPaths = append(removedPaths, mustRepoRelativePath(repoRoot, bundlePath))

	sort.Strings(removedPaths)
	return removedPaths, nil
}

func buildBundleMemberShrinkPlan(
	ctx context.Context,
	repoRoot string,
	record BundleRecord,
	removedMemberIDs []string,
) (BundleMemberShrinkPlan, error) {
	return BuildBundleMemberShrinkPlan(ctx, repoRoot, record, removedMemberIDs)
}

func applyBundleMemberShrinkPlan(repoRoot string, plan BundleMemberShrinkPlan) ([]string, error) {
	return ApplyBundleMemberShrinkPlan(repoRoot, plan)
}

func sortedStringSetKeys(items map[string]struct{}) []string {
	keys := make([]string, 0, len(items))
	for item := range items {
		keys = append(keys, item)
	}
	sort.Strings(keys)
	return keys
}

func resolveInstalledBundleSource(ctx context.Context, repoRoot string, record BundleRecord) (LocalTemplateInstallSource, error) {
	switch strings.TrimSpace(record.Template.SourceKind) {
	case orbittemplate.InstallSourceKindLocalBranch:
		if strings.TrimSpace(record.Template.TemplateCommit) == "" {
			return LocalTemplateInstallSource{}, fmt.Errorf("template.template_commit must not be empty")
		}
		exists, err := gitpkg.RevisionExists(ctx, repoRoot, record.Template.TemplateCommit)
		if err != nil {
			return LocalTemplateInstallSource{}, fmt.Errorf("check recorded bundle template commit %q: %w", record.Template.TemplateCommit, err)
		}
		if !exists {
			return LocalTemplateInstallSource{}, fmt.Errorf("recorded bundle template commit %q is not available locally", record.Template.TemplateCommit)
		}
		return loadTemplateInstallSourceAtRevision(
			ctx,
			repoRoot,
			record.Template.TemplateCommit,
			record.Template.SourceRef,
			record.Template.TemplateCommit,
		)
	case orbittemplate.InstallSourceKindExternalGit:
		var source LocalTemplateInstallSource
		if err := gitpkg.WithFetchedRemoteRef(ctx, repoRoot, record.Template.SourceRepo, record.Template.SourceRef, func(_ string) error {
			exists, err := gitpkg.RevisionExists(ctx, repoRoot, record.Template.TemplateCommit)
			if err != nil {
				return fmt.Errorf("check recorded bundle template commit %q: %w", record.Template.TemplateCommit, err)
			}
			if !exists {
				return fmt.Errorf("recorded bundle template commit %q is not available locally", record.Template.TemplateCommit)
			}
			resolved, err := loadTemplateInstallSourceAtRevision(
				ctx,
				repoRoot,
				record.Template.TemplateCommit,
				record.Template.SourceRef,
				record.Template.TemplateCommit,
			)
			if err != nil {
				return err
			}
			source = resolved
			return nil
		}); err != nil {
			return LocalTemplateInstallSource{}, fmt.Errorf(
				"resolve external harness template source %q from %q: %w",
				record.Template.SourceRef,
				record.Template.SourceRepo,
				err,
			)
		}
		return source, nil
	default:
		return LocalTemplateInstallSource{}, fmt.Errorf("unsupported bundle source kind %q", record.Template.SourceKind)
	}
}

func shrinkBundleRecord(
	record BundleRecord,
	source LocalTemplateInstallSource,
	survivingMemberIDs []string,
	deletePaths map[string]struct{},
) BundleRecord {
	keptPaths := make([]string, 0, len(record.OwnedPaths))
	keptDigests := make(map[string]string, len(record.OwnedPathDigests))
	for _, path := range record.OwnedPaths {
		if _, removed := deletePaths[path]; removed {
			continue
		}
		keptPaths = append(keptPaths, path)
	}
	sort.Strings(keptPaths)

	for path, digest := range record.OwnedPathDigests {
		if _, removed := deletePaths[path]; removed {
			continue
		}
		keptDigests[path] = digest
	}

	next := BundleRecord{
		SchemaVersion:        record.SchemaVersion,
		HarnessID:            record.HarnessID,
		Template:             record.Template,
		RecommendedFramework: record.RecommendedFramework,
		MemberIDs:            append([]string(nil), survivingMemberIDs...),
		AppliedAt:            record.AppliedAt,
		IncludesRootAgents:   record.IncludesRootAgents,
		OwnedPaths:           keptPaths,
		OwnedPathDigests:     keptDigests,
		RootAgentsDigest:     record.RootAgentsDigest,
		Variables:            filterBundleVariablesSnapshot(record.Variables, source, survivingMemberIDs),
	}

	return next
}

func filterBundleVariablesSnapshot(
	snapshot *orbittemplate.InstallVariablesSnapshot,
	source LocalTemplateInstallSource,
	survivingMemberIDs []string,
) *orbittemplate.InstallVariablesSnapshot {
	if snapshot == nil {
		return nil
	}

	keepNames := make(map[string]struct{})
	for _, memberID := range survivingMemberIDs {
		memberSnapshot, ok := source.MemberSnapshots[memberID]
		if !ok {
			continue
		}
		for name := range memberSnapshot.Snapshot.Variables {
			keepNames[name] = struct{}{}
		}
	}
	if len(keepNames) == 0 {
		return nil
	}

	filtered := orbittemplate.InstallVariablesSnapshot{
		Declarations:              map[string]bindings.VariableDeclaration{},
		Namespaces:                map[string]string{},
		ResolvedAtApply:           map[string]bindings.VariableBinding{},
		UnresolvedAtApply:         []string{},
		ObservedRuntimeUnresolved: []string{},
	}
	for name, declaration := range snapshot.Declarations {
		if _, ok := keepNames[name]; ok {
			filtered.Declarations[name] = declaration
		}
	}
	for name, namespace := range snapshot.Namespaces {
		if _, ok := keepNames[name]; ok {
			filtered.Namespaces[name] = namespace
		}
	}
	for name, binding := range snapshot.ResolvedAtApply {
		if _, ok := keepNames[name]; ok {
			filtered.ResolvedAtApply[name] = binding
		}
	}
	for _, name := range snapshot.UnresolvedAtApply {
		if _, ok := keepNames[name]; ok {
			filtered.UnresolvedAtApply = append(filtered.UnresolvedAtApply, name)
		}
	}
	for _, name := range snapshot.ObservedRuntimeUnresolved {
		if _, ok := keepNames[name]; ok {
			filtered.ObservedRuntimeUnresolved = append(filtered.ObservedRuntimeUnresolved, name)
		}
	}
	if len(filtered.Declarations) == 0 &&
		len(filtered.Namespaces) == 0 &&
		len(filtered.ResolvedAtApply) == 0 &&
		len(filtered.UnresolvedAtApply) == 0 &&
		len(filtered.ObservedRuntimeUnresolved) == 0 {
		return nil
	}

	sort.Strings(filtered.UnresolvedAtApply)
	sort.Strings(filtered.ObservedRuntimeUnresolved)

	return &filtered
}
