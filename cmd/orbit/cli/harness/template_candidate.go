package harness

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// TemplateMemberCandidate is one merge-ready harness template candidate derived from one runtime member.
type TemplateMemberCandidate struct {
	OrbitID              string
	Source               string
	Files                []orbittemplate.CandidateFile
	Variables            map[string]TemplateVariableSpec
	ReplacementSummaries []orbittemplate.FileReplacementSummary
	Ambiguities          []orbittemplate.FileReplacementAmbiguity
	Warnings             []string
}

// FilePaths returns the stable candidate file path list.
func (candidate TemplateMemberCandidate) FilePaths() []string {
	paths := make([]string, 0, len(candidate.Files))
	for _, file := range candidate.Files {
		paths = append(paths, file.Path)
	}

	return paths
}

// BuildTemplateMemberCandidate builds one merge-ready harness template candidate from one runtime member.
func BuildTemplateMemberCandidate(ctx context.Context, repoRoot string, member RuntimeMember) (TemplateMemberCandidate, error) {
	return BuildTemplateMemberCandidateWithOptions(ctx, repoRoot, member, false)
}

// BuildTemplateMemberCandidateWithOptions builds one merge-ready harness template candidate
// from one runtime member while honoring caller-selected export overrides.
func BuildTemplateMemberCandidateWithOptions(
	ctx context.Context,
	repoRoot string,
	member RuntimeMember,
	includeCompletedBootstrap bool,
) (TemplateMemberCandidate, error) {
	repoConfig, err := loadTemplateCandidateRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return TemplateMemberCandidate{}, fmt.Errorf("load repository config: %w", err)
	}
	if err := orbit.ValidateRepositoryConfig(repoConfig.Global, repoConfig.Orbits); err != nil {
		return TemplateMemberCandidate{}, fmt.Errorf("validate repository config: %w", err)
	}

	definition, found := repoConfig.OrbitByID(member.OrbitID)
	if !found {
		return TemplateMemberCandidate{}, fmt.Errorf("orbit %q not found", member.OrbitID)
	}

	trackedFiles, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return TemplateMemberCandidate{}, fmt.Errorf("load tracked files: %w", err)
	}
	worktreeFiles, err := gitpkg.WorktreeFiles(ctx, repoRoot)
	if err != nil {
		return TemplateMemberCandidate{}, fmt.Errorf("load worktree files: %w", err)
	}
	candidateScopeFiles := sortedUniqueStrings(append(trackedFiles, worktreeFiles...))

	spec, plan, err := orbit.LoadOrbitSpecAndProjectionPlan(ctx, repoRoot, repoConfig, definition.ID, candidateScopeFiles)
	if err != nil {
		return TemplateMemberCandidate{}, fmt.Errorf("resolve orbit export plan: %w", err)
	}
	filteredExport, err := orbittemplate.FilterCompletedBootstrapExportPaths(ctx, orbittemplate.RuntimeExportBootstrapFilterInput{
		RepoRoot:                  repoRoot,
		OrbitID:                   definition.ID,
		Spec:                      spec,
		ExportPaths:               plan.ExportPaths,
		IncludeCompletedBootstrap: includeCompletedBootstrap,
	})
	if err != nil {
		return TemplateMemberCandidate{}, fmt.Errorf("filter runtime bootstrap export paths: %w", err)
	}

	varsFile, err := loadOptionalTemplateCandidateVars(ctx, repoRoot)
	if err != nil {
		return TemplateMemberCandidate{}, fmt.Errorf("load harness vars: %w", err)
	}

	buildResult, err := orbittemplate.BuildTemplateContent(ctx, orbittemplate.BuildInput{
		RepoRoot:             repoRoot,
		OrbitID:              definition.ID,
		UserScope:            filteredExport.ExportPaths,
		RuntimeCompanionPath: firstProjectionPlanCompanionPath(spec, plan),
		Bindings:             varsFile.Variables,
	})
	if err != nil {
		return TemplateMemberCandidate{}, fmt.Errorf("build template content: %w", err)
	}

	variableSpecs, err := buildTemplateCandidateVariableSpecs(definition.ID, buildResult.Files, varsFile.Variables)
	if err != nil {
		return TemplateMemberCandidate{}, err
	}

	return TemplateMemberCandidate{
		OrbitID:              member.OrbitID,
		Source:               member.Source,
		Files:                buildResult.Files,
		Variables:            variableSpecs,
		ReplacementSummaries: buildResult.ReplacementSummaries,
		Ambiguities:          buildResult.Ambiguities,
		Warnings:             append([]string(nil), filteredExport.Warnings...),
	}, nil
}

