package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"gopkg.in/yaml.v3"
)

const (
	runtimeVarsRepoRelativePath     = ".harness/vars.yaml"
	runtimeInstallRecordRelativeDir = ".harness/installs"
	runtimeManifestRelativePath     = ".harness/manifest.yaml"
)

// LocalTemplateSource is the resolved local template branch payload.
type LocalTemplateSource struct {
	Ref            string
	Commit         string
	Manifest       Manifest
	Spec           orbit.OrbitSpec
	Definition     orbit.Definition
	DefinitionData []byte
	Files          []CandidateFile
}

// TemplateApplyPreviewInput describes the local apply preview path.
type TemplateApplyPreviewInput struct {
	RepoRoot                string
	SourceRef               string
	BindingsFilePath        string
	VariableNamespaces      map[string]string
	RuntimeInstallOrbitIDs  []string
	OverwriteExisting       bool
	SkipSharedAgentsWrite   bool
	AllowUnresolvedBindings bool
	Interactive             bool
	Prompter                BindingPrompter
	EditorMode              bool
	Editor                  Editor
	Now                     time.Time
}

// ApplyConflict summarizes one fail-closed apply conflict.
type ApplyConflict struct {
	Path    string
	Message string
}

// TemplateApplyPreview is the full preview for local template apply.
type TemplateApplyPreview struct {
	Source                   LocalTemplateSource
	RemoteRequestedRef       string
	RemoteResolutionKind     RemoteTemplateResolutionKind
	ResolvedBindings         map[string]bindings.ResolvedBinding
	RenderedFiles            []CandidateFile
	RenderedSharedAgentsFile *CandidateFile
	InstallRecord            InstallRecord
	VarsFile                 *bindings.VarsFile
	Conflicts                []ApplyConflict
	Warnings                 []string
}

// TemplateApplyInput describes the real write path for local template apply.
type TemplateApplyInput struct {
	Preview TemplateApplyPreviewInput
}

// RemoteTemplateApplyPreviewInput describes the remote apply preview path.
type RemoteTemplateApplyPreviewInput struct {
	RepoRoot                string
	RemoteURL               string
	RequestedRef            string
	BindingsFilePath        string
	VariableNamespaces      map[string]string
	RuntimeInstallOrbitIDs  []string
	OverwriteExisting       bool
	SkipSharedAgentsWrite   bool
	AllowUnresolvedBindings bool
	Interactive             bool
	Prompter                BindingPrompter
	EditorMode              bool
	Editor                  Editor
	Now                     time.Time
}

// RemoteTemplateApplyInput describes the real write path for remote template apply.
type RemoteTemplateApplyInput struct {
	Preview RemoteTemplateApplyPreviewInput
}

type templateApplyLocalInputs struct {
	bindingsFile    bindings.VarsFile
	repoVarsFile    bindings.VarsFile
	hasRepoVarsFile bool
}

// TemplateApplyResult returns the preview plus the runtime paths written.
type TemplateApplyResult struct {
	Preview      TemplateApplyPreview
	WrittenPaths []string
}

// ResolveLocalTemplateSource loads one local template branch and validates its schema-backed contract.
func ResolveLocalTemplateSource(ctx context.Context, repoRoot string, ref string) (LocalTemplateSource, error) {
	trimmedRef := strings.TrimSpace(ref)
	if trimmedRef == "" {
		return LocalTemplateSource{}, fmt.Errorf("template source ref must not be empty")
	}

	exists, err := gitpkg.RevisionExists(ctx, repoRoot, trimmedRef)
	if err != nil {
		return LocalTemplateSource{}, fmt.Errorf("check template source %q: %w", trimmedRef, err)
	}
	if !exists {
		return LocalTemplateSource{}, fmt.Errorf("template source %q not found", trimmedRef)
	}

	return loadTemplateSourceAtRevision(ctx, repoRoot, trimmedRef, trimmedRef, "")
}

