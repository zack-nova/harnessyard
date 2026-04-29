package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// TemplateSavePreviewInput describes the shared runtime-to-template preview pipeline.
type TemplateSavePreviewInput struct {
	RepoRoot                  string
	OrbitID                   string
	TargetBranch              string
	DefaultBranch             bool
	Now                       time.Time
	EditTemplate              bool
	Editor                    Editor
	ManifestMeta              *Metadata
	Warnings                  []string
	IncludeCompletedBootstrap bool
}

// TemplateSavePreview contains the fully built template candidate plus branch-manifest metadata.
type TemplateSavePreview struct {
	RepoRoot             string
	OrbitID              string
	TargetBranch         string
	Files                []CandidateFile
	ReplacementSummaries []FileReplacementSummary
	Ambiguities          []FileReplacementAmbiguity
	Warnings             []string
	Manifest             Manifest
}

// FilePaths returns the stable preview file list without the generated manifest path.
func (preview TemplateSavePreview) FilePaths() []string {
	paths := make([]string, 0, len(preview.Files))
	for _, file := range preview.Files {
		paths = append(paths, file.Path)
	}

	return paths
}

// TemplateSaveInput describes the real branch-writing save path.
type TemplateSaveInput struct {
	Preview      TemplateSavePreviewInput
	Overwrite    bool
	ParentCommit string
}

// TemplateSaveWriteInput describes writing one already-built preview to a branch.
type TemplateSaveWriteInput struct {
	Preview            TemplateSavePreview
	Overwrite          bool
	ParentCommit       string
	AllowCurrentBranch bool
}

// TemplateSaveResult contains the preview plus the written template branch result.
type TemplateSaveResult struct {
	Preview     TemplateSavePreview
	WriteResult gitpkg.WriteTemplateBranchResult
}

