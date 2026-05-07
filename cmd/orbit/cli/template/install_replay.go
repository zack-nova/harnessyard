package orbittemplate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// InstalledTemplateReplayInput captures the minimal contract needed to reconstruct one install-backed payload.
type InstalledTemplateReplayInput struct {
	RepoRoot string
	Record   InstallRecord
}

// InstallDriftKind is one schema-backed drift classification primitive for install-backed members.
type InstallDriftKind string

const (
	DriftKindDefinition             InstallDriftKind = "definition_drift"
	DriftKindRuntimeFile            InstallDriftKind = "runtime_file_drift"
	DriftKindProvenanceUnresolvable InstallDriftKind = "provenance_unresolvable"
)

// InstallDriftFinding is one stable drift finding for later harness check integration.
type InstallDriftFinding struct {
	Kind InstallDriftKind
	Path string
}

// InstallOwnedCleanupPlan captures the stale install-owned paths that may be removed after a successful overwrite.
type InstallOwnedCleanupPlan struct {
	DeletePaths            []string
	RemoveSharedAgentsFile bool
}

// ReplayInstalledTemplate reconstructs the install-backed template payload using the recorded source pin
// plus the current runtime bindings context from the repository.
func ReplayInstalledTemplate(ctx context.Context, input InstalledTemplateReplayInput) (TemplateApplyPreview, error) {
	if err := ValidateInstallRecord(input.Record); err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("validate install record: %w", err)
	}

	source, err := resolveInstalledTemplateSource(ctx, input.RepoRoot, input.Record)
	if err != nil {
		return TemplateApplyPreview{}, err
	}
	if input.Record.Variables != nil {
		return replayInstalledTemplateFromSnapshot(source, input.Record)
	}

	preview, err := buildTemplateApplyPreviewFromSource(
		ctx,
		input.RepoRoot,
		source,
		input.Record.Template,
		"",
		nil,
		nil,
		false,
		false,
		false,
		nil,
		false,
		nil,
		input.Record.AppliedAt,
	)
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("replay installed template: %w", err)
	}

	return preview, nil
}

func replayInstalledTemplateFromSnapshot(source LocalTemplateSource, record InstallRecord) (TemplateApplyPreview, error) {
	renderValues := make(map[string]string, len(record.Variables.ResolvedAtApply))
	resolvedBindings := make(map[string]bindings.ResolvedBinding, len(record.Variables.ResolvedAtApply))
	for name, binding := range record.Variables.ResolvedAtApply {
		renderValues[name] = binding.Value
		resolvedBindings[name] = bindings.ResolvedBinding{
			Value:       binding.Value,
			Description: binding.Description,
			Required:    record.Variables.Declarations[name].Required,
			Namespace:   record.Variables.Namespaces[name],
		}
	}

	allowUnresolved := len(record.Variables.UnresolvedAtApply) > 0
	renderedFiles, err := renderTemplateFilesWithOptions(source.Files, renderValues, renderTemplateOptions{
		AllowUnresolved: allowUnresolved,
	})
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("render replayed template files: %w", err)
	}
	renderedSharedAgentsFile, hasSharedAgents, err := renderSharedAgentsPayloadWithOptions(source, renderValues, renderTemplateOptions{
		AllowUnresolved: allowUnresolved,
	})
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("render replayed shared AGENTS payload: %w", err)
	}

	var renderedSharedAgentsFilePtr *CandidateFile
	if hasSharedAgents {
		renderedSharedAgentsFilePtr = &renderedSharedAgentsFile
	}

	preview := TemplateApplyPreview{
		Source:                   source,
		ResolvedBindings:         resolvedBindings,
		RenderedFiles:            renderedFiles,
		RenderedSharedAgentsFile: renderedSharedAgentsFilePtr,
		InstallRecord:            record,
	}
	if allowUnresolved {
		preview.Warnings = []string{
			fmt.Sprintf("install kept template variables unresolved: %s", strings.Join(record.Variables.UnresolvedAtApply, ", ")),
		}
	}

	return preview, nil
}