func loadTemplateSourceAtRevision(ctx context.Context, repoRoot string, revision string, sourceRef string, sourceCommit string) (LocalTemplateSource, error) {
	trimmedRef := strings.TrimSpace(sourceRef)
	if trimmedRef == "" {
		return LocalTemplateSource{}, fmt.Errorf("template source ref must not be empty")
	}

	branchManifestData, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, branchManifestPath)
	if err != nil {
		return LocalTemplateSource{}, fmt.Errorf("template source %q is not a valid orbit template branch: read %s: %w", trimmedRef, branchManifestPath, err)
	}
	branchManifest, err := parseOrbitTemplateBranchManifestData(branchManifestData)
	if err != nil {
		return LocalTemplateSource{}, fmt.Errorf("template source %q is not a valid orbit template branch: parse %s: %w", trimmedRef, branchManifestPath, err)
	}
	if branchManifest.Kind != "orbit_template" {
		return LocalTemplateSource{}, fmt.Errorf("template source %q is not a valid orbit template branch: %s kind must be %q", trimmedRef, branchManifestPath, "orbit_template")
	}
	if strings.TrimSpace(branchManifest.Template.OrbitID) == "" {
		return LocalTemplateSource{}, fmt.Errorf("template source %q is not a valid orbit template branch: %s template.orbit_id must not be empty", trimmedRef, branchManifestPath)
	}

	manifest, err := loadTemplateManifestFromRevision(trimmedRef, branchManifest)
	if err != nil {
		return LocalTemplateSource{}, err
	}

	hostedDefinitionPath, legacyDefinitionPath, err := templateCompanionPaths(manifest.Template.OrbitID)
	if err != nil {
		return LocalTemplateSource{}, fmt.Errorf("build template definition path: %w", err)
	}
	definitionPath, spec, err := loadTemplateCompanionAtRevision(ctx, repoRoot, revision, manifest.Template.OrbitID)
	if err != nil {
		return LocalTemplateSource{}, err
	}
	definitionData, err := stableOrbitSpecData(spec)
	if err != nil {
		return LocalTemplateSource{}, fmt.Errorf("normalize template definition %s from %q: %w", definitionPath, trimmedRef, err)
	}
	definition := spec.LegacyDefinition()

	allPaths, err := gitpkg.ListAllFilesAtRev(ctx, repoRoot, revision)
	if err != nil {
		return LocalTemplateSource{}, fmt.Errorf("list template source files from %q: %w", trimmedRef, err)
	}

	files := make([]CandidateFile, 0, len(allPaths))
	for _, path := range allPaths {
		switch path {
		case manifestRelativePath, branchManifestPath, definitionPath, hostedDefinitionPath, legacyDefinitionPath:
			continue
		}
		if strings.HasPrefix(path, ".git/orbit/state/") {
			return LocalTemplateSource{}, fmt.Errorf("template source %q contains forbidden path %s", trimmedRef, path)
		}
		if strings.HasPrefix(path, ".harness/") {
			return LocalTemplateSource{}, fmt.Errorf("template source %q contains forbidden path %s", trimmedRef, path)
		}
		if strings.HasPrefix(path, ".orbit/") {
			return LocalTemplateSource{}, fmt.Errorf("template source %q contains forbidden path %s", trimmedRef, path)
		}
		if path == sharedFilePathAgents {
			return LocalTemplateSource{}, fmt.Errorf("template source %q contains unsupported legacy AGENTS.md payload", trimmedRef)
		}

		data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, path)
		if err != nil {
			return LocalTemplateSource{}, fmt.Errorf("read template file %s from %q: %w", path, trimmedRef, err)
		}
		mode, err := gitpkg.FileModeAtRev(ctx, repoRoot, revision, path)
		if err != nil {
			return LocalTemplateSource{}, fmt.Errorf("read template file mode %s from %q: %w", path, trimmedRef, err)
		}
		files = append(files, CandidateFile{
			Path:    path,
			Content: data,
			Mode:    mode,
		})
	}

	scanResult := ScanVariables(files, manifest.Variables)
	agentsCandidate, hasAgentsCandidate, err := companionAgentsCandidate([]CandidateFile{{
		Path:    definitionPath,
		Content: definitionData,
		Mode:    gitpkg.FileModeRegular,
	}}, manifest.Template.OrbitID)
	if err != nil {
		return LocalTemplateSource{}, fmt.Errorf("scan template companion agents body from %q: %w", trimmedRef, err)
	}
	if hasAgentsCandidate {
		scanResult = ScanVariables(append(files, agentsCandidate), manifest.Variables)
	}
	if len(scanResult.Undeclared) > 0 {
		return LocalTemplateSource{}, fmt.Errorf("template source %q references undeclared variables: %s", trimmedRef, strings.Join(scanResult.Undeclared, ", "))
	}

	commit := strings.TrimSpace(sourceCommit)
	if commit == "" {
		commit, err = resolveRevisionCommit(ctx, repoRoot, revision)
		if err != nil {
			return LocalTemplateSource{}, fmt.Errorf("resolve template source commit for %q: %w", trimmedRef, err)
		}
	}

	return LocalTemplateSource{
		Ref:            trimmedRef,
		Commit:         commit,
		Manifest:       manifest,
		Spec:           spec,
		Definition:     definition,
		DefinitionData: definitionData,
		Files:          files,
	}, validateTemplateSourceCapabilities(spec, files)
}

func validateTemplateSourceCapabilities(spec orbit.OrbitSpec, files []CandidateFile) error {
	if spec.Capabilities == nil && spec.AgentAddons == nil {
		return nil
	}

	definition, err := orbit.CompatibilityDefinitionFromOrbitSpec(spec)
	if err != nil {
		return fmt.Errorf("build compatibility definition for capability preflight: %w", err)
	}

	trackedFiles := make([]string, 0, len(files))
	fileContents := make(map[string][]byte, len(files))
	for _, file := range files {
		trackedFiles = append(trackedFiles, file.Path)
		fileContents[file.Path] = append([]byte(nil), file.Content...)
	}

	plan, err := orbit.ResolveProjectionPlan(orbit.RepositoryConfig{
		Global: orbit.DefaultGlobalConfig(),
		Orbits: []orbit.Definition{definition},
	}, spec, trackedFiles)
	if err != nil {
		return fmt.Errorf("resolve template capability projection plan: %w", err)
	}
	if err := orbit.PreflightResolvedCapabilitiesFromFiles(spec, trackedFiles, plan.ExportPaths, fileContents); err != nil {
		return fmt.Errorf("preflight template capabilities: %w", err)
	}

	return nil
}