// BuildTemplateSavePreview runs the documented Phase 2A-1 runtime-to-template preview pipeline.
func BuildTemplateSavePreview(ctx context.Context, input TemplateSavePreviewInput) (TemplateSavePreview, error) {
	repoConfig, err := loadTemplateSaveRepositoryConfig(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load repository config: %w", err)
	}
	if err := orbit.ValidateRepositoryConfig(repoConfig.Global, repoConfig.Orbits); err != nil {
		return TemplateSavePreview{}, fmt.Errorf("validate repository config: %w", err)
	}

	definition, found := repoConfig.OrbitByID(input.OrbitID)
	if !found {
		fallbackConfig, fallbackErr := orbit.LoadRepositoryConfig(ctx, input.RepoRoot)
		if fallbackErr != nil {
			return TemplateSavePreview{}, fmt.Errorf("orbit %q not found", input.OrbitID)
		}
		if err := orbit.ValidateRepositoryConfig(fallbackConfig.Global, fallbackConfig.Orbits); err != nil {
			return TemplateSavePreview{}, fmt.Errorf("validate repository config: %w", err)
		}
		fallbackDefinition, fallbackFound := fallbackConfig.OrbitByID(input.OrbitID)
		if !fallbackFound {
			return TemplateSavePreview{}, fmt.Errorf("orbit %q not found", input.OrbitID)
		}
		repoConfig = fallbackConfig
		definition = fallbackDefinition
	}

	trackedFiles, err := gitpkg.TrackedFiles(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load tracked files: %w", err)
	}

	spec, plan, err := orbit.LoadOrbitSpecAndProjectionPlan(ctx, input.RepoRoot, repoConfig, definition.ID, trackedFiles)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load orbit export plan: %w", err)
	}
	filteredExport, err := FilterCompletedBootstrapExportPaths(ctx, RuntimeExportBootstrapFilterInput{
		RepoRoot:                  input.RepoRoot,
		OrbitID:                   definition.ID,
		Spec:                      spec,
		ExportPaths:               plan.ExportPaths,
		IncludeCompletedBootstrap: input.IncludeCompletedBootstrap,
	})
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("filter runtime bootstrap export paths: %w", err)
	}

	varsFile, _, err := loadOptionalRepoVarsFile(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load runtime vars: %w", err)
	}

	warnings := append([]string(nil), input.Warnings...)
	warnings = append(warnings, filteredExport.Warnings...)

	if err := orbit.PreflightResolvedCapabilities(input.RepoRoot, spec, trackedFiles, filteredExport.ExportPaths); err != nil {
		return TemplateSavePreview{}, fmt.Errorf("preflight resolved capabilities: %w", err)
	}

	buildResult, err := BuildTemplateContent(ctx, BuildInput{
		RepoRoot:  input.RepoRoot,
		OrbitID:   definition.ID,
		UserScope: filteredExport.ExportPaths,
		Bindings:  varsFile.Variables,
	})
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("build template content: %w", err)
	}
	buildResult.Files = normalizeTemplateSaveFiles(buildResult.Files)
	sortFileReplacementSummaries(buildResult.ReplacementSummaries)
	sortFileReplacementAmbiguities(buildResult.Ambiguities)
	payloadWarnings, err := ensureWorktreeExportPayloadIsPresent(ctx, input.RepoRoot, repoConfig, spec, buildResult.Files, input.IncludeCompletedBootstrap)
	if err != nil {
		return TemplateSavePreview{}, err
	}
	warnings = append(warnings, payloadWarnings...)

	state, err := LoadCurrentRepoState(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load current repo state: %w", err)
	}
	currentBranch, err := RequireCurrentBranch(state, "template save")
	if err != nil {
		return TemplateSavePreview{}, err
	}
	headCommit, err := CurrentCommitOrZero(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("resolve current provenance commit: %w", err)
	}

	files := buildResult.Files
	if input.EditTemplate {
		files, err = editTemplateFiles(ctx, definition.ID, buildResult.Files, input.Editor)
		if err != nil {
			return TemplateSavePreview{}, fmt.Errorf("edit template candidate: %w", err)
		}
		files = normalizeTemplateSaveFiles(files)
	}

	createdFromBranch := currentBranch
	createdFromCommit := headCommit
	createdAt := resolveSaveTime(input.Now)
	if input.ManifestMeta != nil {
		if input.ManifestMeta.CreatedFromBranch != "" {
			createdFromBranch = input.ManifestMeta.CreatedFromBranch
		}
		if input.ManifestMeta.CreatedFromCommit != "" {
			createdFromCommit = input.ManifestMeta.CreatedFromCommit
		}
		if !input.ManifestMeta.CreatedAt.IsZero() {
			createdAt = input.ManifestMeta.CreatedAt.UTC()
		}
	}

	manifest, err := buildTemplateSaveManifest(
		definition.ID,
		input.DefaultBranch,
		createdFromBranch,
		createdFromCommit,
		createdAt,
		varsFile.Variables,
		files,
	)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("build template manifest: %w", err)
	}

	return TemplateSavePreview{
		RepoRoot:             input.RepoRoot,
		OrbitID:              definition.ID,
		TargetBranch:         input.TargetBranch,
		Files:                files,
		ReplacementSummaries: buildResult.ReplacementSummaries,
		Ambiguities:          buildResult.Ambiguities,
		Warnings:             warnings,
		Manifest:             manifest,
	}, nil
}

