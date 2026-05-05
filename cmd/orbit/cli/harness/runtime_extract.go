package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// ExtractRuntimeMemberDetachedResult captures one successful bundle-owned detached extract.
type ExtractRuntimeMemberDetachedResult struct {
	ManifestPath        string
	Runtime             RuntimeFile
	WrittenPaths        []string
	RemovedPaths        []string
	RemovedAgentsBlock  bool
	DeletedBundleRecord bool
}

// ExtractRuntimeMemberToInstallResult captures one successful extract that
// rehomes a bundle-backed runtime member into install_orbit provenance.
type ExtractRuntimeMemberToInstallResult struct {
	ManifestPath        string
	InstallRecordPath   string
	Runtime             RuntimeFile
	TargetBranch        string
	TemplateRef         string
	TemplateCommit      string
	WrittenPaths        []string
	RemovedPaths        []string
	RemovedAgentsBlock  bool
	DeletedBundleRecord bool
}

type bundleMemberDetachPlan struct {
	ExistingRecord        BundleRecord
	NextRecord            *BundleRecord
	RemoveRootAgentsBlock bool
	DeleteBundleRecord    bool
}

type preparedBundleMemberExtract struct {
	Runtime     RuntimeFile
	TargetIndex int
	Target      RuntimeMember
	Record      BundleRecord
	DetachPlan  bundleMemberDetachPlan
}

type bundleMemberExtractMutationInput struct {
	RepoRoot      string
	Prepared      preparedBundleMemberExtract
	NextSource    string
	NextOrigin    *orbittemplate.Source
	InstallRecord *orbittemplate.InstallRecord
	MutationTime  time.Time
}

type bundleMemberExtractMutationResult struct {
	ManifestPath        string
	InstallRecordPath   string
	Runtime             RuntimeFile
	WrittenPaths        []string
	RemovedPaths        []string
	RemovedAgentsBlock  bool
	DeletedBundleRecord bool
}

// ExtractRuntimeMemberDetached detaches one bundle-backed runtime member into a standalone manual member.
func ExtractRuntimeMemberDetached(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	now time.Time,
) (ExtractRuntimeMemberDetachedResult, error) {
	prepared, err := prepareBundleMemberExtract(ctx, repoRoot, orbitID)
	if err != nil {
		return ExtractRuntimeMemberDetachedResult{}, err
	}
	mutation, err := applyBundleMemberExtractMutation(ctx, bundleMemberExtractMutationInput{
		RepoRoot:     repoRoot,
		Prepared:     prepared,
		NextSource:   MemberSourceManual,
		NextOrigin:   prepared.Target.LastStandaloneOrigin,
		MutationTime: now,
	})
	if err != nil {
		return ExtractRuntimeMemberDetachedResult{}, err
	}

	return ExtractRuntimeMemberDetachedResult{
		ManifestPath:        mutation.ManifestPath,
		Runtime:             mutation.Runtime,
		WrittenPaths:        mutation.WrittenPaths,
		RemovedPaths:        mutation.RemovedPaths,
		RemovedAgentsBlock:  mutation.RemovedAgentsBlock,
		DeletedBundleRecord: mutation.DeletedBundleRecord,
	}, nil
}