func loadTemplateManifestFromRevision(displayRef string, branchManifest orbitTemplateBranchManifest) (Manifest, error) {
	manifest, err := manifestFromOrbitTemplateBranchManifest(branchManifest)
	if err != nil {
		return Manifest{}, fmt.Errorf("template source %q is not a valid orbit template branch: %w", displayRef, err)
	}

	return manifest, nil
}

func manifestFromOrbitTemplateBranchManifest(branchManifest orbitTemplateBranchManifest) (Manifest, error) {
	manifest := Manifest{
		SchemaVersion: manifestSchemaVersion,
		Kind:          TemplateKind,
		Template: Metadata{
			OrbitID:           branchManifest.Template.OrbitID,
			DefaultTemplate:   branchManifest.Template.DefaultTemplate != nil && *branchManifest.Template.DefaultTemplate,
			CreatedFromBranch: branchManifest.Template.CreatedFromBranch,
			CreatedFromCommit: branchManifest.Template.CreatedFromCommit,
			CreatedAt:         branchManifest.Template.CreatedAt,
		},
		Variables: map[string]VariableSpec{},
	}
	for name, spec := range branchManifest.Variables {
		manifest.Variables[name] = spec
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, fmt.Errorf("validate %s payload: %w", branchManifestPath, err)
	}

	return manifest, nil
}

// BuildTemplateApplyPreview resolves a local template branch, resolves bindings, renders files,
// and reports any conflicts without writing to the runtime repository.
func BuildTemplateApplyPreview(ctx context.Context, input TemplateApplyPreviewInput) (TemplateApplyPreview, error) {
	if err := ensureTemplateApplyTargetInitialized(input.RepoRoot); err != nil {
		return TemplateApplyPreview{}, err
	}

	source, err := ResolveLocalTemplateSource(ctx, input.RepoRoot, input.SourceRef)
	if err != nil {
		return TemplateApplyPreview{}, err
	}

	localInputs, err := loadTemplateApplyLocalInputs(ctx, input.RepoRoot, input.BindingsFilePath)
	if err != nil {
		return TemplateApplyPreview{}, err
	}

	return buildTemplateApplyPreviewFromSourceWithLocalInputs(ctx, input.RepoRoot, source, Source{
		SourceKind:     InstallSourceKindLocalBranch,
		SourceRepo:     "",
		SourceRef:      source.Ref,
		TemplateCommit: source.Commit,
	}, localInputs, input.VariableNamespaces, input.RuntimeInstallOrbitIDs, input.SkipSharedAgentsWrite, input.AllowUnresolvedBindings, input.Interactive, input.Prompter, input.EditorMode, input.Editor, input.Now)
}

// BuildRemoteTemplateApplyPreview resolves a remote template source, renders files, and reports conflicts without writing.
func BuildRemoteTemplateApplyPreview(ctx context.Context, input RemoteTemplateApplyPreviewInput) (TemplateApplyPreview, error) {
	if err := ensureTemplateApplyTargetInitialized(input.RepoRoot); err != nil {
		return TemplateApplyPreview{}, err
	}

	localInputs, err := loadTemplateApplyLocalInputs(ctx, input.RepoRoot, input.BindingsFilePath)
	if err != nil {
		return TemplateApplyPreview{}, err
	}

	candidate, source, err := resolveRemoteTemplateSourceSnapshot(ctx, input.RepoRoot, input.RemoteURL, input.RequestedRef)
	if err != nil {
		return TemplateApplyPreview{}, err
	}

	preview, err := buildTemplateApplyPreviewFromSourceWithLocalInputs(ctx, input.RepoRoot, source, Source{
		SourceKind:     InstallSourceKindExternalGit,
		SourceRepo:     candidate.RepoURL,
		SourceRef:      candidate.Branch,
		TemplateCommit: source.Commit,
	}, localInputs, input.VariableNamespaces, input.RuntimeInstallOrbitIDs, input.SkipSharedAgentsWrite, input.AllowUnresolvedBindings, input.Interactive, input.Prompter, input.EditorMode, input.Editor, input.Now)
	if err != nil {
		return TemplateApplyPreview{}, err
	}

	preview.RemoteRequestedRef = candidate.RequestedRef
	preview.RemoteResolutionKind = candidate.ResolutionKind

	return preview, nil
}

func ensureTemplateApplyTargetInitialized(repoRoot string) error {
	_, manifestErr := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(runtimeManifestRelativePath)))
	if manifestErr == nil {
		return nil
	}
	if !errors.Is(manifestErr, os.ErrNotExist) {
		return fmt.Errorf("stat runtime manifest: %w", manifestErr)
	}

	_, configErr := os.Stat(orbit.ConfigPath(repoRoot))
	if configErr == nil {
		return nil
	}
	if errors.Is(configErr, os.ErrNotExist) {
		return fmt.Errorf("orbit is not initialized; run `orbit init` first")
	}

	return fmt.Errorf("stat runtime config: %w", configErr)
}

