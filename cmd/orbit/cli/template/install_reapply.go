package orbittemplate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// InstalledTemplateBindingsApplyPreviewInput captures the contract for re-rendering one install-backed orbit
// from its recorded source pin and the current runtime vars file.
type InstalledTemplateBindingsApplyPreviewInput struct {
	RepoRoot               string
	OrbitID                string
	RuntimeInstallOrbitIDs []string
	Now                    time.Time
	Progress               func(string) error
}

// InstalledTemplateBindingsApplyPreview captures the current-vars render preview plus changed paths and drift findings.
type InstalledTemplateBindingsApplyPreview struct {
	Preview                          TemplateApplyPreview
	PreviousRenderedSharedAgentsFile *CandidateFile
	ChangedPaths                     []string
	DriftFindings                    []InstallDriftFinding
}

// InstalledTemplateBindingsApplyInput captures the real write path for one install-backed orbit reapply.
type InstalledTemplateBindingsApplyInput struct {
	Preview InstalledTemplateBindingsApplyPreviewInput
	Force   bool
}

// InstalledTemplateBindingsApplyResult returns the preview plus the runtime paths actually rewritten.
type InstalledTemplateBindingsApplyResult struct {
	Preview      InstalledTemplateBindingsApplyPreview
	WrittenPaths []string
}

// BuildInstalledTemplateBindingsApplyPreview re-renders one install-backed orbit from current runtime vars
// without mutating the runtime repository.
func BuildInstalledTemplateBindingsApplyPreview(
	ctx context.Context,
	input InstalledTemplateBindingsApplyPreviewInput,
) (InstalledTemplateBindingsApplyPreview, error) {
	record, err := loadRuntimeInstallRecord(input.RepoRoot, input.OrbitID)
	if err != nil {
		return InstalledTemplateBindingsApplyPreview{}, fmt.Errorf("load install record: %w", err)
	}
	repoVarsFile, _, err := loadOptionalRepoVarsFile(ctx, input.RepoRoot)
	if err != nil {
		return InstalledTemplateBindingsApplyPreview{}, fmt.Errorf("load runtime vars: %w", err)
	}

	if err := bindingsApplyStage(input.Progress, "replaying install source"); err != nil {
		return InstalledTemplateBindingsApplyPreview{}, err
	}
	source, err := resolveInstalledTemplateSource(ctx, input.RepoRoot, record)
	if err != nil {
		return InstalledTemplateBindingsApplyPreview{}, fmt.Errorf("resolve installed template source: %w", err)
	}
	previousPreview, previousOK := replayInstalledTemplateBestEffort(ctx, input.RepoRoot, record)
	var previousSharedAgentsFile *CandidateFile
	if previousOK {
		previousSharedAgentsFile = previousPreview.RenderedSharedAgentsFile
	}

	preview, err := buildInstalledTemplateBindingsPreviewFromRecord(
		ctx,
		input.RepoRoot,
		record,
		source,
		repoVarsFile,
		input.RuntimeInstallOrbitIDs,
		input.Now,
	)
	if err != nil {
		return InstalledTemplateBindingsApplyPreview{}, err
	}

	if err := bindingsApplyStage(input.Progress, "analyzing drift"); err != nil {
		return InstalledTemplateBindingsApplyPreview{}, err
	}
	driftFindings, err := AnalyzeInstalledTemplateDrift(ctx, input.RepoRoot, input.OrbitID)
	if err != nil {
		return InstalledTemplateBindingsApplyPreview{}, fmt.Errorf("analyze installed template drift: %w", err)
	}

	changedPaths, err := analyzeInstalledTemplateBindingsChangedPaths(input.RepoRoot, preview)
	if err != nil {
		return InstalledTemplateBindingsApplyPreview{}, err
	}

	return InstalledTemplateBindingsApplyPreview{
		Preview:                          preview,
		PreviousRenderedSharedAgentsFile: previousSharedAgentsFile,
		ChangedPaths:                     changedPaths,
		DriftFindings:                    driftFindings,
	}, nil
}

func bindingsApplyStage(progress func(string) error, stage string) error {
	if progress == nil {
		return nil
	}

	return progress(stage)
}