// ExtractRuntimeMemberToInstall extracts one bundle-backed runtime member into
// install_orbit provenance by first saving the current runtime orbit to a
// target template branch.
func ExtractRuntimeMemberToInstall(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	targetBranch string,
	now time.Time,
) (ExtractRuntimeMemberToInstallResult, error) {
	prepared, err := prepareBundleMemberExtract(ctx, repoRoot, orbitID)
	if err != nil {
		return ExtractRuntimeMemberToInstallResult{}, err
	}

	saveResult, err := saveRuntimeMemberTemplateBranch(ctx, repoRoot, orbitID, targetBranch, now)
	if err != nil {
		return ExtractRuntimeMemberToInstallResult{}, err
	}

	record := orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       orbitID,
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      saveResult.WriteResult.Branch,
			TemplateCommit: saveResult.WriteResult.Commit,
		},
		AppliedAt: resolveMutationTime(now),
	}
	mutation, err := applyBundleMemberExtractMutation(ctx, bundleMemberExtractMutationInput{
		RepoRoot:      repoRoot,
		Prepared:      prepared,
		NextSource:    MemberSourceInstallOrbit,
		NextOrigin:    nil,
		InstallRecord: &record,
		MutationTime:  now,
	})
	if err != nil {
		return ExtractRuntimeMemberToInstallResult{}, rollbackSavedTemplateBranch(ctx, repoRoot, saveResult.WriteResult.Branch, err)
	}

	return ExtractRuntimeMemberToInstallResult{
		ManifestPath:        mutation.ManifestPath,
		InstallRecordPath:   mutation.InstallRecordPath,
		Runtime:             mutation.Runtime,
		TargetBranch:        saveResult.WriteResult.Branch,
		TemplateRef:         saveResult.WriteResult.Ref,
		TemplateCommit:      saveResult.WriteResult.Commit,
		WrittenPaths:        mutation.WrittenPaths,
		RemovedPaths:        mutation.RemovedPaths,
		RemovedAgentsBlock:  mutation.RemovedAgentsBlock,
		DeletedBundleRecord: mutation.DeletedBundleRecord,
	}, nil
}

// ExtractRuntimeMemberReuseOrigin extracts one bundle-backed runtime member
// back into install_orbit provenance using a stored standalone origin hint.
func ExtractRuntimeMemberReuseOrigin(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	now time.Time,
) (ExtractRuntimeMemberToInstallResult, error) {
	prepared, err := prepareBundleMemberExtract(ctx, repoRoot, orbitID)
	if err != nil {
		return ExtractRuntimeMemberToInstallResult{}, err
	}
	if prepared.Target.LastStandaloneOrigin == nil {
		return ExtractRuntimeMemberToInstallResult{}, fmt.Errorf(
			"bundle-backed member %q has no last_standalone_origin; --reuse-origin cannot determine a safe target branch",
			orbitID,
		)
	}
	if prepared.Target.LastStandaloneOrigin.SourceKind != orbittemplate.InstallSourceKindLocalBranch {
		return ExtractRuntimeMemberToInstallResult{}, fmt.Errorf(
			"bundle-backed member %q cannot reuse last_standalone_origin source_kind %q safely; only %q is currently supported",
			orbitID,
			prepared.Target.LastStandaloneOrigin.SourceKind,
			orbittemplate.InstallSourceKindLocalBranch,
		)
	}

	saveResult, err := saveRuntimeMemberTemplateBranch(
		ctx,
		repoRoot,
		orbitID,
		prepared.Target.LastStandaloneOrigin.SourceRef,
		now,
	)
	if err != nil {
		return ExtractRuntimeMemberToInstallResult{}, err
	}

	record := orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       orbitID,
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      saveResult.WriteResult.Branch,
			TemplateCommit: saveResult.WriteResult.Commit,
		},
		AppliedAt: resolveMutationTime(now),
	}
	mutation, err := applyBundleMemberExtractMutation(ctx, bundleMemberExtractMutationInput{
		RepoRoot:      repoRoot,
		Prepared:      prepared,
		NextSource:    MemberSourceInstallOrbit,
		NextOrigin:    nil,
		InstallRecord: &record,
		MutationTime:  now,
	})
	if err != nil {
		return ExtractRuntimeMemberToInstallResult{}, rollbackSavedTemplateBranch(ctx, repoRoot, saveResult.WriteResult.Branch, err)
	}

	return ExtractRuntimeMemberToInstallResult{
		ManifestPath:        mutation.ManifestPath,
		InstallRecordPath:   mutation.InstallRecordPath,
		Runtime:             mutation.Runtime,
		TargetBranch:        saveResult.WriteResult.Branch,
		TemplateRef:         saveResult.WriteResult.Ref,
		TemplateCommit:      saveResult.WriteResult.Commit,
		WrittenPaths:        mutation.WrittenPaths,
		RemovedPaths:        mutation.RemovedPaths,
		RemovedAgentsBlock:  mutation.RemovedAgentsBlock,
		DeletedBundleRecord: mutation.DeletedBundleRecord,
	}, nil
}