func buildTemplateApplyPreviewFromSource(
	ctx context.Context,
	repoRoot string,
	source LocalTemplateSource,
	installSource Source,
	bindingsFilePath string,
	variableNamespaces map[string]string,
	runtimeInstallOrbitIDs []string,
	skipSharedAgentsWrite bool,
	allowUnresolvedBindings bool,
	interactive bool,
	prompter BindingPrompter,
	editorMode bool,
	editor Editor,
	now time.Time,
) (TemplateApplyPreview, error) {
	localInputs, err := loadTemplateApplyLocalInputs(ctx, repoRoot, bindingsFilePath)
	if err != nil {
		return TemplateApplyPreview{}, err
	}

	return buildTemplateApplyPreviewFromSourceWithLocalInputs(
		ctx,
		repoRoot,
		source,
		installSource,
		localInputs,
		variableNamespaces,
		runtimeInstallOrbitIDs,
		skipSharedAgentsWrite,
		allowUnresolvedBindings,
		interactive,
		prompter,
		editorMode,
		editor,
		now,
	)
}

func buildTemplateApplyPreviewFromSourceWithLocalInputs(
	ctx context.Context,
	repoRoot string,
	source LocalTemplateSource,
	installSource Source,
	localInputs templateApplyLocalInputs,
	variableNamespaces map[string]string,
	runtimeInstallOrbitIDs []string,
	skipSharedAgentsWrite bool,
	allowUnresolvedBindings bool,
	interactive bool,
	prompter BindingPrompter,
	editorMode bool,
	editor Editor,
	now time.Time,
) (TemplateApplyPreview, error) {
	bindingsFile := localInputs.bindingsFile
	repoVarsFile := localInputs.repoVarsFile
	hasRepoVarsFile := localInputs.hasRepoVarsFile
	repoVars := repoVarsFile.Variables
	orbitID := source.Manifest.Template.OrbitID

	declared := make(map[string]bindings.VariableDeclaration, len(source.Manifest.Variables))
	for name, spec := range source.Manifest.Variables {
		declared[name] = bindings.VariableDeclaration{
			Description: spec.Description,
			Required:    spec.Required,
		}
	}
	runtimeNamespaces, err := resolveRuntimeInstallVariableNamespaces(repoRoot, orbitID, declared, runtimeInstallOrbitIDs)
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("resolve runtime variable namespaces: %w", err)
	}
	namespaceByVariable := mergeVariableNamespaces(runtimeNamespaces, variableNamespaces)

	mergeResult, err := resolveTemplateBindings(
		ctx,
		declared,
		bindingsFile.Variables,
		bindings.ScopedVariablesForNamespace(bindingsFile, orbitID),
		repoVars,
		bindings.ScopedVariablesForNamespace(repoVarsFile, orbitID),
		orbitID,
		namespaceByVariable,
		interactive,
		prompter,
		editorMode,
		editor,
	)
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("merge bindings: %w", err)
	}
	unresolvedNames := unresolvedBindingNames(mergeResult.Unresolved)
	if len(unresolvedNames) > 0 && !allowUnresolvedBindings {
		return TemplateApplyPreview{}, fmt.Errorf("missing required bindings: %s", strings.Join(unresolvedNames, ", "))
	}

	renderValues := make(map[string]string, len(mergeResult.Resolved))
	for name, resolved := range mergeResult.Resolved {
		renderValues[name] = resolved.Value
	}

	renderedFiles, err := renderTemplateFilesWithOptions(source.Files, renderValues, renderTemplateOptions{
		AllowUnresolved: allowUnresolvedBindings,
	})
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("render template files: %w", err)
	}
	renderedSharedAgentsFile, hasSharedAgents, err := renderSharedAgentsPayloadWithOptions(source, renderValues, renderTemplateOptions{
		AllowUnresolved: allowUnresolvedBindings,
	})
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("render shared AGENTS payload: %w", err)
	}
	var renderedSharedAgentsFilePtr *CandidateFile
	if hasSharedAgents {
		renderedSharedAgentsFilePtr = &renderedSharedAgentsFile
	}

	varsFile := planResolvedBindingsWrite(repoVarsFile, hasRepoVarsFile, mergeResult.Resolved)
	installRecord := InstallRecord{
		SchemaVersion: installRecordSchemaVersion,
		OrbitID:       orbitID,
		Template:      installSource,
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
		observed := collectObservedRuntimeUnresolved(
			renderedFiles,
			renderedSharedAgentsFilePtr,
		)
		if len(observed) > 0 {
			installRecord.Variables.ObservedRuntimeUnresolved = observed
		}
	}

	conflicts, warnings, err := analyzeApplyConflicts(
		ctx,
		repoRoot,
		source,
		renderedFiles,
		renderedSharedAgentsFilePtr,
		skipSharedAgentsWrite,
		repoVarsFile,
		hasRepoVarsFile,
		varsFile,
		installRecord,
	)
	if err != nil {
		return TemplateApplyPreview{}, fmt.Errorf("analyze apply conflicts: %w", err)
	}
	if len(unresolvedNames) > 0 {
		warnings = append([]string{
			fmt.Sprintf("install kept template variables unresolved: %s", strings.Join(unresolvedNames, ", ")),
		}, warnings...)
	}

	preview := TemplateApplyPreview{
		Source:                   source,
		ResolvedBindings:         mergeResult.Resolved,
		RenderedFiles:            renderedFiles,
		RenderedSharedAgentsFile: renderedSharedAgentsFilePtr,
		InstallRecord:            installRecord,
		VarsFile:                 varsFile,
		Conflicts:                conflicts,
		Warnings:                 warnings,
	}

	return preview, nil
}