// BuildInstallOwnedCleanupPlan reconstructs the current install-owned payload and identifies stale runtime paths
// that are safe to remove after a successful overwrite.
func BuildInstallOwnedCleanupPlan(
	ctx context.Context,
	repoRoot string,
	existingRecord InstallRecord,
	nextPreview TemplateApplyPreview,
) (InstallOwnedCleanupPlan, error) {
	var err error
	existingRecord, err = recordWithDestructiveReplayVariablesSnapshot(ctx, repoRoot, existingRecord)
	if err != nil {
		return InstallOwnedCleanupPlan{}, err
	}

	previousPreview, err := ReplayInstalledTemplate(ctx, InstalledTemplateReplayInput{
		RepoRoot: repoRoot,
		Record:   existingRecord,
	})
	if err != nil {
		return InstallOwnedCleanupPlan{}, fmt.Errorf("replay existing install: %w", err)
	}

	nextOwnedPaths := make(map[string]struct{}, len(nextPreview.RenderedFiles))
	for _, file := range nextPreview.RenderedFiles {
		nextOwnedPaths[file.Path] = struct{}{}
	}

	plan := InstallOwnedCleanupPlan{
		DeletePaths: make([]string, 0),
	}
	for _, oldFile := range previousPreview.RenderedFiles {
		if _, stillOwned := nextOwnedPaths[oldFile.Path]; stillOwned {
			continue
		}
		if err := ensureStaleOwnedFileMatches(repoRoot, oldFile); err != nil {
			return InstallOwnedCleanupPlan{}, err
		}
		plan.DeletePaths = append(plan.DeletePaths, oldFile.Path)
	}

	if previousPreview.RenderedSharedAgentsFile != nil && nextPreview.RenderedSharedAgentsFile == nil {
		if err := ensureRuntimeAgentsBlockMatches(repoRoot, existingRecord.OrbitID, previousPreview.RenderedSharedAgentsFile.Content); err != nil {
			return InstallOwnedCleanupPlan{}, err
		}
		plan.RemoveSharedAgentsFile = true
	}

	sort.Strings(plan.DeletePaths)

	return plan, nil
}

// ApplyInstallOwnedCleanup removes the stale paths previously validated by BuildInstallOwnedCleanupPlan.
func ApplyInstallOwnedCleanup(repoRoot string, orbitID string, plan InstallOwnedCleanupPlan) ([]string, error) {
	removed := make([]string, 0, len(plan.DeletePaths)+1)
	for _, path := range plan.DeletePaths {
		filename := filepath.Join(repoRoot, filepath.FromSlash(path))
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale install-owned path %s: %w", path, err)
		}
		removed = append(removed, path)
	}

	if plan.RemoveSharedAgentsFile {
		if err := removeRuntimeAgentsBlock(repoRoot, orbitID); err != nil {
			return nil, fmt.Errorf("remove stale runtime AGENTS block: %w", err)
		}
		removed = append(removed, sharedFilePathAgents)
	}

	sort.Strings(removed)

	return removed, nil
}

