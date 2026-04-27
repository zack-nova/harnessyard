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

// RootAgentsTemplateResult captures the root AGENTS whole-file lane output for harness template save.
type RootAgentsTemplateResult struct {
	File               *orbittemplate.CandidateFile
	Variables          map[string]TemplateVariableSpec
	ReplacementSummary *orbittemplate.FileReplacementSummary
	Ambiguity          *orbittemplate.FileReplacementAmbiguity
	IncludesRootAgents bool
}

// BuildRootAgentsTemplateFile builds the root AGENTS whole-file candidate for harness template save.
func BuildRootAgentsTemplateFile(ctx context.Context, repoRoot string) (RootAgentsTemplateResult, error) {
	varsFile, err := loadOptionalTemplateCandidateVars(ctx, repoRoot)
	if err != nil {
		return RootAgentsTemplateResult{}, fmt.Errorf("load harness vars: %w", err)
	}

	data, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(ctx, repoRoot, rootAgentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RootAgentsTemplateResult{
				Variables: map[string]TemplateVariableSpec{},
			}, nil
		}
		return RootAgentsTemplateResult{}, fmt.Errorf("read root AGENTS.md: %w", err)
	}

	mode, err := gitpkg.TrackedFileModeWorktreeOrHEAD(ctx, repoRoot, rootAgentsPath)
	if err != nil {
		return RootAgentsTemplateResult{}, fmt.Errorf("read root AGENTS.md mode: %w", err)
	}
	data, err = orbittemplate.NormalizeRuntimeAgentsPayload(data)
	if err != nil {
		return RootAgentsTemplateResult{}, fmt.Errorf("normalize root AGENTS.md: %w", err)
	}

	candidate := orbittemplate.CandidateFile{
		Path:    rootAgentsPath,
		Content: data,
		Mode:    mode,
	}
	replaced, err := orbittemplate.ReplaceRuntimeValues(candidate, varsFile.Variables)
	if err != nil {
		return RootAgentsTemplateResult{}, fmt.Errorf("replace runtime values for %s: %w", candidate.Path, err)
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

	variableSpecs := buildRootAgentsVariableSpecs(*renderedCandidate, varsFile.Variables)

	return RootAgentsTemplateResult{
		File:               renderedCandidate,
		Variables:          variableSpecs,
		ReplacementSummary: replacementSummary,
		Ambiguity:          ambiguity,
		IncludesRootAgents: true,
	}, nil
}

func buildRootAgentsVariableSpecs(
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