func loadTemplateApplyLocalInputs(ctx context.Context, repoRoot string, bindingsFilePath string) (templateApplyLocalInputs, error) {
	bindingsFile, err := loadOptionalBindingsFile(bindingsFilePath)
	if err != nil {
		return templateApplyLocalInputs{}, fmt.Errorf("load --bindings file: %w", err)
	}
	repoVarsFile, hasRepoVarsFile, err := loadOptionalRepoVarsFile(ctx, repoRoot)
	if err != nil {
		return templateApplyLocalInputs{}, fmt.Errorf("load runtime vars: %w", err)
	}

	return templateApplyLocalInputs{
		bindingsFile:    bindingsFile,
		repoVarsFile:    repoVarsFile,
		hasRepoVarsFile: hasRepoVarsFile,
	}, nil
}

// PreflightRemoteTemplateApplyLocalInputs validates the local bindings sources needed before remote template resolution.
func PreflightRemoteTemplateApplyLocalInputs(ctx context.Context, repoRoot string, bindingsFilePath string) error {
	_, err := loadTemplateApplyLocalInputs(ctx, repoRoot, bindingsFilePath)
	return err
}

// ApplyLocalTemplate writes the rendered local template payload into the runtime repository.
func ApplyLocalTemplate(ctx context.Context, input TemplateApplyInput) (TemplateApplyResult, error) {
	preview, err := BuildTemplateApplyPreview(ctx, input.Preview)
	if err != nil {
		return TemplateApplyResult{}, err
	}
	return applyTemplatePreview(input.Preview.RepoRoot, preview, input.Preview.OverwriteExisting, input.Preview.SkipSharedAgentsWrite)
}

// ApplyRemoteTemplate writes the rendered remote template payload into the runtime repository.
func ApplyRemoteTemplate(ctx context.Context, input RemoteTemplateApplyInput) (TemplateApplyResult, error) {
	preview, err := BuildRemoteTemplateApplyPreview(ctx, input.Preview)
	if err != nil {
		return TemplateApplyResult{}, err
	}

	return applyTemplatePreview(input.Preview.RepoRoot, preview, input.Preview.OverwriteExisting, input.Preview.SkipSharedAgentsWrite)
}

func applyTemplatePreview(repoRoot string, preview TemplateApplyPreview, overwriteExisting bool, skipSharedAgentsWrite bool) (TemplateApplyResult, error) {
	if len(preview.Conflicts) > 0 && !overwriteExisting {
		return TemplateApplyResult{}, fmt.Errorf("conflicts detected; re-run with --overwrite-existing to allow replacing existing runtime files")
	}

	writtenPaths := make([]string, 0, len(preview.RenderedFiles)+4)
	for _, file := range preview.RenderedFiles {
		filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		perm, err := gitpkg.FilePermForMode(file.Mode)
		if err != nil {
			return TemplateApplyResult{}, fmt.Errorf("resolve rendered file mode %s: %w", file.Path, err)
		}
		if err := contractutil.AtomicWriteFileMode(filename, file.Content, perm); err != nil {
			return TemplateApplyResult{}, fmt.Errorf("write rendered file %s: %w", file.Path, err)
		}
		writtenPaths = append(writtenPaths, file.Path)
	}
	if preview.RenderedSharedAgentsFile != nil && !skipSharedAgentsWrite {
		if err := applySharedAgentsPayload(repoRoot, preview.Source.Manifest.Template.OrbitID, preview.RenderedSharedAgentsFile.Content); err != nil {
			return TemplateApplyResult{}, fmt.Errorf("write runtime AGENTS.md: %w", err)
		}
		writtenPaths = append(writtenPaths, sharedFilePathAgents)
	}

	definitionPath, err := writeRuntimeCompanionDefinition(repoRoot, preview.Source)
	if err != nil {
		return TemplateApplyResult{}, fmt.Errorf("write orbit definition: %w", err)
	}
	installPath, err := writeRuntimeInstallRecord(repoRoot, preview.InstallRecord)
	if err != nil {
		return TemplateApplyResult{}, fmt.Errorf("write install record: %w", err)
	}
	writtenPaths = append(writtenPaths,
		mustRepoRelativePath(repoRoot, definitionPath),
		mustRepoRelativePath(repoRoot, installPath),
	)
	if preview.VarsFile != nil {
		varsPath, err := writeRuntimeVarsFile(repoRoot, *preview.VarsFile)
		if err != nil {
			return TemplateApplyResult{}, fmt.Errorf("write runtime vars: %w", err)
		}
		writtenPaths = append(writtenPaths, mustRepoRelativePath(repoRoot, varsPath))
	}
	sort.Strings(writtenPaths)

	return TemplateApplyResult{
		Preview:      preview,
		WrittenPaths: writtenPaths,
	}, nil
}

