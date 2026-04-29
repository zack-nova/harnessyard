package harness

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const rootAgentsPath = "AGENTS.md"

func isRootGuidancePath(path string) bool {
	switch path {
	case rootAgentsPath, rootHumansPath, rootBootstrapPath:
		return true
	default:
		return false
	}
}

// RootAgentsTemplateResult captures the root AGENTS whole-file lane output for harness template save.
type RootAgentsTemplateResult struct {
	File               *orbittemplate.CandidateFile
	Variables          map[string]TemplateVariableSpec
	ReplacementSummary *orbittemplate.FileReplacementSummary
	Ambiguity          *orbittemplate.FileReplacementAmbiguity
	IncludesRootAgents bool
}

type rootGuidanceTemplateFileResult struct {
	File               *orbittemplate.CandidateFile
	Variables          map[string]TemplateVariableSpec
	ReplacementSummary *orbittemplate.FileReplacementSummary
	Ambiguity          *orbittemplate.FileReplacementAmbiguity
}

type rootGuidanceTemplateResult struct {
	Files                []orbittemplate.CandidateFile
	Variables            map[string]TemplateVariableSpec
	ReplacementSummaries []orbittemplate.FileReplacementSummary
	Ambiguities          []orbittemplate.FileReplacementAmbiguity
}

// BuildRootAgentsTemplateFile builds the root AGENTS whole-file candidate for harness template save.
func BuildRootAgentsTemplateFile(ctx context.Context, repoRoot string) (RootAgentsTemplateResult, error) {
	result, err := buildRootGuidanceTemplateFile(ctx, repoRoot, rootAgentsPath)
	if err != nil {
		return RootAgentsTemplateResult{}, err
	}

	return RootAgentsTemplateResult{
		File:               result.File,
		Variables:          result.Variables,
		ReplacementSummary: result.ReplacementSummary,
		Ambiguity:          result.Ambiguity,
		IncludesRootAgents: result.File != nil,
	}, nil
}

func buildRootGuidanceTemplateFiles(ctx context.Context, repoRoot string, runtimeFile RuntimeFile, includeBootstrap bool) (rootGuidanceTemplateResult, error) {
	targetPaths := []string{rootAgentsPath, rootHumansPath}
	includeBootstrapFile, err := shouldIncludeRootBootstrapTemplateFile(ctx, repoRoot, runtimeFile, includeBootstrap)
	if err != nil {
		return rootGuidanceTemplateResult{}, err
	}
	if includeBootstrapFile {
		targetPaths = append(targetPaths, rootBootstrapPath)
	}
	result := rootGuidanceTemplateResult{
		Files:     make([]orbittemplate.CandidateFile, 0, len(targetPaths)),
		Variables: map[string]TemplateVariableSpec{},
	}

	for _, path := range targetPaths {
		fileResult, err := buildRootGuidanceTemplateFile(ctx, repoRoot, path)
		if err != nil {
			return rootGuidanceTemplateResult{}, err
		}
		if fileResult.File == nil {
			continue
		}
		result.Files = append(result.Files, *fileResult.File)
		for name, spec := range fileResult.Variables {
			current, ok := result.Variables[name]
			if !ok {
				result.Variables[name] = spec
				continue
			}
			merged, err := mergeTemplateVariableSpec(name, current, spec)
			if err != nil {
				return rootGuidanceTemplateResult{}, err
			}
			result.Variables[name] = merged
		}
		if fileResult.ReplacementSummary != nil {
			result.ReplacementSummaries = append(result.ReplacementSummaries, *fileResult.ReplacementSummary)
		}
		if fileResult.Ambiguity != nil {
			result.Ambiguities = append(result.Ambiguities, *fileResult.Ambiguity)
		}
	}

	return result, nil
}