func saveRuntimeMemberTemplateBranch(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	targetBranch string,
	now time.Time,
) (orbittemplate.TemplateSaveResult, error) {
	targetBranch = strings.TrimSpace(targetBranch)
	if targetBranch == "" {
		return orbittemplate.TemplateSaveResult{}, fmt.Errorf("target branch must not be empty")
	}
	if err := orbittemplate.EnsureMemberHintExportSync(ctx, repoRoot, orbitID, "extracting"); err != nil {
		return orbittemplate.TemplateSaveResult{}, fmt.Errorf("ensure member hint export sync: %w", err)
	}
	briefSync, err := orbittemplate.EnsureBriefExportSync(ctx, repoRoot, orbitID, "extracting", false)
	if err != nil {
		return orbittemplate.TemplateSaveResult{}, fmt.Errorf("ensure brief export sync: %w", err)
	}

	previewInput := orbittemplate.TemplateSavePreviewInput{
		RepoRoot:     repoRoot,
		OrbitID:      orbitID,
		TargetBranch: targetBranch,
		Now:          now,
	}
	if briefSync.Warning != "" {
		previewInput.Warnings = append(previewInput.Warnings, briefSync.Warning)
	}
	preview, err := orbittemplate.BuildTemplateSavePreview(ctx, previewInput)
	if err != nil {
		return orbittemplate.TemplateSaveResult{}, fmt.Errorf("build template save preview: %w", err)
	}
	result, err := orbittemplate.WriteTemplateSavePreview(ctx, orbittemplate.TemplateSaveWriteInput{
		Preview: preview,
	})
	if err != nil {
		return orbittemplate.TemplateSaveResult{}, fmt.Errorf("save template branch: %w", err)
	}

	return result, nil
}

func rollbackSavedTemplateBranch(
	ctx context.Context,
	repoRoot string,
	targetBranch string,
	cause error,
) error {
	if err := gitpkg.DeleteLocalBranch(ctx, repoRoot, targetBranch); err != nil {
		return errors.Join(cause, fmt.Errorf("rollback saved template branch %q: %w", targetBranch, err))
	}
	return cause
}

func prepareBundleMemberExtract(
	ctx context.Context,
	repoRoot string,
	orbitID string,
) (preparedBundleMemberExtract, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return preparedBundleMemberExtract{}, fmt.Errorf("load harness runtime: %w", err)
	}

	targetIndex := -1
	for index, member := range runtimeFile.Members {
		if member.OrbitID != orbitID {
			continue
		}
		targetIndex = index
		break
	}
	if targetIndex < 0 {
		return preparedBundleMemberExtract{}, fmt.Errorf("member %q not found", orbitID)
	}

	targetMember := runtimeFile.Members[targetIndex]
	if targetMember.Source != MemberSourceInstallBundle {
		return preparedBundleMemberExtract{}, fmt.Errorf(
			"member %q is not bundle-backed; extract currently requires source %q",
			orbitID,
			MemberSourceInstallBundle,
		)
	}
	if strings.TrimSpace(targetMember.OwnerHarnessID) == "" {
		return preparedBundleMemberExtract{}, fmt.Errorf("bundle-backed member %q is missing owner_harness_id", orbitID)
	}

	record, err := LoadBundleRecord(repoRoot, targetMember.OwnerHarnessID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return preparedBundleMemberExtract{}, fmt.Errorf(
				"bundle-backed member %q has no bundle record for owner_harness_id %q",
				orbitID,
				targetMember.OwnerHarnessID,
			)
		}
		return preparedBundleMemberExtract{}, fmt.Errorf("load bundle record for %q: %w", targetMember.OwnerHarnessID, err)
	}

	detachPlan, err := buildBundleMemberDetachPlan(ctx, repoRoot, record, orbitID)
	if err != nil {
		return preparedBundleMemberExtract{}, fmt.Errorf("build extract plan for %q: %w", orbitID, err)
	}

	return preparedBundleMemberExtract{
		Runtime:     runtimeFile,
		TargetIndex: targetIndex,
		Target:      targetMember,
		Record:      record,
		DetachPlan:  detachPlan,
	}, nil
}

