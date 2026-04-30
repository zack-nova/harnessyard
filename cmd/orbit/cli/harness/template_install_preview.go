package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// TemplateInstallPreviewInput describes one harness template install preview or apply analysis.
type TemplateInstallPreviewInput struct {
	RepoRoot                string
	Source                  LocalTemplateInstallSource
	InstallSource           orbittemplate.Source
	BindingsFilePath        string
	OverrideOrbitIDs        map[string]struct{}
	OverwriteExisting       bool
	Interactive             bool
	Prompter                orbittemplate.BindingPrompter
	EditorMode              bool
	Editor                  orbittemplate.Editor
	RequireResolvedBindings bool
	Now                     time.Time
}

// TemplateInstallPreview captures one harness template install preview plus mixed-install diagnostics.
type TemplateInstallPreview struct {
	Source                    LocalTemplateInstallSource
	InstallSource             orbittemplate.Source
	ResolvedBindings          map[string]bindings.ResolvedBinding
	RenderedDefinitionFiles   []orbittemplate.CandidateFile
	RenderedFiles             []orbittemplate.CandidateFile
	RenderedRootAgentsFile    *orbittemplate.CandidateFile
	VarsFile                  *bindings.VarsFile
	BundleRecord              BundleRecord
	Conflicts                 []orbittemplate.ApplyConflict
	OverriddenInstallOrbitIDs []string
	OverriddenBundleMembers   []templateInstallBundleOverrideTarget
	Warnings                  []string
}

type templateInstallBundleOverrideTarget struct {
	OrbitID        string
	OwnerHarnessID string
}

type templateInstallMaterialization struct {
	ResolvedBindings        map[string]bindings.ResolvedBinding
	RenderedDefinitionFiles []orbittemplate.CandidateFile
	RenderedFiles           []orbittemplate.CandidateFile
	RenderedRootAgentsFile  *orbittemplate.CandidateFile
	VarsFile                *bindings.VarsFile
	BundleRecord            BundleRecord
	Warnings                []string
}

type templateInstallLocalInputs struct {
	bindingsFile    map[string]bindings.VariableBinding
	repoVarsFile    bindings.VarsFile
	hasRepoVarsFile bool
}