// AnalyzeInstalledTemplateDrift compares the current runtime materialization with the replayed expected output.
func AnalyzeInstalledTemplateDrift(ctx context.Context, repoRoot string, orbitID string) ([]InstallDriftFinding, error) {
	record, err := loadRuntimeInstallRecord(repoRoot, orbitID)
	if err != nil {
		return nil, fmt.Errorf("load runtime install record: %w", err)
	}

	replay, ok := replayInstalledTemplateBestEffort(ctx, repoRoot, record)
	if !ok {
		return []InstallDriftFinding{
			{
				Kind: DriftKindProvenanceUnresolvable,
				Path: fmt.Sprintf(".harness/installs/%s.yaml", orbitID),
			},
		}, nil
	}

	findings := make([]InstallDriftFinding, 0)

	definitionPath, err := orbit.HostedDefinitionPath(repoRoot, orbitID)
	if err != nil {
		return nil, fmt.Errorf("build runtime definition path: %w", err)
	}
	plannedDefinitionData, err := plannedRuntimeCompanionWrite(replay.Source)
	if err != nil {
		return nil, fmt.Errorf("build runtime definition replay: %w", err)
	}
	if definitionDrift, err := runtimeFileHasDrift(definitionPath, plannedDefinitionData); err != nil {
		return nil, fmt.Errorf("check definition drift: %w", err)
	} else if definitionDrift {
		findings = append(findings, InstallDriftFinding{
			Kind: DriftKindDefinition,
			Path: mustRepoRelativePath(repoRoot, definitionPath),
		})
	}

	for _, file := range replay.RenderedFiles {
		filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		drifted, err := runtimeFileHasDrift(filename, file.Content)
		if err != nil {
			return nil, fmt.Errorf("check runtime drift for %s: %w", file.Path, err)
		}
		if drifted {
			findings = append(findings, InstallDriftFinding{
				Kind: DriftKindRuntimeFile,
				Path: file.Path,
			})
		}
	}

	agentsDrift, err := runtimeAgentsHasDrift(repoRoot, orbitID, replay.RenderedSharedAgentsFile, runtimeAgentsDriftOptions{
		AllowMarkerlessPresentation: true,
	})
	if err != nil || agentsDrift {
		findings = append(findings, InstallDriftFinding{
			Kind: DriftKindRuntimeFile,
			Path: sharedFilePathAgents,
		})
	}

	sort.Slice(findings, func(left, right int) bool {
		if findings[left].Path == findings[right].Path {
			return findings[left].Kind < findings[right].Kind
		}
		return findings[left].Path < findings[right].Path
	})

	return findings, nil
}

func resolveInstalledTemplateSource(ctx context.Context, repoRoot string, record InstallRecord) (LocalTemplateSource, error) {
	switch normalizeInstallSourceKind(record.Template.SourceKind) {
	case InstallSourceKindLocalBranch:
		exists, err := gitpkg.RevisionExists(ctx, repoRoot, record.Template.TemplateCommit)
		if err != nil {
			return LocalTemplateSource{}, fmt.Errorf("check recorded local template commit %q: %w", record.Template.TemplateCommit, err)
		}
		if !exists {
			return LocalTemplateSource{}, fmt.Errorf("recorded local template commit %q is not available", record.Template.TemplateCommit)
		}

		source, err := loadTemplateSourceAtRevision(ctx, repoRoot, record.Template.TemplateCommit, record.Template.SourceRef, record.Template.TemplateCommit)
		if err != nil {
			return LocalTemplateSource{}, fmt.Errorf("resolve recorded local template source %q: %w", record.Template.SourceRef, err)
		}
		return source, nil
	case InstallSourceKindExternalGit:
		remoteRef := normalizeRecordedRemoteTemplateRef(record.Template.SourceRef)

		var source LocalTemplateSource
		if err := gitpkg.WithFetchedRemoteRevisionOrRef(
			ctx,
			repoRoot,
			record.Template.SourceRepo,
			record.Template.TemplateCommit,
			remoteRef,
			func(_ string) error {
				resolved, err := loadTemplateSourceAtRevision(ctx, repoRoot, record.Template.TemplateCommit, record.Template.SourceRef, record.Template.TemplateCommit)
				if err != nil {
					return err
				}
				source = resolved
				return nil
			},
		); err != nil {
			return LocalTemplateSource{}, fmt.Errorf("resolve recorded external template source %q from %q: %w", record.Template.SourceRef, record.Template.SourceRepo, err)
		}

		return source, nil
	default:
		return LocalTemplateSource{}, fmt.Errorf("unsupported install source kind %q", record.Template.SourceKind)
	}
}

func replayInstalledTemplateBestEffort(ctx context.Context, repoRoot string, record InstallRecord) (TemplateApplyPreview, bool) {
	replay, err := ReplayInstalledTemplate(ctx, InstalledTemplateReplayInput{
		RepoRoot: repoRoot,
		Record:   record,
	})
	if err != nil {
		return TemplateApplyPreview{}, false
	}

	return replay, true
}