func applyBundleMemberExtractMutation(
	ctx context.Context,
	input bundleMemberExtractMutationInput,
) (bundleMemberExtractMutationResult, error) {
	touchedPaths := []string{ManifestRepoPath()}
	bundleRecordRepoPath, err := BundleRecordRepoPath(input.Prepared.Record.HarnessID)
	if err != nil {
		return bundleMemberExtractMutationResult{}, err
	}
	touchedPaths = append(touchedPaths, bundleRecordRepoPath)
	cleanCheckPaths := []string{}
	if input.Prepared.DetachPlan.RemoveRootAgentsBlock {
		touchedPaths = append(touchedPaths, rootAgentsPath)
		cleanCheckPaths = append(cleanCheckPaths, rootAgentsPath)
	}
	if input.InstallRecord != nil {
		installRecordRepoPath, err := InstallRecordRepoPath(input.InstallRecord.OrbitID)
		if err != nil {
			return bundleMemberExtractMutationResult{}, err
		}
		touchedPaths = append(touchedPaths, installRecordRepoPath)

		installRecordPath, err := InstallRecordPath(input.RepoRoot, input.InstallRecord.OrbitID)
		if err != nil {
			return bundleMemberExtractMutationResult{}, err
		}
		if _, err := os.Stat(installRecordPath); err == nil {
			return bundleMemberExtractMutationResult{}, fmt.Errorf(
				"member %q already has install provenance at %s",
				input.InstallRecord.OrbitID,
				installRecordPath,
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return bundleMemberExtractMutationResult{}, fmt.Errorf("stat %s: %w", installRecordPath, err)
		}
	}
	if err := ensureRuntimeRemovePathsClean(ctx, input.RepoRoot, input.Prepared.Target.OrbitID, cleanCheckPaths); err != nil {
		return bundleMemberExtractMutationResult{}, err
	}

	tx, err := BeginInstallTransaction(ctx, input.RepoRoot, touchedPaths)
	if err != nil {
		return bundleMemberExtractMutationResult{}, fmt.Errorf("begin extract transaction: %w", err)
	}
	rollbackOnError := func(cause error) (bundleMemberExtractMutationResult, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return bundleMemberExtractMutationResult{}, errors.Join(cause, fmt.Errorf("rollback extract transaction: %w", rollbackErr))
		}
		return bundleMemberExtractMutationResult{}, cause
	}

	writtenPaths := make([]string, 0, 3)
	removedPaths := make([]string, 0, 2)
	if input.Prepared.DetachPlan.DeleteBundleRecord {
		bundlePath, err := BundleRecordPath(input.RepoRoot, input.Prepared.DetachPlan.ExistingRecord.HarnessID)
		if err != nil {
			return rollbackOnError(fmt.Errorf("build bundle record path: %w", err))
		}
		if err := os.Remove(bundlePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return rollbackOnError(fmt.Errorf("delete bundle record %s: %w", input.Prepared.DetachPlan.ExistingRecord.HarnessID, err))
		}
		removedPaths = append(removedPaths, mustRepoRelativePath(input.RepoRoot, bundlePath))
	} else {
		if input.Prepared.DetachPlan.NextRecord == nil {
			return rollbackOnError(fmt.Errorf("extract plan is missing next bundle record"))
		}
		bundlePath, err := WriteBundleRecord(input.RepoRoot, *input.Prepared.DetachPlan.NextRecord)
		if err != nil {
			return rollbackOnError(fmt.Errorf("write extracted bundle record: %w", err))
		}
		writtenPaths = append(writtenPaths, mustRepoRelativePath(input.RepoRoot, bundlePath))
	}

	if input.Prepared.DetachPlan.RemoveRootAgentsBlock {
		if err := RemoveBundleAgentsPayloadForRecord(input.RepoRoot, input.Prepared.DetachPlan.ExistingRecord); err != nil {
			return rollbackOnError(fmt.Errorf("remove bundle AGENTS block: %w", err))
		}
		removedPaths = append(removedPaths, rootAgentsPath)
	}

	runtimeFile := input.Prepared.Runtime
	runtimeFile.Members[input.Prepared.TargetIndex].Source = input.NextSource
	runtimeFile.Members[input.Prepared.TargetIndex].OwnerHarnessID = ""
	runtimeFile.Members[input.Prepared.TargetIndex].LastStandaloneOrigin = cloneTemplateSource(input.NextOrigin)
	runtimeFile.Harness.UpdatedAt = resolveMutationTime(input.MutationTime)

	manifestPath, err := WriteManifestFile(input.RepoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return rollbackOnError(fmt.Errorf("write harness manifest: %w", err))
	}
	writtenPaths = append(writtenPaths, mustRepoRelativePath(input.RepoRoot, manifestPath))

	installRecordPath := ""
	if input.InstallRecord != nil {
		writtenInstallPath, err := WriteInstallRecord(input.RepoRoot, *input.InstallRecord)
		if err != nil {
			return rollbackOnError(fmt.Errorf("write harness install record: %w", err))
		}
		installRecordPath = writtenInstallPath
		writtenPaths = append(writtenPaths, mustRepoRelativePath(input.RepoRoot, writtenInstallPath))
	}

	sort.Strings(writtenPaths)
	sort.Strings(removedPaths)
	tx.Commit()

	return bundleMemberExtractMutationResult{
		ManifestPath:        manifestPath,
		InstallRecordPath:   installRecordPath,
		Runtime:             runtimeFile,
		WrittenPaths:        writtenPaths,
		RemovedPaths:        removedPaths,
		RemovedAgentsBlock:  input.Prepared.DetachPlan.RemoveRootAgentsBlock,
		DeletedBundleRecord: input.Prepared.DetachPlan.DeleteBundleRecord,
	}, nil
}