// BuildTemplateInstallPreview analyzes one harness template install against the current runtime.
func BuildTemplateInstallPreview(
	ctx context.Context,
	input TemplateInstallPreviewInput,
) (TemplateInstallPreview, error) {
	runtimeFile, err := LoadRuntimeFile(input.RepoRoot)
	if err != nil {
		return TemplateInstallPreview{}, fmt.Errorf("load harness runtime: %w", err)
	}

	statusEntries, err := git.WorktreeStatus(ctx, input.RepoRoot)
	if err != nil {
		return TemplateInstallPreview{}, fmt.Errorf("load runtime worktree status: %w", err)
	}
	statusByPath := make(map[string]git.StatusEntry, len(statusEntries))
	for _, entry := range statusEntries {
		statusByPath[entry.Path] = entry
	}

	localInputs, err := loadTemplateInstallLocalInputs(ctx, input.RepoRoot, input.BindingsFilePath)
	if err != nil {
		return TemplateInstallPreview{}, err
	}

	materialization, err := buildTemplateInstallMaterialization(ctx, input, localInputs)
	if err != nil {
		return TemplateInstallPreview{}, err
	}

	conflicts := make([]orbittemplate.ApplyConflict, 0)
	var existingBundleRecord BundleRecord
	hasExistingBundleRecord := false
	if record, err := LoadBundleRecord(input.RepoRoot, input.Source.Manifest.Template.HarnessID); err == nil {
		existingBundleRecord = record
		hasExistingBundleRecord = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return TemplateInstallPreview{}, fmt.Errorf("load existing bundle record: %w", err)
	}
	sameUnitReplace := input.OverwriteExisting && hasExistingBundleRecord
	allowedOwnedPaths := make(map[string]struct{})
	allowedBundleMembers := make(map[string]struct{})
	overriddenInstallOrbitIDs := make([]string, 0)
	overriddenBundleMembers := make([]templateInstallBundleOverrideTarget, 0)
	if sameUnitReplace {
		for _, path := range existingBundleRecord.OwnedPaths {
			allowedOwnedPaths[path] = struct{}{}
		}
		for _, memberID := range existingBundleRecord.MemberIDs {
			allowedBundleMembers[memberID] = struct{}{}
		}
	}

	for _, member := range input.Source.Manifest.Members {
		for _, existing := range runtimeFile.Members {
			if existing.OrbitID != member.OrbitID {
				continue
			}
			if sameUnitReplace && existing.Source == MemberSourceInstallBundle {
				if _, ok := allowedBundleMembers[member.OrbitID]; ok {
					continue
				}
			}
			if templateInstallOverrideRequested(input.OverrideOrbitIDs, member.OrbitID) && existing.Source == MemberSourceInstallOrbit {
				record, err := LoadInstallRecord(input.RepoRoot, member.OrbitID)
				if err != nil {
					return TemplateInstallPreview{}, fmt.Errorf("load install record for override target %q: %w", member.OrbitID, err)
				}
				cleanupPlan, err := orbittemplate.BuildInstallOwnedCleanupPlan(ctx, input.RepoRoot, record, orbittemplate.TemplateApplyPreview{})
				if err != nil {
					return TemplateInstallPreview{}, fmt.Errorf("reconstruct existing install ownership for override target %q: %w", member.OrbitID, err)
				}
				for _, path := range cleanupPlan.DeletePaths {
					allowedOwnedPaths[path] = struct{}{}
				}
				definitionPath, err := orbit.HostedDefinitionRelativePath(member.OrbitID)
				if err != nil {
					return TemplateInstallPreview{}, fmt.Errorf("build hosted definition path for override target %q: %w", member.OrbitID, err)
				}
				allowedOwnedPaths[definitionPath] = struct{}{}
				if cleanupPlan.RemoveSharedAgentsFile {
					allowedOwnedPaths[rootAgentsPath] = struct{}{}
				}
				overriddenInstallOrbitIDs = append(overriddenInstallOrbitIDs, member.OrbitID)
				continue
			}
			if templateInstallOverrideRequested(input.OverrideOrbitIDs, member.OrbitID) && existing.Source == MemberSourceInstallBundle {
				if strings.TrimSpace(existing.OwnerHarnessID) == "" {
					return TemplateInstallPreview{}, fmt.Errorf("bundle-backed member %q is missing owner_harness_id", member.OrbitID)
				}
				record, err := LoadBundleRecord(input.RepoRoot, existing.OwnerHarnessID)
				if err != nil {
					return TemplateInstallPreview{}, fmt.Errorf("load bundle record for override target %q: %w", member.OrbitID, err)
				}
				shrinkPlan, err := BuildBundleMemberShrinkPlan(ctx, input.RepoRoot, record, []string{member.OrbitID})
				if err != nil {
					return TemplateInstallPreview{}, fmt.Errorf("build bundle shrink plan for override target %q: %w", member.OrbitID, err)
				}
				for _, path := range shrinkPlan.DeletePaths {
					allowedOwnedPaths[path] = struct{}{}
				}
				if shrinkPlan.RemoveRootAgentsBlock {
					allowedOwnedPaths[rootAgentsPath] = struct{}{}
				}
				overriddenBundleMembers = append(overriddenBundleMembers, templateInstallBundleOverrideTarget{
					OrbitID:        member.OrbitID,
					OwnerHarnessID: existing.OwnerHarnessID,
				})
				continue
			}
			conflicts = append(conflicts, orbittemplate.ApplyConflict{
				Path:    ManifestRepoPath(),
				Message: fmt.Sprintf("member %q already exists in harness runtime", member.OrbitID),
			})
		}
	}

	if hasExistingBundleRecord && !sameUnitReplace {
		conflicts = append(conflicts, orbittemplate.ApplyConflict{
			Path:    BundleRecordsDirRepoPath(),
			Message: fmt.Sprintf("bundle %q already exists in harness runtime", input.Source.Manifest.Template.HarnessID),
		})
	}

	for _, file := range materialization.RenderedDefinitionFiles {
		conflictsForFile, err := analyzeTemplateInstallPathConflict(input.RepoRoot, file, statusByPath, allowedOwnedPaths)
		if err != nil {
			return TemplateInstallPreview{}, err
		}
		conflicts = append(conflicts, conflictsForFile...)
	}
	for _, file := range materialization.RenderedFiles {
		conflictsForFile, err := analyzeTemplateInstallPathConflict(input.RepoRoot, file, statusByPath, allowedOwnedPaths)
		if err != nil {
			return TemplateInstallPreview{}, err
		}
		conflicts = append(conflicts, conflictsForFile...)
	}
	if materialization.RenderedRootAgentsFile != nil {
		_, allowAgentsStatusConflict := allowedOwnedPaths[rootAgentsPath]
		agentsConflicts, err := analyzeBundleAgentsInstallPreview(
			input.RepoRoot,
			input.Source.Manifest.Template.HarnessID,
			statusByPath,
			allowAgentsStatusConflict,
		)
		if err != nil {
			return TemplateInstallPreview{}, err
		}
		conflicts = append(conflicts, agentsConflicts...)
	}

	excludedBundleHarnessID := ""
	if sameUnitReplace {
		excludedBundleHarnessID = existingBundleRecord.HarnessID
	}
	variableConflicts, err := analyzeTemplateInstallVariableConflicts(
		input.RepoRoot,
		runtimeFile,
		input.Source.Manifest.Variables,
		excludedBundleHarnessID,
	)
	if err != nil {
		return TemplateInstallPreview{}, fmt.Errorf("load install-unit variable declarations: %w", err)
	}
	conflicts = append(conflicts, variableConflicts...)

	if sameUnitReplace {
		_, err := buildBundleOwnedCleanupPlan(input.RepoRoot, existingBundleRecord, materialization.BundleRecord)
		if err != nil {
			var proofErr *bundleOwnershipProofError
			if errors.As(err, &proofErr) {
				conflicts = append(conflicts, orbittemplate.ApplyConflict{
					Path:    proofErr.Path,
					Message: proofErr.Message,
				})
			} else {
				return TemplateInstallPreview{}, fmt.Errorf("build stale bundle cleanup plan: %w", err)
			}
		}
	}

	sort.Slice(conflicts, func(left, right int) bool {
		if conflicts[left].Path == conflicts[right].Path {
			return conflicts[left].Message < conflicts[right].Message
		}
		return conflicts[left].Path < conflicts[right].Path
	})
	sort.Strings(materialization.Warnings)

	return TemplateInstallPreview{
		Source:                    input.Source,
		InstallSource:             input.InstallSource,
		ResolvedBindings:          materialization.ResolvedBindings,
		RenderedDefinitionFiles:   materialization.RenderedDefinitionFiles,
		RenderedFiles:             materialization.RenderedFiles,
		RenderedRootAgentsFile:    materialization.RenderedRootAgentsFile,
		VarsFile:                  materialization.VarsFile,
		BundleRecord:              materialization.BundleRecord,
		Conflicts:                 conflicts,
		OverriddenInstallOrbitIDs: slicesCompactStrings(overriddenInstallOrbitIDs),
		OverriddenBundleMembers:   compactTemplateInstallBundleOverrideTargets(overriddenBundleMembers),
		Warnings:                  materialization.Warnings,
	}, nil
}