// ApplyInstalledTemplateBindings rewrites one install-backed orbit from current runtime vars.
func ApplyInstalledTemplateBindings(
	ctx context.Context,
	input InstalledTemplateBindingsApplyInput,
) (InstalledTemplateBindingsApplyResult, error) {
	preview, err := BuildInstalledTemplateBindingsApplyPreview(ctx, input.Preview)
	if err != nil {
		return InstalledTemplateBindingsApplyResult{}, err
	}
	if len(preview.DriftFindings) > 0 && !input.Force {
		return InstalledTemplateBindingsApplyResult{}, fmt.Errorf(
			"current runtime has drift; re-run with --force to rewrite install-owned paths",
		)
	}

	writtenPaths, err := applyInstalledTemplateBindingsPreview(input.Preview.RepoRoot, preview.Preview, preview.ChangedPaths, preview.PreviousRenderedSharedAgentsFile)
	if err != nil {
		return InstalledTemplateBindingsApplyResult{}, err
	}

	return InstalledTemplateBindingsApplyResult{
		Preview:      preview,
		WrittenPaths: writtenPaths,
	}, nil
}

// WriteInstalledTemplateBindingsApplyPreview rewrites one install-backed orbit from an already-built preview.
func WriteInstalledTemplateBindingsApplyPreview(
	repoRoot string,
	preview InstalledTemplateBindingsApplyPreview,
) ([]string, error) {
	changedPaths := append([]string(nil), preview.ChangedPaths...)

	installPath, err := runtimeInstallRecordPath(repoRoot, preview.Preview.Source.Manifest.Template.OrbitID)
	if err != nil {
		return nil, fmt.Errorf("build install record path: %w", err)
	}
	installRepoPath := mustRepoRelativePath(repoRoot, installPath)
	if !slices.Contains(changedPaths, installRepoPath) {
		changedPaths = append(changedPaths, installRepoPath)
	}

	return applyInstalledTemplateBindingsPreview(repoRoot, preview.Preview, changedPaths, preview.PreviousRenderedSharedAgentsFile)
}

func buildInstalledTemplateBindingsPreviewFromRecord(
	ctx context.Context,
	repoRoot string,
	record InstallRecord,
	source LocalTemplateSource,
	repoVarsFile bindings.VarsFile,
	runtimeInstallOrbitIDs []string,
	now time.Time,
) (TemplateApplyPreview, error) {
	declared := make(map[string]bindings.VariableDeclaration, len(source.Manifest.Variables))
	for name, spec := range source.Manifest.Variables {
		declared[name] = bindings.VariableDeclaration{
			Description: spec.Description,
			Required:    spec.Required,
		}
	}
	runtimeNamespaces, err := resolveRuntimeInstallVariableNamespaces(repoRoot, record.OrbitID, declared, runtimeInstallOrbitIDs)
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("resolve runtime variable namespaces: %w", err)
	}
	var recordNamespaces map[string]string
	if record.Variables != nil {
		recordNamespaces = record.Variables.Namespaces
	}

	mergeResult, err := resolveTemplateBindings(
		ctx,
		declared,
		map[string]bindings.VariableBinding{},
		nil,
		repoVarsFile.Variables,
		bindings.ScopedVariablesForNamespace(repoVarsFile, record.OrbitID),
		record.OrbitID,
		mergeVariableNamespaces(runtimeNamespaces, recordNamespaces),
		false,
		nil,
		false,
		nil,
	)
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("merge bindings: %w", err)
	}

	renderValues := make(map[string]string, len(mergeResult.Resolved))
	for name, resolved := range mergeResult.Resolved {
		renderValues[name] = resolved.Value
	}

	renderedFiles, err := renderTemplateFilesWithOptions(source.Files, renderValues, renderTemplateOptions{
		AllowUnresolved: true,
	})
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("render template files: %w", err)
	}

	renderedSharedAgentsFile, hasSharedAgents, err := renderSharedAgentsPayloadWithOptions(source, renderValues, renderTemplateOptions{
		AllowUnresolved: true,
	})
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("render shared AGENTS payload: %w", err)
	}

	var renderedSharedAgentsFilePtr *CandidateFile
	if hasSharedAgents {
		renderedSharedAgentsFilePtr = &renderedSharedAgentsFile
	}

	installRecord := InstallRecord{
		SchemaVersion: installRecordSchemaVersion,
		OrbitID:       record.OrbitID,
		Template:      record.Template,
		AppliedAt:     resolveApplyTime(now),
		Variables:     BuildInstallVariablesSnapshot(declared, mergeResult),
	}
	agentAddons, err := BuildAgentAddonsSnapshot(source.Spec, renderedFiles)
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("snapshot agent add-ons: %w", err)
	}
	if len(agentAddons.Hooks) > 0 {
		installRecord.AgentAddons = agentAddons
	}
	if installRecord.Variables != nil {
		installRecord.Variables.ObservedRuntimeUnresolved = collectObservedRuntimeUnresolved(
			renderedFiles,
			renderedSharedAgentsFilePtr,
		)
	}

	warnings := make([]string, 0, 1)
	if unresolvedNames := unresolvedBindingNames(mergeResult.Unresolved); len(unresolvedNames) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"bindings apply kept template variables unresolved: %s",
			strings.Join(unresolvedNames, ", "),
		))
	}

	return TemplateApplyPreview{
		Source:                   source,
		ResolvedBindings:         mergeResult.Resolved,
		RenderedFiles:            renderedFiles,
		RenderedSharedAgentsFile: renderedSharedAgentsFilePtr,
		InstallRecord:            installRecord,
		Warnings:                 warnings,
	}, nil
}