func ensureWorktreeExportPayloadIsPresent(
	ctx context.Context,
	repoRoot string,
	repoConfig orbit.RepositoryConfig,
	spec orbit.OrbitSpec,
	payloadFiles []CandidateFile,
	includeCompletedBootstrap bool,
) ([]string, error) {
	if !spec.HasMemberSchema() {
		return nil, nil
	}

	worktreeFiles, err := gitpkg.WorktreeFiles(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load worktree files: %w", err)
	}

	worktreePlan, err := orbit.ResolveProjectionPlan(repoConfig, spec, worktreeFiles)
	if err != nil {
		return nil, fmt.Errorf("load worktree export plan: %w", err)
	}
	filteredWorktreeExport, err := FilterCompletedBootstrapExportPaths(ctx, RuntimeExportBootstrapFilterInput{
		RepoRoot:                  repoRoot,
		OrbitID:                   spec.ID,
		Spec:                      spec,
		ExportPaths:               worktreePlan.ExportPaths,
		IncludeCompletedBootstrap: includeCompletedBootstrap,
	})
	if err != nil {
		return nil, fmt.Errorf("filter worktree bootstrap export paths: %w", err)
	}

	payloadSet := make(map[string]struct{}, len(payloadFiles))
	for _, file := range payloadFiles {
		payloadSet[file.Path] = struct{}{}
	}

	missingPayloadPaths := make([]string, 0)
	skippedRuntimeGuidancePaths := make([]string, 0)
	for _, path := range filteredWorktreeExport.ExportPaths {
		if _, ok := payloadSet[path]; ok {
			continue
		}
		if isRuntimeGuidanceTemplateArtifact(path) {
			skippedRuntimeGuidancePaths = append(skippedRuntimeGuidancePaths, path)
			continue
		}
		missingPayloadPaths = append(missingPayloadPaths, path)
	}

	warnings := skippedRuntimeGuidanceExportWarnings(spec.ID, skippedRuntimeGuidancePaths)
	if len(missingPayloadPaths) == 0 {
		return warnings, nil
	}

	if len(missingPayloadPaths) == 1 {
		path := missingPayloadPaths[0]
		return warnings, fmt.Errorf("member export path %q is missing from the template payload; run git add %s before template save", path, path)
	}

	return warnings, fmt.Errorf(
		"member export paths are missing from the template payload: %s; run git add %s before template save",
		strings.Join(missingPayloadPaths, ", "),
		strings.Join(missingPayloadPaths, " "),
	)
}

func isRuntimeGuidanceTemplateArtifact(path string) bool {
	return path == sharedFilePathAgents || path == runtimeHumansRepoPath || path == runtimeBootstrapRepoPath
}

func skippedRuntimeGuidanceExportWarnings(orbitID string, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	return []string{
		fmt.Sprintf(
			"skip runtime guidance export paths for orbit %q: %s; template publishing uses authored guidance fields instead",
			orbitID,
			strings.Join(paths, ", "),
		),
	}
}

func loadTemplateSaveRepositoryConfig(ctx context.Context, repoRoot string) (orbit.RepositoryConfig, error) {
	repoConfig, err := orbit.LoadRuntimeRepositoryConfig(ctx, repoRoot)
	if err == nil && len(repoConfig.Orbits) > 0 {
		return repoConfig, nil
	}

	discoveredConfig, discoveredErr := loadHostedDiscoveryRepositoryConfig(ctx, repoRoot)
	if discoveredErr == nil && len(discoveredConfig.Orbits) > 0 {
		return discoveredConfig, nil
	}

	if err == nil {
		return repoConfig, nil
	}
	if discoveredErr == nil {
		return discoveredConfig, nil
	}

	return orbit.RepositoryConfig{}, fmt.Errorf("load runtime repository config: %w", err)
}

func loadHostedDiscoveryRepositoryConfig(ctx context.Context, repoRoot string) (orbit.RepositoryConfig, error) {
	hasLegacyGlobalConfig := false
	globalConfig, err := orbit.LoadGlobalConfig(ctx, repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			globalConfig = orbit.DefaultGlobalConfig()
		} else {
			return orbit.RepositoryConfig{}, fmt.Errorf("load global config: %w", err)
		}
	} else {
		hasLegacyGlobalConfig = true
	}

	definitions, err := orbit.DiscoverHostedDefinitions(ctx, repoRoot)
	if err != nil {
		return orbit.RepositoryConfig{}, fmt.Errorf("load hosted orbit definitions: %w", err)
	}

	return orbit.RepositoryConfig{
		Global:                globalConfig,
		Orbits:                definitions,
		HasLegacyGlobalConfig: hasLegacyGlobalConfig,
	}, nil
}