// ApplyTemplateInstallPreview writes one harness template install into the runtime repository.
func ApplyTemplateInstallPreview(
	ctx context.Context,
	repoRoot string,
	preview TemplateInstallPreview,
	overwriteExisting bool,
) (TemplateInstallResult, error) {
	if len(preview.Conflicts) > 0 {
		return TemplateInstallResult{}, fmt.Errorf(
			"conflicts detected; mixed harness template install requires disjoint targets: %s",
			preview.Conflicts[0].Message,
		)
	}

	var cleanupPlan bundleOwnedCleanupPlan
	var hasCleanupPlan bool
	if overwriteExisting {
		existingRecord, err := LoadBundleRecord(repoRoot, preview.Source.Manifest.Template.HarnessID)
		if err == nil {
			cleanupPlan, err = buildBundleOwnedCleanupPlan(repoRoot, existingRecord, preview.BundleRecord)
			if err != nil {
				return TemplateInstallResult{}, fmt.Errorf("verify stale bundle-owned cleanup: %w", err)
			}
			hasCleanupPlan = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return TemplateInstallResult{}, fmt.Errorf("load existing bundle record: %w", err)
		}
	}

	installOverrideCleanupPlans, err := buildTemplateInstallOverrideCleanupPlans(ctx, repoRoot, preview)
	if err != nil {
		return TemplateInstallResult{}, err
	}
	bundleOverrideShrinkPlans, err := buildTemplateInstallBundleOverrideShrinkPlans(ctx, repoRoot, preview)
	if err != nil {
		return TemplateInstallResult{}, err
	}

	transactionPaths, err := buildTemplateInstallTransactionPaths(
		preview,
		hasCleanupPlan,
		cleanupPlan,
		installOverrideCleanupPlans,
		bundleOverrideShrinkPlans,
	)
	if err != nil {
		return TemplateInstallResult{}, err
	}

	tx, err := BeginInstallTransaction(ctx, repoRoot, transactionPaths)
	if err != nil {
		return TemplateInstallResult{}, fmt.Errorf("begin install transaction: %w", err)
	}
	rollbackOnError := func(cause error) (TemplateInstallResult, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return TemplateInstallResult{}, errors.Join(cause, fmt.Errorf("rollback install transaction: %w", rollbackErr))
		}
		return TemplateInstallResult{}, cause
	}

	writtenPaths := make([]string, 0, len(preview.RenderedDefinitionFiles)+len(preview.RenderedFiles)+4)
	for _, file := range preview.RenderedDefinitionFiles {
		if err := writeTemplateCandidateFile(repoRoot, file); err != nil {
			return rollbackOnError(err)
		}
		writtenPaths = append(writtenPaths, file.Path)
	}
	for _, file := range preview.RenderedFiles {
		if err := writeTemplateCandidateFile(repoRoot, file); err != nil {
			return rollbackOnError(err)
		}
		writtenPaths = append(writtenPaths, file.Path)
	}
	if preview.RenderedRootAgentsFile != nil {
		if err := ApplyBundleAgentsPayload(repoRoot, preview.Source.Manifest.Template.HarnessID, preview.RenderedRootAgentsFile.Content); err != nil {
			return rollbackOnError(fmt.Errorf("write runtime AGENTS.md: %w", err))
		}
		writtenPaths = append(writtenPaths, rootAgentsPath)
	}
	if preview.VarsFile != nil {
		varsPath, err := WriteVarsFile(repoRoot, *preview.VarsFile)
		if err != nil {
			return rollbackOnError(fmt.Errorf("write harness vars file: %w", err))
		}
		writtenPaths = append(writtenPaths, mustRepoRelativePath(repoRoot, varsPath))
	}

	bundlePath, err := WriteBundleRecord(repoRoot, preview.BundleRecord)
	if err != nil {
		return rollbackOnError(fmt.Errorf("write bundle record: %w", err))
	}
	writtenPaths = append(writtenPaths, mustRepoRelativePath(repoRoot, bundlePath))

	var memberResult MutateMembersResult
	previousMemberIDs := append([]string(nil), preview.OverriddenInstallOrbitIDs...)
	for _, shrinkPlan := range bundleOverrideShrinkPlans {
		previousMemberIDs = append(previousMemberIDs, shrinkPlan.RemovedMemberIDs...)
	}
	if hasCleanupPlan {
		previousMemberIDs = append(previousMemberIDs, cleanupPlan.PreviousMemberIDs...)
	}
	previousMemberIDs = slicesCompactStrings(previousMemberIDs)
	if len(previousMemberIDs) > 0 {
		memberResult, err = ReplaceBundleMembers(ctx, repoRoot, preview.BundleRecord.HarnessID, previousMemberIDs, preview.Source.MemberIDs(), preview.BundleRecord.AppliedAt)
		if err != nil {
			return rollbackOnError(fmt.Errorf("replace bundle-backed members: %w", err))
		}
	} else {
		memberResult, err = AddBundleMembers(ctx, repoRoot, preview.BundleRecord.HarnessID, preview.Source.MemberIDs(), preview.BundleRecord.AppliedAt)
		if err != nil {
			return rollbackOnError(fmt.Errorf("record bundle-backed members: %w", err))
		}
	}
	writtenPaths = append(writtenPaths, mustRepoRelativePath(repoRoot, memberResult.ManifestPath))

	if hasCleanupPlan {
		runBeforeBundleOwnedCleanupHook(repoRoot, preview.Source.Manifest.Template.HarnessID, cleanupPlan)
		removedPaths, err := applyBundleOwnedCleanup(repoRoot, preview.Source.Manifest.Template.HarnessID, cleanupPlan)
		if err != nil {
			return rollbackOnError(fmt.Errorf("remove stale bundle-owned paths: %w", err))
		}
		writtenPaths = append(writtenPaths, removedPaths...)
	}
	for orbitID, cleanupPlan := range installOverrideCleanupPlans {
		removedPaths, err := orbittemplate.ApplyInstallOwnedCleanup(repoRoot, orbitID, cleanupPlan)
		if err != nil {
			return rollbackOnError(fmt.Errorf("remove stale install-owned paths for override target %q: %w", orbitID, err))
		}
		writtenPaths = append(writtenPaths, removedPaths...)

		installRecordPath, err := InstallRecordPath(repoRoot, orbitID)
		if err != nil {
			return rollbackOnError(fmt.Errorf("build install record path for override target %q: %w", orbitID, err))
		}
		if err := os.Remove(installRecordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return rollbackOnError(fmt.Errorf("delete install record for override target %q: %w", orbitID, err))
		}
		writtenPaths = append(writtenPaths, mustRepoRelativePath(repoRoot, installRecordPath))
	}
	for _, harnessID := range sortedBundleShrinkPlanKeys(bundleOverrideShrinkPlans) {
		removedPaths, err := ApplyBundleMemberShrinkPlan(repoRoot, bundleOverrideShrinkPlans[harnessID])
		if err != nil {
			return rollbackOnError(fmt.Errorf("apply bundle shrink for override target harness %q: %w", harnessID, err))
		}
		writtenPaths = append(writtenPaths, removedPaths...)
	}

	sort.Strings(writtenPaths)
	writtenPaths = slicesCompactStrings(writtenPaths)
	tx.Commit()

	return TemplateInstallResult{
		Preview:      preview,
		Runtime:      memberResult.Runtime,
		ManifestPath: memberResult.ManifestPath,
		BundlePath:   bundlePath,
		WrittenPaths: writtenPaths,
	}, nil
}