func buildBundleMemberDetachPlan(
	ctx context.Context,
	repoRoot string,
	record BundleRecord,
	orbitID string,
) (bundleMemberDetachPlan, error) {
	if err := ValidateBundleRecord(record); err != nil {
		return bundleMemberDetachPlan{}, fmt.Errorf("validate bundle record: %w", err)
	}

	source, err := resolveInstalledBundleSource(ctx, repoRoot, record)
	if err != nil {
		return bundleMemberDetachPlan{}, fmt.Errorf("resolve installed bundle source: %w", err)
	}

	recordMembers := make(map[string]struct{}, len(record.MemberIDs))
	for _, memberID := range record.MemberIDs {
		recordMembers[memberID] = struct{}{}
	}
	if _, ok := recordMembers[orbitID]; !ok {
		return bundleMemberDetachPlan{}, fmt.Errorf("bundle %q does not own member %q", record.HarnessID, orbitID)
	}

	ownership, err := AnalyzeTemplateMemberOwnership(source, orbitID)
	if err != nil {
		return bundleMemberDetachPlan{}, fmt.Errorf("analyze bundle member ownership for %q: %w", orbitID, err)
	}
	if len(ownership.SharedPaths) > 0 {
		return bundleMemberDetachPlan{}, fmt.Errorf(
			"bundle-backed member %q cannot be detached because shared payload path %q would become ambiguous",
			orbitID,
			ownership.SharedPaths[0],
		)
	}

	surviving := make([]string, 0, len(record.MemberIDs)-1)
	for _, memberID := range record.MemberIDs {
		if memberID == orbitID {
			continue
		}
		surviving = append(surviving, memberID)
	}

	plan := bundleMemberDetachPlan{
		ExistingRecord: record,
	}
	if len(surviving) == 0 {
		if record.IncludesRootAgents {
			if err := verifyBundleAgentsBlockOwnership(repoRoot, record); err != nil {
				return bundleMemberDetachPlan{}, err
			}
			plan.RemoveRootAgentsBlock = true
		}
		plan.DeleteBundleRecord = true
		return plan, nil
	}

	if record.IncludesRootAgents {
		return bundleMemberDetachPlan{}, fmt.Errorf(
			"bundle-backed member %q cannot be detached while owner harness %q still owns root AGENTS.md for surviving bundle members",
			orbitID,
			record.HarnessID,
		)
	}

	extractedOwnedPaths := make(map[string]struct{}, len(ownership.ExclusivePaths)+1)
	definitionPath, err := orbitpkg.HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return bundleMemberDetachPlan{}, fmt.Errorf("build hosted definition path for %q: %w", orbitID, err)
	}
	extractedOwnedPaths[definitionPath] = struct{}{}
	for _, path := range ownership.ExclusivePaths {
		extractedOwnedPaths[path] = struct{}{}
	}

	nextRecord := shrinkBundleRecord(record, source, surviving, extractedOwnedPaths)
	plan.NextRecord = &nextRecord
	return plan, nil
}