func loadOptionalBindingsFile(filename string) (bindings.VarsFile, error) {
	if strings.TrimSpace(filename) == "" {
		return bindings.VarsFile{
			SchemaVersion: 1,
			Variables:     map[string]bindings.VariableBinding{},
		}, nil
	}

	//nolint:gosec // The bindings file path is an explicit user-provided local file path.
	data, err := os.ReadFile(filename)
	if err != nil {
		return bindings.VarsFile{}, fmt.Errorf("read %s: %w", filename, err)
	}

	file, err := bindings.ParseVarsData(data)
	if err != nil {
		return bindings.VarsFile{}, fmt.Errorf("parse %s: %w", filename, err)
	}

	return file, nil
}

func loadOptionalRepoVarsFile(ctx context.Context, repoRoot string) (bindings.VarsFile, bool, error) {
	empty := bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     map[string]bindings.VariableBinding{},
	}
	if _, err := os.Stat(runtimeVarsPath(repoRoot)); err == nil {
		file, err := loadRuntimeVarsFile(repoRoot)
		if err != nil {
			return bindings.VarsFile{}, false, fmt.Errorf("load runtime vars from worktree: %w", err)
		}

		return file, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return bindings.VarsFile{}, false, fmt.Errorf("stat runtime vars in worktree: %w", err)
	}

	existsAtHead, err := gitpkg.PathExistsAtRev(ctx, repoRoot, "HEAD", runtimeVarsRepoPath())
	if err != nil {
		return bindings.VarsFile{}, false, fmt.Errorf("check runtime vars at HEAD: %w", err)
	}
	if !existsAtHead {
		return empty, false, nil
	}

	file, err := loadRuntimeVarsFileWorktreeOrHEAD(ctx, repoRoot)
	if err != nil {
		return bindings.VarsFile{}, false, fmt.Errorf("load runtime vars from worktree or HEAD: %w", err)
	}

	return file, true, nil
}