func recordWithDestructiveReplayVariablesSnapshot(
	ctx context.Context,
	repoRoot string,
	record InstallRecord,
) (InstallRecord, error) {
	if record.Variables != nil {
		return record, nil
	}

	source, err := resolveInstalledTemplateSource(ctx, repoRoot, record)
	if err != nil {
		return InstallRecord{}, err
	}
	if len(source.Manifest.Variables) > 0 {
		return InstallRecord{}, missingVariablesSnapshotForOverwriteReplayError(record)
	}

	record.Variables = &InstallVariablesSnapshot{}
	return record, nil
}

func missingVariablesSnapshotForOverwriteReplayError(record InstallRecord) error {
	return fmt.Errorf(
		"install record for orbit %q does not contain variables snapshot required for overwrite replay",
		record.OrbitID,
	)
}

func normalizeRecordedRemoteTemplateRef(ref string) string {
	trimmed := strings.TrimSpace(ref)
	if strings.HasPrefix(trimmed, "refs/") {
		return trimmed
	}
	return "refs/heads/" + trimmed
}

func ensureStaleOwnedFileMatches(repoRoot string, file CandidateFile) error {
	filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
	//nolint:gosec // The candidate file path is repo-relative and built from validated template content.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read stale install-owned path %s: %w", file.Path, err)
	}
	if !bytes.Equal(data, file.Content) {
		return fmt.Errorf("stale install-owned path %s no longer matches reconstructed content", file.Path)
	}

	return nil
}

func ensureRuntimeAgentsBlockMatches(repoRoot string, orbitID string, payload []byte) error {
	filename := filepath.Join(repoRoot, sharedFilePathAgents)
	//nolint:gosec // The AGENTS path is fixed under the repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	document, err := ParseRuntimeAgentsDocument(data)
	if err != nil {
		return fmt.Errorf("parse runtime AGENTS.md: %w", err)
	}
	actual, found := runtimeAgentsBlockContent(document, orbitID)
	if !found {
		return nil
	}
	if !bytes.Equal(actual, normalizeRuntimeAgentsPayload(payload)) {
		return fmt.Errorf("runtime AGENTS.md block %q no longer matches reconstructed content", orbitID)
	}

	return nil
}

type runtimeAgentsDriftOptions struct {
	AllowMarkerlessPresentation bool
}

func runtimeAgentsHasDrift(repoRoot string, orbitID string, expected *CandidateFile, options runtimeAgentsDriftOptions) (bool, error) {
	filename := filepath.Join(repoRoot, sharedFilePathAgents)
	//nolint:gosec // The AGENTS path is fixed under the repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return expected != nil, nil
		}
		return false, fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	document, err := ParseRuntimeAgentsDocument(data)
	if err != nil {
		return true, fmt.Errorf("parse runtime AGENTS.md: %w", err)
	}
	actual, found := runtimeAgentsBlockContent(document, orbitID)

	if expected == nil {
		return found, nil
	}
	if !found {
		if options.AllowMarkerlessPresentation && runtimeAgentsDocumentHasMarkerlessPresentationContent(document) {
			return false, nil
		}
		if runtimeAgentsDocumentContainsRunViewPayload(document, data, expected.Content) {
			return false, nil
		}
		return true, nil
	}

	return !bytes.Equal(actual, normalizeRuntimeAgentsPayload(expected.Content)), nil
}

func runtimeAgentsBlockContent(document AgentsRuntimeDocument, orbitID string) ([]byte, bool) {
	for _, segment := range document.Segments {
		if segment.Kind == AgentsRuntimeSegmentBlock && segment.OwnerKind == OwnerKindOrbit && segment.WorkflowID == orbitID {
			return append([]byte(nil), segment.Content...), true
		}
	}

	return nil, false
}

func normalizeRuntimeAgentsPayload(payload []byte) []byte {
	normalized := append([]byte(nil), payload...)
	if len(normalized) > 0 && normalized[len(normalized)-1] != '\n' {
		normalized = append(normalized, '\n')
	}

	return normalized
}

func runtimeFileHasDrift(filename string, expected []byte) (bool, error) {
	//nolint:gosec // The filename is a repo-local runtime path built by the caller.
	actual, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read %s: %w", filename, err)
	}

	return !bytes.Equal(actual, expected), nil
}