func buildTemplateInstallTransactionPaths(
	preview TemplateInstallPreview,
	hasCleanupPlan bool,
	cleanupPlan bundleOwnedCleanupPlan,
	installOverrideCleanupPlans map[string]orbittemplate.InstallOwnedCleanupPlan,
	bundleOverrideShrinkPlans map[string]BundleMemberShrinkPlan,
) ([]string, error) {
	paths := make([]string, 0, len(preview.RenderedDefinitionFiles)+len(preview.RenderedFiles)+len(cleanupPlan.DeletePaths)+5)
	for _, file := range preview.RenderedDefinitionFiles {
		paths = append(paths, file.Path)
	}
	for _, file := range preview.RenderedFiles {
		paths = append(paths, file.Path)
	}
	if preview.RenderedRootAgentsFile != nil {
		paths = append(paths, rootAgentsPath)
	}
	if preview.VarsFile != nil {
		paths = append(paths, VarsRepoPath())
	}

	bundleRepoPath, err := BundleRecordRepoPath(preview.Source.Manifest.Template.HarnessID)
	if err != nil {
		return nil, fmt.Errorf("build bundle record path: %w", err)
	}
	paths = append(paths, bundleRepoPath, ManifestRepoPath())

	if hasCleanupPlan {
		paths = append(paths, cleanupPlan.DeletePaths...)
		if cleanupPlan.RemoveRootAgentsBlock {
			paths = append(paths, rootAgentsPath)
		}
	}
	for orbitID, cleanupPlan := range installOverrideCleanupPlans {
		paths = append(paths, cleanupPlan.DeletePaths...)
		if cleanupPlan.RemoveSharedAgentsFile {
			paths = append(paths, rootAgentsPath)
		}
		installRecordRepoPath, err := InstallRecordRepoPath(orbitID)
		if err != nil {
			return nil, fmt.Errorf("build install record path: %w", err)
		}
		paths = append(paths, installRecordRepoPath)
	}
	for harnessID, shrinkPlan := range bundleOverrideShrinkPlans {
		paths = append(paths, shrinkPlan.DeletePaths...)
		if shrinkPlan.RemoveRootAgentsBlock {
			paths = append(paths, rootAgentsPath)
		}
		bundleRepoPath, err := BundleRecordRepoPath(harnessID)
		if err != nil {
			return nil, fmt.Errorf("build bundle record path: %w", err)
		}
		paths = append(paths, bundleRepoPath)
	}

	return slicesCompactStrings(paths), nil
}

func buildTemplateInstallOverrideCleanupPlans(
	ctx context.Context,
	repoRoot string,
	preview TemplateInstallPreview,
) (map[string]orbittemplate.InstallOwnedCleanupPlan, error) {
	plans := make(map[string]orbittemplate.InstallOwnedCleanupPlan, len(preview.OverriddenInstallOrbitIDs))
	incomingOwnedPaths := make(map[string]struct{}, len(preview.RenderedDefinitionFiles)+len(preview.RenderedFiles))
	for _, file := range preview.RenderedDefinitionFiles {
		incomingOwnedPaths[file.Path] = struct{}{}
	}
	for _, file := range preview.RenderedFiles {
		incomingOwnedPaths[file.Path] = struct{}{}
	}

	for _, orbitID := range preview.OverriddenInstallOrbitIDs {
		record, err := LoadInstallRecord(repoRoot, orbitID)
		if err != nil {
			return nil, fmt.Errorf("load install record for override target %q: %w", orbitID, err)
		}
		cleanupPlan, err := orbittemplate.BuildInstallOwnedCleanupPlan(ctx, repoRoot, record, orbittemplate.TemplateApplyPreview{})
		if err != nil {
			return nil, fmt.Errorf("reconstruct existing install ownership for override target %q: %w", orbitID, err)
		}
		filteredDeletePaths := make([]string, 0, len(cleanupPlan.DeletePaths))
		for _, path := range cleanupPlan.DeletePaths {
			if _, stillOwned := incomingOwnedPaths[path]; stillOwned {
				continue
			}
			filteredDeletePaths = append(filteredDeletePaths, path)
		}
		cleanupPlan.DeletePaths = filteredDeletePaths
		plans[orbitID] = cleanupPlan
	}
	return plans, nil
}

func buildTemplateInstallBundleOverrideShrinkPlans(
	ctx context.Context,
	repoRoot string,
	preview TemplateInstallPreview,
) (map[string]BundleMemberShrinkPlan, error) {
	if len(preview.OverriddenBundleMembers) == 0 {
		return map[string]BundleMemberShrinkPlan{}, nil
	}

	removedByHarness := make(map[string][]string)
	for _, target := range preview.OverriddenBundleMembers {
		removedByHarness[target.OwnerHarnessID] = append(removedByHarness[target.OwnerHarnessID], target.OrbitID)
	}

	preservedPaths := make(map[string]struct{}, len(preview.RenderedDefinitionFiles)+len(preview.RenderedFiles)+1)
	for _, file := range preview.RenderedDefinitionFiles {
		preservedPaths[file.Path] = struct{}{}
	}
	for _, file := range preview.RenderedFiles {
		preservedPaths[file.Path] = struct{}{}
	}
	if preview.RenderedRootAgentsFile != nil {
		preservedPaths[preview.RenderedRootAgentsFile.Path] = struct{}{}
	}

	plans := make(map[string]BundleMemberShrinkPlan, len(removedByHarness))
	for harnessID, removedMemberIDs := range removedByHarness {
		record, err := LoadBundleRecord(repoRoot, harnessID)
		if err != nil {
			return nil, fmt.Errorf("load bundle record for override target harness %q: %w", harnessID, err)
		}
		plan, err := BuildBundleMemberShrinkPlan(ctx, repoRoot, record, removedMemberIDs)
		if err != nil {
			return nil, fmt.Errorf("build bundle shrink plan for override target harness %q: %w", harnessID, err)
		}
		plans[harnessID] = FilterBundleMemberShrinkPlanDeletePaths(plan, preservedPaths)
	}

	return plans, nil
}

func templateInstallOverrideRequested(overrideOrbitIDs map[string]struct{}, orbitID string) bool {
	_, ok := overrideOrbitIDs[orbitID]
	return ok
}

type bundleOwnedCleanupPlan struct {
	DeletePaths           []string
	RemoveRootAgentsBlock bool
	PreviousMemberIDs     []string
}

// Test-only hook for deterministic stale bundle-owned cleanup failure injection.
var beforeBundleOwnedCleanupHook func(repoRoot string, harnessID string, plan bundleOwnedCleanupPlan)

func runBeforeBundleOwnedCleanupHook(repoRoot string, harnessID string, plan bundleOwnedCleanupPlan) {
	if beforeBundleOwnedCleanupHook == nil {
		return
	}
	beforeBundleOwnedCleanupHook(repoRoot, harnessID, plan)
}