func mergeVariableNamespaces(namespaceMaps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, values := range namespaceMaps {
		for name, namespace := range values {
			trimmed := strings.TrimSpace(namespace)
			if trimmed == "" {
				continue
			}
			merged[name] = trimmed
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func planResolvedBindingsWrite(
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
			Variables: cloneVariableBindings(scoped.Variables),
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
		if strings.TrimSpace(binding.Namespace) != "" {
			scoped := scopedMerged[binding.Namespace]
			if scoped.Variables == nil {
				scoped.Variables = map[string]bindings.VariableBinding{}
			}
			if current, ok := scoped.Variables[name]; ok && reflect.DeepEqual(current, next) {
				continue
			}

			scoped.Variables[name] = next
			scopedMerged[binding.Namespace] = scoped
			changed = true
			continue
		}

		if current, ok := merged[name]; ok && reflect.DeepEqual(current, next) {
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
	if hasExisting && reflect.DeepEqual(existing, planned) {
		return nil
	}

	return &planned
}

func cloneVariableBindings(values map[string]bindings.VariableBinding) map[string]bindings.VariableBinding {
	if values == nil {
		return nil
	}
	cloned := make(map[string]bindings.VariableBinding, len(values))
	for name, binding := range values {
		cloned[name] = binding
	}
	return cloned
}

func resolveTemplateBindings(
	ctx context.Context,
	declared map[string]bindings.VariableDeclaration,
	bindingsFile map[string]bindings.VariableBinding,
	bindingsFileScoped map[string]bindings.VariableBinding,
	repoVars map[string]bindings.VariableBinding,
	repoVarsScoped map[string]bindings.VariableBinding,
	namespace string,
	namespaceByVariable map[string]string,
	interactive bool,
	prompter BindingPrompter,
	editorMode bool,
	editor Editor,
) (bindings.MergeResult, error) {
	mergeInput := bindings.MergeInput{
		Declared:            declared,
		BindingsFile:        bindingsFile,
		BindingsFileScoped:  bindingsFileScoped,
		RepoVars:            repoVars,
		RepoVarsScoped:      repoVarsScoped,
		Namespace:           namespace,
		NamespaceByVariable: namespaceByVariable,
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
		fillIn, err := editMissingBindings(ctx, mergeResult.Unresolved, editor)
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

func unresolvedBindingNames(unresolved []bindings.UnresolvedBinding) []string {
	names := make([]string, 0, len(unresolved))
	for _, item := range unresolved {
		names = append(names, item.Name)
	}
	sort.Strings(names)

	return names
}

// BuildInstallVariablesSnapshot captures declaration, resolved, and unresolved binding provenance.
func BuildInstallVariablesSnapshot(
	declared map[string]bindings.VariableDeclaration,
	mergeResult bindings.MergeResult,
) *InstallVariablesSnapshot {
	if len(declared) == 0 && len(mergeResult.Resolved) == 0 && len(mergeResult.Unresolved) == 0 {
		return nil
	}

	snapshot := &InstallVariablesSnapshot{
		Declarations:    make(map[string]bindings.VariableDeclaration, len(declared)),
		ResolvedAtApply: make(map[string]bindings.VariableBinding, len(mergeResult.Resolved)),
	}
	for name, declaration := range declared {
		snapshot.Declarations[name] = declaration
	}
	for name, resolved := range mergeResult.Resolved {
		snapshot.ResolvedAtApply[name] = bindings.VariableBinding{
			Value:       resolved.Value,
			Description: resolved.Description,
		}
		if strings.TrimSpace(resolved.Namespace) != "" {
			if snapshot.Namespaces == nil {
				snapshot.Namespaces = map[string]string{}
			}
			snapshot.Namespaces[name] = resolved.Namespace
		}
	}
	if len(mergeResult.Unresolved) > 0 {
		snapshot.UnresolvedAtApply = unresolvedBindingNames(mergeResult.Unresolved)
		for _, unresolved := range mergeResult.Unresolved {
			if strings.TrimSpace(unresolved.Namespace) == "" {
				continue
			}
			if snapshot.Namespaces == nil {
				snapshot.Namespaces = map[string]string{}
			}
			snapshot.Namespaces[unresolved.Name] = unresolved.Namespace
		}
	}

	return snapshot
}

func analyzeApplyConflicts(
	ctx context.Context,
	repoRoot string,
	source LocalTemplateSource,
	renderedFiles []CandidateFile,
	renderedSharedAgentsFile *CandidateFile,
	skipSharedAgentsWrite bool,
	repoVarsFile bindings.VarsFile,
	hasRepoVarsFile bool,
	varsFile *bindings.VarsFile,
	installRecord InstallRecord,
) ([]ApplyConflict, []string, error) {
	conflicts := make([]ApplyConflict, 0)
	definitionPath, err := orbit.HostedDefinitionPath(repoRoot, source.Manifest.Template.OrbitID)
	if err != nil {
		return nil, nil, fmt.Errorf("build runtime definition path: %w", err)
	}
	plannedDefinitionData, err := plannedRuntimeCompanionWrite(source)
	if err != nil {
		return nil, nil, fmt.Errorf("build runtime definition plan: %w", err)
	}
	//nolint:gosec // The definition path is built from a validated orbit id under the repo root.
	if data, err := os.ReadFile(definitionPath); err == nil {
		if string(data) != string(plannedDefinitionData) {
			conflicts = append(conflicts, ApplyConflict{
				Path:    filepath.ToSlash(strings.TrimPrefix(definitionPath, repoRoot+"/")),
				Message: fmt.Sprintf("orbit %q already exists with different definition content", source.Manifest.Template.OrbitID),
			})
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read runtime definition %s: %w", definitionPath, err)
	}

	installPath, err := runtimeInstallRecordPath(repoRoot, source.Manifest.Template.OrbitID)
	if err != nil {
		return nil, nil, fmt.Errorf("build runtime install record path: %w", err)
	}
	if existing, err := loadRuntimeInstallRecord(repoRoot, source.Manifest.Template.OrbitID); err == nil {
		if existing.Template.SourceKind != installRecord.Template.SourceKind ||
			existing.Template.SourceRef != installRecord.Template.SourceRef ||
			existing.Template.TemplateCommit != installRecord.Template.TemplateCommit {
			conflicts = append(conflicts, ApplyConflict{
				Path:    filepath.ToSlash(strings.TrimPrefix(installPath, repoRoot+"/")),
				Message: fmt.Sprintf("install record for orbit %q already points to a different template source", source.Manifest.Template.OrbitID),
			})
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read runtime install record %s: %w", installPath, err)
	}

	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("load runtime worktree status: %w", err)
	}
	statusByPath := make(map[string]gitpkg.StatusEntry, len(statusEntries))
	for _, entry := range statusEntries {
		statusByPath[entry.Path] = entry
	}

	for _, file := range renderedFiles {
		filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		//nolint:gosec // The rendered target path is repo-local and built from a validated repo-relative path.
		if data, err := os.ReadFile(filename); err == nil && string(data) != string(file.Content) {
			conflicts = append(conflicts, ApplyConflict{
				Path:    file.Path,
				Message: "target path already exists with different content",
			})
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("read runtime path %s: %w", file.Path, err)
		}

		if status, ok := statusByPath[file.Path]; ok {
			conflicts = append(conflicts, ApplyConflict{
				Path:    file.Path,
				Message: fmt.Sprintf("target path has uncommitted worktree status %s", status.Code),
			})
		}
	}
	if varsFile != nil {
		varsPath := mustRepoRelativePath(repoRoot, runtimeVarsPath(repoRoot))
		if hasRepoVarsFile && !reflect.DeepEqual(repoVarsFile, *varsFile) {
			conflicts = append(conflicts, ApplyConflict{
				Path:    varsPath,
				Message: "target path already exists with different content",
			})
		}
		if status, ok := statusByPath[varsPath]; ok {
			conflicts = append(conflicts, ApplyConflict{
				Path:    varsPath,
				Message: fmt.Sprintf("target path has uncommitted worktree status %s", status.Code),
			})
		}
	}
	warnings := make([]string, 0, 1)
	if renderedSharedAgentsFile != nil && !skipSharedAgentsWrite {
		agentsConflicts, agentsWarnings, err := analyzeSharedAgentsApply(repoRoot, source.Manifest.Template.OrbitID, statusByPath)
		if err != nil {
			return nil, nil, err
		}
		conflicts = append(conflicts, agentsConflicts...)
		warnings = append(warnings, agentsWarnings...)
	}

	sort.Slice(conflicts, func(left, right int) bool {
		if conflicts[left].Path == conflicts[right].Path {
			return conflicts[left].Message < conflicts[right].Message
		}
		return conflicts[left].Path < conflicts[right].Path
	})

	return conflicts, warnings, nil
}

func resolveApplyTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}

	return now.UTC()
}

func resolveRevisionCommit(ctx context.Context, repoRoot string, rev string) (string, error) {
	value, err := gitpkg.ResolveRevision(ctx, repoRoot, rev)
	if err != nil {
		return "", fmt.Errorf("resolve revision %q: %w", rev, err)
	}

	return value, nil
}

func mustRepoRelativePath(repoRoot string, absolutePath string) string {
	relativePath, err := filepath.Rel(repoRoot, absolutePath)
	if err != nil {
		return absolutePath
	}
	normalizedPath, err := ids.NormalizeRepoRelativePath(filepath.ToSlash(relativePath))
	if err != nil {
		return filepath.ToSlash(relativePath)
	}

	return normalizedPath
}

func runtimeVarsRepoPath() string {
	return runtimeVarsRepoRelativePath
}

func runtimeVarsPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(runtimeVarsRepoRelativePath))
}

func loadRuntimeVarsFile(repoRoot string) (bindings.VarsFile, error) {
	file, err := bindings.LoadVarsFileAtPath(runtimeVarsPath(repoRoot))
	if err != nil {
		return bindings.VarsFile{}, fmt.Errorf("load runtime vars file: %w", err)
	}

	return file, nil
}

func loadRuntimeVarsFileWorktreeOrHEAD(ctx context.Context, repoRoot string) (bindings.VarsFile, error) {
	file, err := bindings.LoadVarsFileWorktreeOrHEADAtRepoPath(ctx, repoRoot, runtimeVarsRepoPath())
	if err != nil {
		return bindings.VarsFile{}, fmt.Errorf("load runtime vars file: %w", err)
	}

	return file, nil
}

func writeRuntimeVarsFile(repoRoot string, file bindings.VarsFile) (string, error) {
	filename, err := bindings.WriteVarsFileAtPath(runtimeVarsPath(repoRoot), file)
	if err != nil {
		return "", fmt.Errorf("write runtime vars file: %w", err)
	}

	return filename, nil
}

func runtimeInstallRecordRepoPath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return runtimeInstallRecordRelativeDir + "/" + orbitID + ".yaml", nil
}

func runtimeInstallRecordPath(repoRoot string, orbitID string) (string, error) {
	repoPath, err := runtimeInstallRecordRepoPath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(repoPath)), nil
}

func loadRuntimeInstallRecord(repoRoot string, orbitID string) (InstallRecord, error) {
	filename, err := runtimeInstallRecordPath(repoRoot, orbitID)
	if err != nil {
		return InstallRecord{}, fmt.Errorf("build runtime install record path: %w", err)
	}

	record, err := LoadInstallRecordFile(filename)
	if err != nil {
		return InstallRecord{}, fmt.Errorf("load runtime install record: %w", err)
	}
	if record.OrbitID != orbitID {
		return InstallRecord{}, fmt.Errorf("validate %s: orbit_id must match install path", filename)
	}

	return record, nil
}

func writeRuntimeInstallRecord(repoRoot string, record InstallRecord) (string, error) {
	filename, err := runtimeInstallRecordPath(repoRoot, record.OrbitID)
	if err != nil {
		return "", fmt.Errorf("build runtime install record path: %w", err)
	}

	written, err := WriteInstallRecordFile(filename, record)
	if err != nil {
		return "", fmt.Errorf("write runtime install record: %w", err)
	}

	return written, nil
}

func stableDefinitionData(definition orbit.Definition) ([]byte, error) {
	stable := definition
	if stable.Exclude == nil {
		stable.Exclude = []string{}
	}

	data, err := yaml.Marshal(stable)
	if err != nil {
		return nil, fmt.Errorf("marshal orbit definition: %w", err)
	}

	return append(data, '\n'), nil
}

func writeRuntimeCompanionDefinition(repoRoot string, source LocalTemplateSource) (string, error) {
	if !source.Spec.HasMemberSchema() {
		filename, err := orbit.WriteHostedDefinition(repoRoot, source.Definition)
		if err != nil {
			return "", fmt.Errorf("write hosted definition: %w", err)
		}
		return filename, nil
	}

	runtimeSpec := source.Spec
	runtimeSpec.SourcePath = ""

	runtimePath, err := orbit.HostedDefinitionRelativePath(runtimeSpec.ID)
	if err != nil {
		return "", fmt.Errorf("build hosted runtime companion path: %w", err)
	}
	if runtimeSpec.Meta != nil {
		runtimeSpec.Meta.File = runtimePath
	}

	filename, err := orbit.WriteHostedOrbitSpec(repoRoot, runtimeSpec)
	if err != nil {
		return "", fmt.Errorf("write hosted orbit spec: %w", err)
	}

	return filename, nil
}