func shouldIncludeRootBootstrapTemplateFile(ctx context.Context, repoRoot string, runtimeFile RuntimeFile, includeBootstrap bool) (bool, error) {
	if includeBootstrap {
		return true, nil
	}

	repo, err := gitpkg.DiscoverRepo(ctx, repoRoot)
	if err != nil {
		return false, fmt.Errorf("discover repository git dir: %w", err)
	}

	for _, member := range runtimeFile.Members {
		status, err := orbittemplate.InspectBootstrapOrbit(ctx, repoRoot, repo.GitDir, member.OrbitID)
		if err != nil {
			return false, fmt.Errorf("inspect bootstrap state for orbit %q: %w", member.OrbitID, err)
		}
		if orbittemplate.PlanBootstrapCompose(status).Action == orbittemplate.BootstrapActionAllow {
			return true, nil
		}
	}

	return false, nil
}

func buildRootGuidanceTemplateFile(ctx context.Context, repoRoot string, repoPath string) (rootGuidanceTemplateFileResult, error) {
	varsFile, err := loadOptionalTemplateCandidateVars(ctx, repoRoot)
	if err != nil {
		return rootGuidanceTemplateFileResult{}, fmt.Errorf("load harness vars: %w", err)
	}

	data, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(ctx, repoRoot, repoPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return rootGuidanceTemplateFileResult{
				Variables: map[string]TemplateVariableSpec{},
			}, nil
		}
		return rootGuidanceTemplateFileResult{}, fmt.Errorf("read root %s: %w", repoPath, err)
	}

	mode, err := gitpkg.TrackedFileModeWorktreeOrHEAD(ctx, repoRoot, repoPath)
	if err != nil {
		return rootGuidanceTemplateFileResult{}, fmt.Errorf("read root %s mode: %w", repoPath, err)
	}
	data, err = orbittemplate.NormalizeRuntimeAgentsPayload(data)
	if err != nil {
		return rootGuidanceTemplateFileResult{}, fmt.Errorf("normalize root %s: %w", repoPath, err)
	}

	candidate := orbittemplate.CandidateFile{
		Path:    repoPath,
		Content: data,
		Mode:    mode,
	}
	replaced, err := orbittemplate.ReplaceRuntimeValues(candidate, varsFile.Variables)
	if err != nil {
		return rootGuidanceTemplateFileResult{}, fmt.Errorf("replace runtime values for %s: %w", candidate.Path, err)
	}

	renderedCandidate := &orbittemplate.CandidateFile{
		Path:    candidate.Path,
		Content: replaced.Content,
		Mode:    candidate.Mode,
	}

	var replacementSummary *orbittemplate.FileReplacementSummary
	if len(replaced.Replacements) > 0 {
		replacementSummary = &orbittemplate.FileReplacementSummary{
			Path:         candidate.Path,
			Replacements: replaced.Replacements,
		}
	}

	var ambiguity *orbittemplate.FileReplacementAmbiguity
	if len(replaced.Ambiguities) > 0 {
		ambiguity = &orbittemplate.FileReplacementAmbiguity{
			Path:        candidate.Path,
			Ambiguities: replaced.Ambiguities,
		}
	}

	variableSpecs := buildRootGuidanceVariableSpecs(*renderedCandidate, varsFile.Variables)

	return rootGuidanceTemplateFileResult{
		File:               renderedCandidate,
		Variables:          variableSpecs,
		ReplacementSummary: replacementSummary,
		Ambiguity:          ambiguity,
	}, nil
}

func buildRootGuidanceVariableSpecs(
	file orbittemplate.CandidateFile,
	runtimeBindings map[string]bindings.VariableBinding,
) map[string]TemplateVariableSpec {
	scanResult := orbittemplate.ScanVariables([]orbittemplate.CandidateFile{file}, nil)
	variables := make(map[string]TemplateVariableSpec, len(scanResult.Referenced))
	for _, name := range scanResult.Referenced {
		variables[name] = TemplateVariableSpec{
			Description: runtimeBindings[name].Description,
			Required:    true,
		}
	}

	return variables
}