// TemplateInstallResult contains one successful harness template install write result.
type TemplateInstallResult struct {
	Preview      TemplateInstallPreview
	Runtime      RuntimeFile
	ManifestPath string
	BundlePath   string
	WrittenPaths []string
}

func buildRenderedTemplateInstallPayload(
	ctx context.Context,
	input TemplateInstallPreviewInput,
	localInputs templateInstallLocalInputs,
) (map[string]bindings.ResolvedBinding, *bindings.VarsFile, []orbittemplate.CandidateFile, *orbittemplate.CandidateFile, *orbittemplate.InstallVariablesSnapshot, []string, error) {
	ordinaryFiles, rootAgentsFile := splitRootAgentsTemplateFiles(input.Source.Files)
	renderedFiles := cloneCandidateFiles(ordinaryFiles)
	var renderedRootAgentsFile *orbittemplate.CandidateFile
	if rootAgentsFile != nil {
		cloned := cloneInstallCandidateFile(*rootAgentsFile)
		renderedRootAgentsFile = &cloned
	}

	declared := make(map[string]bindings.VariableDeclaration, len(input.Source.Manifest.Variables))
	for name, spec := range input.Source.Manifest.Variables {
		declared[name] = bindings.VariableDeclaration{
			Description: spec.Description,
			Required:    spec.Required,
		}
	}
	if len(declared) == 0 {
		return map[string]bindings.ResolvedBinding{}, nil, renderedFiles, renderedRootAgentsFile, nil, []string{}, nil
	}

	bindingsFile := localInputs.bindingsFile
	repoVarsFile := localInputs.repoVarsFile
	hasRepoVarsFile := localInputs.hasRepoVarsFile

	mergeResult, err := resolveTemplateInstallBindings(
		ctx,
		declared,
		bindingsFile,
		repoVarsFile.Variables,
		input.Interactive,
		input.Prompter,
		input.EditorMode,
		input.Editor,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("merge bindings: %w", err)
	}

	unresolvedNames := templateInstallUnresolvedBindingNames(mergeResult.Unresolved)
	if len(unresolvedNames) > 0 && input.RequireResolvedBindings {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("missing required bindings: %s", strings.Join(unresolvedNames, ", "))
	}

	renderValues := make(map[string]string, len(mergeResult.Resolved))
	for name, resolved := range mergeResult.Resolved {
		renderValues[name] = resolved.Value
	}

	renderedFiles, err = renderTemplateInstallFiles(ordinaryFiles, renderValues, !input.RequireResolvedBindings)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("render harness template files: %w", err)
	}
	if rootAgentsFile != nil {
		renderedAgentsFiles, err := renderTemplateInstallFiles(
			[]orbittemplate.CandidateFile{*rootAgentsFile},
			renderValues,
			!input.RequireResolvedBindings,
		)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("render harness template AGENTS.md: %w", err)
		}
		renderedRootAgentsFile = &renderedAgentsFiles[0]
	}

	varsFile := planTemplateInstallBindingsWrite(repoVarsFile, hasRepoVarsFile, mergeResult.Resolved)
	variablesSnapshot := orbittemplate.BuildInstallVariablesSnapshot(declared, mergeResult)
	if variablesSnapshot != nil {
		observed := collectTemplateInstallObservedRuntimeUnresolved(
			renderedFiles,
			renderedRootAgentsFile,
		)
		if len(observed) > 0 {
			variablesSnapshot.ObservedRuntimeUnresolved = observed
		}
	}
	warnings := []string{}
	if len(unresolvedNames) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"install kept harness template variables unresolved: %s",
			strings.Join(unresolvedNames, ", "),
		))
	}

	return mergeResult.Resolved, varsFile, renderedFiles, renderedRootAgentsFile, variablesSnapshot, warnings, nil
}

func buildTemplateInstallMaterialization(
	ctx context.Context,
	input TemplateInstallPreviewInput,
	localInputs templateInstallLocalInputs,
) (templateInstallMaterialization, error) {
	resolvedBindings, varsFile, renderedFiles, renderedRootAgentsFile, variablesSnapshot, warnings, err := buildRenderedTemplateInstallPayload(ctx, input, localInputs)
	if err != nil {
		return templateInstallMaterialization{}, err
	}
	renderedDefinitionFiles, err := materializeRuntimeDefinitionFiles(input.Source.DefinitionFiles)
	if err != nil {
		return templateInstallMaterialization{}, err
	}
	bundleRecord, err := buildBundleInstallRecord(input.Source, input.InstallSource, renderedDefinitionFiles, renderedFiles, renderedRootAgentsFile, variablesSnapshot, input.Now)
	if err != nil {
		return templateInstallMaterialization{}, err
	}

	return templateInstallMaterialization{
		ResolvedBindings:        resolvedBindings,
		RenderedDefinitionFiles: renderedDefinitionFiles,
		RenderedFiles:           renderedFiles,
		RenderedRootAgentsFile:  renderedRootAgentsFile,
		VarsFile:                varsFile,
		BundleRecord:            bundleRecord,
		Warnings:                warnings,
	}, nil
}

func materializeRuntimeDefinitionFiles(files []orbittemplate.CandidateFile) ([]orbittemplate.CandidateFile, error) {
	rendered := cloneCandidateFiles(files)
	for index, file := range rendered {
		name := strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path))
		runtimePath, err := orbit.HostedDefinitionRelativePath(name)
		if err != nil {
			return nil, fmt.Errorf("build runtime definition path for %q: %w", file.Path, err)
		}
		rendered[index].Path = runtimePath
	}

	return rendered, nil
}