// SaveTemplateBranch writes the previewed template tree to a local branch through the Git writer.
func SaveTemplateBranch(ctx context.Context, input TemplateSaveInput) (TemplateSaveResult, error) {
	preview, err := BuildTemplateSavePreview(ctx, input.Preview)
	if err != nil {
		return TemplateSaveResult{}, err
	}
	return WriteTemplateSavePreview(ctx, TemplateSaveWriteInput{
		Preview:      preview,
		Overwrite:    input.Overwrite,
		ParentCommit: input.ParentCommit,
	})
}

// WriteTemplateSavePreview writes one already-built template preview to a branch.
func WriteTemplateSavePreview(ctx context.Context, input TemplateSaveWriteInput) (TemplateSaveResult, error) {
	preview := input.Preview
	if len(preview.Ambiguities) > 0 {
		return TemplateSaveResult{}, fmt.Errorf("replacement ambiguity detected; resolve the previewed ambiguities before saving")
	}

	files := make([]gitpkg.TemplateTreeFile, 0, len(preview.Files))
	for _, file := range preview.Files {
		files = append(files, gitpkg.TemplateTreeFile{
			Path:    file.Path,
			Content: file.Content,
			Mode:    file.Mode,
		})
	}

	branchManifest, err := branchManifestYAML(preview.Manifest)
	if err != nil {
		return TemplateSaveResult{}, fmt.Errorf("build branch manifest: %w", err)
	}
	currentBranch, err := gitpkg.CurrentBranch(ctx, preview.RepoRoot)
	if err != nil {
		return TemplateSaveResult{}, fmt.Errorf("resolve current branch: %w", err)
	}

	writeResult, err := gitpkg.WriteTemplateBranch(ctx, preview.RepoRoot, gitpkg.WriteTemplateBranchInput{
		Branch:             preview.TargetBranch,
		AllowCurrentBranch: input.AllowCurrentBranch && preview.TargetBranch == currentBranch,
		Overwrite:          input.Overwrite,
		ParentCommit:       input.ParentCommit,
		Message:            fmt.Sprintf("orbit template save %s", preview.OrbitID),
		ManifestPath:       branchManifestPath,
		Manifest:           branchManifest,
		Files:              files,
	})
	if err != nil {
		return TemplateSaveResult{}, fmt.Errorf("write template branch: %w", err)
	}

	return TemplateSaveResult{
		Preview:     preview,
		WriteResult: writeResult,
	}, nil
}

func buildTemplateSaveManifest(
	orbitID string,
	defaultTemplate bool,
	createdFromBranch string,
	createdFromCommit string,
	createdAt time.Time,
	runtimeBindings map[string]bindings.VariableBinding,
	files []CandidateFile,
) (Manifest, error) {
	companionPath, _, err := templateCompanionPaths(orbitID)
	if err != nil {
		return Manifest{}, fmt.Errorf("build companion path: %w", err)
	}

	scanFiles := make([]CandidateFile, 0, len(files))
	for _, file := range files {
		if file.Path != companionPath {
			scanFiles = append(scanFiles, file)
		}
	}
	agentsCandidate, hasAgentsCandidate, err := companionAgentsCandidate(files, orbitID)
	if err != nil {
		return Manifest{}, fmt.Errorf("build companion agents candidate: %w", err)
	}
	if hasAgentsCandidate {
		scanFiles = append(scanFiles, agentsCandidate)
	}

	scanResult := ScanVariables(scanFiles, nil)
	variables := make(map[string]VariableSpec, len(scanResult.Referenced))
	for _, name := range scanResult.Referenced {
		variables[name] = VariableSpec{
			Description: runtimeBindings[name].Description,
			Required:    true,
		}
	}

	manifest := Manifest{
		SchemaVersion: manifestSchemaVersion,
		Kind:          TemplateKind,
		Template: Metadata{
			OrbitID:           orbitID,
			DefaultTemplate:   defaultTemplate,
			CreatedFromBranch: createdFromBranch,
			CreatedFromCommit: createdFromCommit,
			CreatedAt:         createdAt,
		},
		Variables: variables,
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, fmt.Errorf("validate manifest: %w", err)
	}

	return manifest, nil
}

func resolveSaveTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}

	return now.UTC()
}