func firstProjectionPlanCompanionPath(spec orbit.OrbitSpec, plan orbit.ProjectionPlan) string {
	if len(plan.MetaPaths) > 0 {
		return plan.MetaPaths[0]
	}

	if spec.Meta == nil {
		return ""
	}

	return spec.Meta.File
}

func loadOptionalTemplateCandidateVars(ctx context.Context, repoRoot string) (bindings.VarsFile, error) {
	file, err := LoadVarsFileWorktreeOrHEAD(ctx, repoRoot)
	if err == nil {
		return file, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return bindings.VarsFile{
			SchemaVersion: 1,
			Variables:     map[string]bindings.VariableBinding{},
		}, nil
	}

	return bindings.VarsFile{}, err
}

func buildTemplateCandidateVariableSpecs(
	orbitID string,
	files []orbittemplate.CandidateFile,
	runtimeBindings map[string]bindings.VariableBinding,
) (map[string]TemplateVariableSpec, error) {
	hostedCompanionPath, err := orbit.HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return nil, fmt.Errorf("build hosted companion definition path: %w", err)
	}
	legacyCompanionPath, err := orbit.DefinitionRelativePath(orbitID)
	if err != nil {
		return nil, fmt.Errorf("build legacy companion definition path: %w", err)
	}

	scanFiles := make([]orbittemplate.CandidateFile, 0, len(files))
	for _, file := range files {
		if file.Path == hostedCompanionPath || file.Path == legacyCompanionPath {
			continue
		}
		scanFiles = append(scanFiles, file)
	}

	scanResult := orbittemplate.ScanVariables(scanFiles, nil)
	variables := make(map[string]TemplateVariableSpec, len(scanResult.Referenced))
	for _, name := range scanResult.Referenced {
		variables[name] = TemplateVariableSpec{
			Description: runtimeBindings[name].Description,
			Required:    true,
		}
	}

	return variables, nil
}

func loadTemplateCandidateRepositoryConfig(ctx context.Context, repoRoot string) (orbit.RepositoryConfig, error) {
	repoConfig, err := orbit.LoadRuntimeRepositoryConfig(ctx, repoRoot)
	if err == nil && len(repoConfig.Orbits) > 0 {
		return repoConfig, nil
	}

	hasLegacyGlobalConfig := false
	globalConfig, globalErr := orbit.LoadGlobalConfig(ctx, repoRoot)
	if globalErr != nil {
		if !errors.Is(globalErr, os.ErrNotExist) {
			return orbit.RepositoryConfig{}, fmt.Errorf("load global config: %w", globalErr)
		}
		globalConfig = orbit.DefaultGlobalConfig()
	} else {
		hasLegacyGlobalConfig = true
	}

	definitions, discoverErr := orbit.DiscoverHostedDefinitions(ctx, repoRoot)
	if discoverErr != nil {
		if err == nil {
			return repoConfig, nil
		}
		return orbit.RepositoryConfig{}, fmt.Errorf("load hosted orbit definitions: %w", discoverErr)
	}
	if len(definitions) > 0 {
		return orbit.RepositoryConfig{
			Global:                globalConfig,
			Orbits:                definitions,
			HasLegacyGlobalConfig: hasLegacyGlobalConfig,
		}, nil
	}

	if err == nil {
		return repoConfig, nil
	}

	return orbit.RepositoryConfig{}, fmt.Errorf("load runtime repository config: %w", err)
}