func analyzeTemplateInstallPathConflict(
	repoRoot string,
	file orbittemplate.CandidateFile,
	statusByPath map[string]git.StatusEntry,
	allowedOwnedPaths map[string]struct{},
) ([]orbittemplate.ApplyConflict, error) {
	conflicts := make([]orbittemplate.ApplyConflict, 0, 2)
	filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))

	//nolint:gosec // The target path is repo-local and derived from one validated template candidate path.
	if data, err := os.ReadFile(filename); err == nil {
		if !bytes.Equal(data, file.Content) {
			if _, ok := allowedOwnedPaths[file.Path]; !ok {
				conflicts = append(conflicts, orbittemplate.ApplyConflict{
					Path:    file.Path,
					Message: "target path already exists with different content",
				})
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read runtime path %s: %w", file.Path, err)
	}

	if status, ok := statusByPath[file.Path]; ok {
		if _, ok := allowedOwnedPaths[file.Path]; !ok {
			conflicts = append(conflicts, orbittemplate.ApplyConflict{
				Path:    file.Path,
				Message: fmt.Sprintf("target path has uncommitted worktree status %s", status.Code),
			})
		}
	}

	return conflicts, nil
}

func analyzeBundleAgentsInstallPreview(
	repoRoot string,
	harnessID string,
	statusByPath map[string]git.StatusEntry,
	allowExistingStatus bool,
) ([]orbittemplate.ApplyConflict, error) {
	conflicts := make([]orbittemplate.ApplyConflict, 0, 1)
	if status, ok := statusByPath[rootAgentsPath]; ok {
		if !allowExistingStatus {
			conflicts = append(conflicts, orbittemplate.ApplyConflict{
				Path:    rootAgentsPath,
				Message: fmt.Sprintf("AGENTS lane has uncommitted worktree status %s", status.Code),
			})
		}
	}

	filename := filepath.Join(repoRoot, rootAgentsPath)
	//nolint:gosec // The runtime AGENTS path is fixed under the repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return conflicts, nil
		}
		return nil, fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	if _, err := orbittemplate.ParseRuntimeAgentsDocument(data); err != nil {
		conflicts = append(conflicts, orbittemplate.ApplyConflict{
			Path:    rootAgentsPath,
			Message: fmt.Sprintf("runtime AGENTS.md is invalid for harness block merge (%s)", harnessID),
		})
	}

	return conflicts, nil
}

func buildBundleInstallRecord(
	source LocalTemplateInstallSource,
	installSource orbittemplate.Source,
	definitionFiles []orbittemplate.CandidateFile,
	renderedFiles []orbittemplate.CandidateFile,
	renderedRootAgentsFile *orbittemplate.CandidateFile,
	variablesSnapshot *orbittemplate.InstallVariablesSnapshot,
	now time.Time,
) (BundleRecord, error) {
	ownedPaths := make([]string, 0, len(definitionFiles)+len(renderedFiles)+1)
	for _, file := range definitionFiles {
		ownedPaths = append(ownedPaths, file.Path)
	}
	for _, file := range renderedFiles {
		ownedPaths = append(ownedPaths, file.Path)
	}
	if renderedRootAgentsFile != nil {
		ownedPaths = append(ownedPaths, renderedRootAgentsFile.Path)
	}
	sort.Strings(ownedPaths)
	ownedPaths = slicesCompactStrings(ownedPaths)
	ownedPathDigests, rootAgentsDigest := buildBundleOwnershipDigests(definitionFiles, renderedFiles, renderedRootAgentsFile)
	agentAddons, err := buildBundleAgentAddonsSnapshot(definitionFiles, renderedFiles)
	if err != nil {
		return BundleRecord{}, err
	}
	if len(agentAddons.Hooks) == 0 {
		agentAddons = nil
	}

	return BundleRecord{
		SchemaVersion:        bundleRecordSchemaVersion,
		HarnessID:            source.Manifest.Template.HarnessID,
		Template:             installSource,
		RecommendedFramework: source.Frameworks.RecommendedFramework,
		AgentConfig:          cloneOptionalAgentConfigFile(source.AgentConfig),
		AgentOverlays:        cloneBundleAgentOverlayContents(source.AgentOverlays),
		AgentAddons:          agentAddons,
		MemberIDs:            source.MemberIDs(),
		AppliedAt:            resolveMutationTime(now),
		IncludesRootAgents:   renderedRootAgentsFile != nil,
		OwnedPaths:           ownedPaths,
		OwnedPathDigests:     ownedPathDigests,
		RootAgentsDigest:     rootAgentsDigest,
		Variables:            variablesSnapshot,
	}, nil
}

func buildBundleAgentAddonsSnapshot(
	definitionFiles []orbittemplate.CandidateFile,
	renderedFiles []orbittemplate.CandidateFile,
) (*orbittemplate.AgentAddonsSnapshot, error) {
	combined := orbittemplate.AgentAddonsSnapshot{}
	for _, definitionFile := range definitionFiles {
		spec, err := orbit.ParseHostedOrbitSpecData(definitionFile.Content, definitionFile.Path)
		if err != nil {
			definition, legacyErr := orbit.ParseDefinitionData(definitionFile.Content, definitionFile.Path)
			if legacyErr != nil {
				return nil, fmt.Errorf("parse bundle agent add-on definition %s: %w", definitionFile.Path, err)
			}
			spec = orbit.OrbitSpecFromDefinition(definition)
		}
		snapshot, err := orbittemplate.BuildAgentAddonsSnapshot(spec, renderedFiles)
		if err != nil {
			return nil, fmt.Errorf("snapshot bundle agent add-ons for orbit %q: %w", spec.ID, err)
		}
		if snapshot == nil {
			continue
		}
		combined.Hooks = append(combined.Hooks, snapshot.Hooks...)
	}
	if len(combined.Hooks) == 0 {
		return &orbittemplate.AgentAddonsSnapshot{}, nil
	}
	if err := orbittemplate.ValidateAgentAddonsSnapshot(combined); err != nil {
		return nil, fmt.Errorf("validate bundle agent add-ons snapshot: %w", err)
	}

	return &combined, nil
}

func cloneOptionalAgentConfigFile(file *AgentConfigFile) *AgentConfigFile {
	if file == nil {
		return nil
	}

	cloned := *file
	return &cloned
}

func cloneBundleAgentOverlayContents(files map[string]AgentOverlayFile) map[string]string {
	if len(files) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(files))
	for agentID, file := range files {
		cloned[agentID] = string(file.Content)
	}

	return cloned
}

func applyBundleOwnedCleanup(repoRoot string, harnessID string, plan bundleOwnedCleanupPlan) ([]string, error) {
	removed := make([]string, 0, len(plan.DeletePaths)+1)
	for _, path := range plan.DeletePaths {
		filename := filepath.Join(repoRoot, filepath.FromSlash(path))
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale bundle-owned path %s: %w", path, err)
		}
		removed = append(removed, path)
	}

	if plan.RemoveRootAgentsBlock {
		if err := RemoveBundleAgentsPayload(repoRoot, harnessID); err != nil {
			return nil, fmt.Errorf("remove stale bundle AGENTS block: %w", err)
		}
		removed = append(removed, rootAgentsPath)
	}

	sort.Strings(removed)
	return removed, nil
}

func writeTemplateCandidateFile(repoRoot string, file orbittemplate.CandidateFile) error {
	filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
	perm, err := git.FilePermForMode(file.Mode)
	if err != nil {
		return fmt.Errorf("resolve rendered file mode %s: %w", file.Path, err)
	}
	if err := contractutil.AtomicWriteFileMode(filename, file.Content, perm); err != nil {
		return fmt.Errorf("write rendered file %s: %w", file.Path, err)
	}
	return nil
}