func collectObservedRuntimeUnresolved(renderedFiles []CandidateFile, sharedAgents *CandidateFile) []string {
	observed := make(map[string]struct{})
	for _, file := range renderedFiles {
		result := ScanVariables([]CandidateFile{file}, nil)
		for _, name := range result.Referenced {
			observed[name] = struct{}{}
		}
	}
	if sharedAgents != nil {
		result := ScanVariables([]CandidateFile{{
			Path:    sharedFilePathAgents,
			Content: sharedAgents.Content,
		}}, nil)
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

func analyzeInstalledTemplateBindingsChangedPaths(repoRoot string, preview TemplateApplyPreview) ([]string, error) {
	changedPaths := make([]string, 0, len(preview.RenderedFiles)+3)

	definitionPath, err := orbitHostedDefinitionPathForPreview(repoRoot, preview)
	if err != nil {
		return nil, err
	}
	plannedDefinitionData, err := plannedRuntimeCompanionWrite(preview.Source)
	if err != nil {
		return nil, fmt.Errorf("build runtime definition plan: %w", err)
	}
	if definitionChanged, err := runtimeFileHasDrift(definitionPath, plannedDefinitionData); err != nil {
		return nil, fmt.Errorf("check definition drift: %w", err)
	} else if definitionChanged {
		changedPaths = append(changedPaths, mustRepoRelativePath(repoRoot, definitionPath))
	}

	for _, file := range preview.RenderedFiles {
		filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		changed, err := runtimeFileHasDrift(filename, file.Content)
		if err != nil {
			return nil, fmt.Errorf("check runtime file drift for %s: %w", file.Path, err)
		}
		if changed {
			changedPaths = append(changedPaths, file.Path)
		}
	}

	agentsChanged, err := runtimeAgentsHasDrift(repoRoot, preview.Source.Manifest.Template.OrbitID, preview.RenderedSharedAgentsFile)
	if err != nil {
		return nil, fmt.Errorf("check runtime AGENTS drift: %w", err)
	}
	if agentsChanged {
		changedPaths = append(changedPaths, sharedFilePathAgents)
	}

	installPath, err := runtimeInstallRecordPath(repoRoot, preview.Source.Manifest.Template.OrbitID)
	if err != nil {
		return nil, fmt.Errorf("build install record path: %w", err)
	}
	installData, err := contractutil.EncodeYAMLDocument(installRecordNode(preview.InstallRecord))
	if err != nil {
		return nil, fmt.Errorf("encode install record plan: %w", err)
	}
	if installChanged, err := runtimeFileHasDrift(installPath, installData); err != nil {
		return nil, fmt.Errorf("check install record drift: %w", err)
	} else if installChanged {
		changedPaths = append(changedPaths, mustRepoRelativePath(repoRoot, installPath))
	}

	sort.Strings(changedPaths)

	return changedPaths, nil
}

func applyInstalledTemplateBindingsPreview(
	repoRoot string,
	preview TemplateApplyPreview,
	changedPaths []string,
	previousSharedAgentsFile *CandidateFile,
) ([]string, error) {
	changedSet := make(map[string]struct{}, len(changedPaths))
	for _, path := range changedPaths {
		changedSet[path] = struct{}{}
	}

	writtenPaths := make([]string, 0, len(changedPaths))
	for _, file := range preview.RenderedFiles {
		if _, ok := changedSet[file.Path]; !ok {
			continue
		}
		filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		perm, err := gitpkg.FilePermForMode(file.Mode)
		if err != nil {
			return nil, fmt.Errorf("resolve rendered file mode %s: %w", file.Path, err)
		}
		if err := contractutil.AtomicWriteFileMode(filename, file.Content, perm); err != nil {
			return nil, fmt.Errorf("write rendered file %s: %w", file.Path, err)
		}
		writtenPaths = append(writtenPaths, file.Path)
	}

	if _, ok := changedSet[sharedFilePathAgents]; ok {
		if preview.RenderedSharedAgentsFile != nil {
			if err := applySharedAgentsPayloadReplacingRunViewPayload(
				repoRoot,
				preview.Source.Manifest.Template.OrbitID,
				previousSharedAgentsFile,
				preview.RenderedSharedAgentsFile.Content,
			); err != nil {
				return nil, fmt.Errorf("write runtime AGENTS.md: %w", err)
			}
		} else {
			if err := removeRuntimeAgentsBlock(repoRoot, preview.Source.Manifest.Template.OrbitID); err != nil {
				return nil, fmt.Errorf("remove runtime AGENTS.md block: %w", err)
			}
		}
		writtenPaths = append(writtenPaths, sharedFilePathAgents)
	}

	definitionPath, err := orbitHostedDefinitionPathForPreview(repoRoot, preview)
	if err != nil {
		return nil, err
	}
	definitionRepoPath := mustRepoRelativePath(repoRoot, definitionPath)
	if _, ok := changedSet[definitionRepoPath]; ok {
		if _, err := writeRuntimeCompanionDefinition(repoRoot, preview.Source); err != nil {
			return nil, fmt.Errorf("write orbit definition: %w", err)
		}
		writtenPaths = append(writtenPaths, definitionRepoPath)
	}

	installPath, err := runtimeInstallRecordPath(repoRoot, preview.Source.Manifest.Template.OrbitID)
	if err != nil {
		return nil, fmt.Errorf("build install record path: %w", err)
	}
	installRepoPath := mustRepoRelativePath(repoRoot, installPath)
	if _, ok := changedSet[installRepoPath]; ok {
		if _, err := writeRuntimeInstallRecord(repoRoot, preview.InstallRecord); err != nil {
			return nil, fmt.Errorf("write install record: %w", err)
		}
		writtenPaths = append(writtenPaths, installRepoPath)
	}

	sort.Strings(writtenPaths)

	return writtenPaths, nil
}

func applySharedAgentsPayloadReplacingRunViewPayload(repoRoot string, orbitID string, previous *CandidateFile, next []byte) error {
	if previous == nil {
		return applySharedAgentsPayload(repoRoot, orbitID, next)
	}

	filename := filepath.Join(repoRoot, filepath.FromSlash(sharedFilePathAgents))
	//nolint:gosec // The runtime AGENTS path is fixed under the repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return applySharedAgentsPayload(repoRoot, orbitID, next)
		}
		return fmt.Errorf("read runtime AGENTS.md: %w", err)
	}
	document, err := ParseRuntimeAgentsDocument(data)
	if err != nil {
		return fmt.Errorf("parse runtime AGENTS.md: %w", err)
	}
	if _, found := runtimeAgentsBlockContent(document, orbitID); found || !runtimeAgentsDocumentHasNoBlocks(document) {
		return applySharedAgentsPayload(repoRoot, orbitID, next)
	}

	previousPayload := normalizeRuntimeAgentsPayload(previous.Content)
	if len(bytes.TrimSpace(previousPayload)) == 0 {
		return applySharedAgentsPayload(repoRoot, orbitID, next)
	}
	normalizedData := normalizeRuntimeAgentsPayload(data)
	if !bytes.Contains(normalizedData, previousPayload) {
		return applySharedAgentsPayload(repoRoot, orbitID, next)
	}

	fileInfo, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("stat runtime AGENTS.md: %w", err)
	}
	updated := bytes.Replace(normalizedData, previousPayload, normalizeRuntimeAgentsPayload(next), 1)
	if err := contractutil.AtomicWriteFileMode(filename, updated, fileInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("write runtime AGENTS.md: %w", err)
	}
	return nil
}

func orbitHostedDefinitionPathForPreview(repoRoot string, preview TemplateApplyPreview) (string, error) {
	path, err := orbit.HostedDefinitionPath(repoRoot, preview.Source.Manifest.Template.OrbitID)
	if err != nil {
		return "", fmt.Errorf("build runtime definition path: %w", err)
	}

	return path, nil
}