func splitRootAgentsTemplateFiles(files []orbittemplate.CandidateFile) ([]orbittemplate.CandidateFile, *orbittemplate.CandidateFile) {
	ordinary := make([]orbittemplate.CandidateFile, 0, len(files))
	var rootAgentsFile *orbittemplate.CandidateFile
	for _, file := range files {
		if file.Path == rootAgentsPath {
			cloned := cloneInstallCandidateFile(file)
			rootAgentsFile = &cloned
			continue
		}
		ordinary = append(ordinary, cloneInstallCandidateFile(file))
	}
	return ordinary, rootAgentsFile
}

func compactTemplateInstallBundleOverrideTargets(targets []templateInstallBundleOverrideTarget) []templateInstallBundleOverrideTarget {
	if len(targets) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(targets))
	compacted := make([]templateInstallBundleOverrideTarget, 0, len(targets))
	for _, target := range targets {
		key := target.OwnerHarnessID + "\x00" + target.OrbitID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		compacted = append(compacted, target)
	}

	sort.Slice(compacted, func(left, right int) bool {
		if compacted[left].OwnerHarnessID == compacted[right].OwnerHarnessID {
			return compacted[left].OrbitID < compacted[right].OrbitID
		}
		return compacted[left].OwnerHarnessID < compacted[right].OwnerHarnessID
	})

	return compacted
}

func sortedBundleShrinkPlanKeys(plans map[string]BundleMemberShrinkPlan) []string {
	keys := make([]string, 0, len(plans))
	for harnessID := range plans {
		keys = append(keys, harnessID)
	}
	sort.Strings(keys)
	return keys
}

func renderTemplateInstallFiles(
	files []orbittemplate.CandidateFile,
	renderValues map[string]string,
	allowUnresolved bool,
) ([]orbittemplate.CandidateFile, error) {
	if allowUnresolved {
		rendered, err := orbittemplate.RenderTemplateFilesAllowingUnresolved(files, renderValues)
		if err != nil {
			return nil, fmt.Errorf("render relaxed template files: %w", err)
		}
		return rendered, nil
	}
	rendered, err := orbittemplate.RenderTemplateFiles(files, renderValues)
	if err != nil {
		return nil, fmt.Errorf("render strict template files: %w", err)
	}
	return rendered, nil
}

func templateInstallUnresolvedBindingNames(unresolved []bindings.UnresolvedBinding) []string {
	names := make([]string, 0, len(unresolved))
	for _, binding := range unresolved {
		names = append(names, binding.Name)
	}
	sort.Strings(names)

	return names
}

func collectTemplateInstallObservedRuntimeUnresolved(
	renderedFiles []orbittemplate.CandidateFile,
	renderedRootAgentsFile *orbittemplate.CandidateFile,
) []string {
	observed := make(map[string]struct{})
	for _, file := range renderedFiles {
		result := orbittemplate.ScanVariables([]orbittemplate.CandidateFile{file}, nil)
		for _, name := range result.Referenced {
			observed[name] = struct{}{}
		}
	}
	if renderedRootAgentsFile != nil {
		result := orbittemplate.ScanVariables([]orbittemplate.CandidateFile{*renderedRootAgentsFile}, nil)
		for _, name := range result.Referenced {
			observed[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(observed))
	for name := range observed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func cloneCandidateFiles(files []orbittemplate.CandidateFile) []orbittemplate.CandidateFile {
	cloned := make([]orbittemplate.CandidateFile, 0, len(files))
	for _, file := range files {
		cloned = append(cloned, cloneInstallCandidateFile(file))
	}
	return cloned
}

func cloneInstallCandidateFile(file orbittemplate.CandidateFile) orbittemplate.CandidateFile {
	return orbittemplate.CandidateFile{
		Path:    file.Path,
		Content: append([]byte(nil), file.Content...),
		Mode:    file.Mode,
	}
}

func loadOptionalTemplateInstallBindingsFile(filename string) (map[string]bindings.VariableBinding, error) {
	if strings.TrimSpace(filename) == "" {
		return map[string]bindings.VariableBinding{}, nil
	}

	//nolint:gosec // The bindings file path is an explicit user-provided local file path.
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}
	file, err := bindings.ParseVarsData(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	return file.Variables, nil
}

func loadTemplateInstallLocalInputs(ctx context.Context, repoRoot string, bindingsFilePath string) (templateInstallLocalInputs, error) {
	bindingsFile, err := loadOptionalTemplateInstallBindingsFile(bindingsFilePath)
	if err != nil {
		return templateInstallLocalInputs{}, fmt.Errorf("load --bindings file: %w", err)
	}
	repoVarsFile, hasRepoVarsFile, err := loadOptionalTemplateInstallRepoVarsFile(ctx, repoRoot)
	if err != nil {
		return templateInstallLocalInputs{}, fmt.Errorf("load harness vars: %w", err)
	}

	return templateInstallLocalInputs{
		bindingsFile:    bindingsFile,
		repoVarsFile:    repoVarsFile,
		hasRepoVarsFile: hasRepoVarsFile,
	}, nil
}

func loadOptionalTemplateInstallRepoVarsFile(ctx context.Context, repoRoot string) (bindings.VarsFile, bool, error) {
	empty := bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     map[string]bindings.VariableBinding{},
	}
	if _, err := os.Stat(VarsPath(repoRoot)); err == nil {
		file, err := LoadVarsFile(repoRoot)
		if err != nil {
			return bindings.VarsFile{}, false, err
		}
		return file, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return bindings.VarsFile{}, false, fmt.Errorf("stat harness vars in worktree: %w", err)
	}

	existsAtHead, err := git.PathExistsAtRev(ctx, repoRoot, "HEAD", VarsRepoPath())
	if err != nil {
		return bindings.VarsFile{}, false, fmt.Errorf("check harness vars at HEAD: %w", err)
	}
	if !existsAtHead {
		return empty, false, nil
	}

	file, err := LoadVarsFileWorktreeOrHEAD(ctx, repoRoot)
	if err != nil {
		return bindings.VarsFile{}, false, err
	}
	return file, true, nil
}

func planTemplateInstallBindingsWrite(
	existing bindings.VarsFile,
	hasExisting bool,
	resolved map[string]bindings.ResolvedBinding,
) *bindings.VarsFile {
	if len(resolved) == 0 {
		return nil
	}

	merged := make(map[string]bindings.VariableBinding, len(existing.Variables)+len(resolved))
	for name, binding := range existing.Variables {
		merged[name] = binding
	}
	scopedMerged := make(map[string]bindings.ScopedVariableBindings, len(existing.ScopedVariables))
	for namespace, scoped := range existing.ScopedVariables {
		scopedMerged[namespace] = bindings.ScopedVariableBindings{
			Variables: cloneTemplateInstallVariableBindings(scoped.Variables),
		}
	}

	changed := false
	for name, binding := range resolved {
		if binding.Source == bindings.SourceRepoVars || binding.Source == bindings.SourceRepoVarsScoped {
			continue
		}

		next := bindings.VariableBinding{
			Value:       binding.Value,
			Description: binding.Description,
		}
		if current, ok := merged[name]; ok && current == next {
			continue
		}
		merged[name] = next
		changed = true
	}

	if !changed {
		return nil
	}

	planned := bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     merged,
	}
	if len(scopedMerged) > 0 {
		planned.ScopedVariables = scopedMerged
	}
	if hasExisting && existing.SchemaVersion == planned.SchemaVersion && mapsEqualBindings(existing.Variables, planned.Variables) {
		return nil
	}
	return &planned
}

func cloneTemplateInstallVariableBindings(values map[string]bindings.VariableBinding) map[string]bindings.VariableBinding {
	if values == nil {
		return nil
	}
	cloned := make(map[string]bindings.VariableBinding, len(values))
	for name, binding := range values {
		cloned[name] = binding
	}
	return cloned
}

func resolveTemplateInstallBindings(
	ctx context.Context,
	declared map[string]bindings.VariableDeclaration,
	bindingsFile map[string]bindings.VariableBinding,
	repoVars map[string]bindings.VariableBinding,
	interactive bool,
	prompter orbittemplate.BindingPrompter,
	editorMode bool,
	editor orbittemplate.Editor,
) (bindings.MergeResult, error) {
	mergeInput := bindings.MergeInput{
		Declared:     declared,
		BindingsFile: bindingsFile,
		RepoVars:     repoVars,
	}
	mergeResult, err := bindings.Merge(mergeInput)
	if err != nil {
		return bindings.MergeResult{}, fmt.Errorf("merge declared bindings before interactive fill: %w", err)
	}
	if len(mergeResult.Unresolved) == 0 {
		return mergeResult, nil
	}

	switch {
	case interactive:
		if prompter == nil {
			return bindings.MergeResult{}, fmt.Errorf("interactive apply requires a binding prompter")
		}
		fillIn, err := prompter.PromptBindings(ctx, mergeResult.Unresolved)
		if err != nil {
			return bindings.MergeResult{}, fmt.Errorf("prompt for missing bindings: %w", err)
		}
		mergeInput.FillIn = fillIn
		mergeInput.FillSource = bindings.SourceInteractive
	case editorMode:
		fillIn, err := editMissingTemplateInstallBindings(ctx, mergeResult.Unresolved, editor)
		if err != nil {
			return bindings.MergeResult{}, fmt.Errorf("edit missing bindings: %w", err)
		}
		mergeInput.FillIn = fillIn
		mergeInput.FillSource = bindings.SourceEditor
	default:
		return mergeResult, nil
	}

	mergeResult, err = bindings.Merge(mergeInput)
	if err != nil {
		return bindings.MergeResult{}, fmt.Errorf("merge bindings after fill: %w", err)
	}
	return mergeResult, nil
}

func editMissingTemplateInstallBindings(
	ctx context.Context,
	unresolved []bindings.UnresolvedBinding,
	editor orbittemplate.Editor,
) (map[string]bindings.VariableBinding, error) {
	if editor == nil {
		return nil, fmt.Errorf("editor apply requires an editor")
	}

	declared := make(map[string]bindings.VariableDeclaration, len(unresolved))
	for _, missing := range unresolved {
		declared[missing.Name] = bindings.VariableDeclaration{
			Description: missing.Description,
			Required:    missing.Required,
		}
	}
	skeleton := bindings.SkeletonFromDeclarations(declared)
	data, err := bindings.MarshalVarsFile(skeleton)
	if err != nil {
		return nil, fmt.Errorf("encode editor bindings skeleton: %w", err)
	}

	tempFile, err := os.CreateTemp("", "orbit-bindings-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create editor bindings temp file: %w", err)
	}
	tempName := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("close editor bindings temp file: %w", err)
	}
	defer func() { _ = os.Remove(tempName) }()

	//nolint:gosec,nolintlint // tempName comes from os.CreateTemp in this function; newer gosec releases no longer flag it.
	if err := os.WriteFile(tempName, data, 0o600); err != nil {
		return nil, fmt.Errorf("write editor bindings skeleton: %w", err)
	}
	if err := editor.Edit(ctx, tempName); err != nil {
		return nil, fmt.Errorf("run bindings editor: %w", err)
	}

	//nolint:gosec // tempName comes from os.CreateTemp in this function and is only rewritten by the configured editor process.
	editedData, err := os.ReadFile(tempName)
	if err != nil {
		return nil, fmt.Errorf("read edited bindings skeleton: %w", err)
	}
	editedFile, err := bindings.ParseVarsData(editedData)
	if err != nil {
		return nil, fmt.Errorf("parse edited bindings skeleton: %w", err)
	}

	filled := make(map[string]bindings.VariableBinding, len(editedFile.Variables))
	for name, binding := range editedFile.Variables {
		if strings.TrimSpace(binding.Value) == "" {
			continue
		}
		filled[name] = binding
	}
	return filled, nil
}

func mapsEqualBindings(left map[string]bindings.VariableBinding, right map[string]bindings.VariableBinding) bool {
	if len(left) != len(right) {
		return false
	}
	for name, leftBinding := range left {
		if rightBinding, ok := right[name]; !ok || rightBinding != leftBinding {
			return false
		}
	}
	return true
}

func mustRepoRelativePath(repoRoot string, absolutePath string) string {
	relativePath, err := filepath.Rel(repoRoot, absolutePath)
	if err != nil {
		return filepath.ToSlash(absolutePath)
	}
	return filepath.ToSlash(relativePath)
}

func slicesCompactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	sort.Strings(values)
	deduped := values[:0]
	for _, value := range values {
		if len(deduped) > 0 && deduped[len(deduped)-1] == value {
			continue
		}
		deduped = append(deduped, value)
	}
	return deduped
}
