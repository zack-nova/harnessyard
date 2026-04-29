package orbittemplate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// BuildInput is the pure-data input contract for the template content builder.
type BuildInput struct {
	RepoRoot             string
	OrbitID              string
	UserScope            []string
	RuntimeCompanionPath string
	Bindings             map[string]bindings.VariableBinding
}

// FileReplacementSummary groups one file path with its replacement summary.
type FileReplacementSummary struct {
	Path         string
	Replacements []ReplacementSummary
}

// FileReplacementAmbiguity groups one file path with its ambiguities.
type FileReplacementAmbiguity struct {
	Path        string
	Ambiguities []ReplacementAmbiguity
}

// BuildResult contains the candidate template file tree plus replacement metadata.
type BuildResult struct {
	Files                []CandidateFile
	ReplacementSummaries []FileReplacementSummary
	Ambiguities          []FileReplacementAmbiguity
}

// BuildTemplateContent builds the candidate template tree from runtime repo content.
func BuildTemplateContent(ctx context.Context, input BuildInput) (BuildResult, error) {
	companionPath, legacyCompanionPath, err := templateCompanionPaths(input.OrbitID)
	if err != nil {
		return BuildResult{}, fmt.Errorf("build companion definition path: %w", err)
	}
	runtimeCompanionPath, err := runtimeCompanionPath(input.OrbitID, input.RuntimeCompanionPath, input.UserScope)
	if err != nil {
		return BuildResult{}, fmt.Errorf("resolve runtime companion definition path: %w", err)
	}

	paths := make(map[string]struct{}, len(input.UserScope)+1)
	for _, rawPath := range input.UserScope {
		normalizedPath, err := ids.NormalizeRepoRelativePath(rawPath)
		if err != nil {
			return BuildResult{}, fmt.Errorf("normalize user scope path %q: %w", rawPath, err)
		}
		if normalizedPath == companionPath || normalizedPath == legacyCompanionPath || normalizedPath == runtimeCompanionPath {
			continue
		}
		if isForbiddenTemplatePath(normalizedPath) {
			continue
		}
		paths[normalizedPath] = struct{}{}
	}
	paths[companionPath] = struct{}{}

	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	result := BuildResult{
		Files: make([]CandidateFile, 0, len(sortedPaths)),
	}

	for _, path := range sortedPaths {
		readPath := path
		if path == companionPath {
			readPath = runtimeCompanionPath
		}

		content, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(ctx, input.RepoRoot, readPath)
		if err != nil {
			return BuildResult{}, fmt.Errorf("read candidate file %s: %w", path, err)
		}
		mode, err := gitpkg.TrackedFileModeWorktreeOrHEAD(ctx, input.RepoRoot, readPath)
		if err != nil {
			return BuildResult{}, fmt.Errorf("read candidate file mode %s: %w", path, err)
		}

		candidate := CandidateFile{
			Path:    path,
			Content: content,
			Mode:    mode,
		}
		if path == companionPath {
			candidate.Content, err = rewriteRuntimeCompanionDataForTemplate(input.OrbitID, runtimeCompanionPath, candidate.Content)
			if err != nil {
				return BuildResult{}, fmt.Errorf("rewrite companion definition %s for template source: %w", path, err)
			}
			result.Files = append(result.Files, candidate)
			continue
		}

		replaced, err := ReplaceRuntimeValues(candidate, input.Bindings)
		if err != nil {
			return BuildResult{}, fmt.Errorf("replace runtime values for %s: %w", path, err)
		}

		result.Files = append(result.Files, CandidateFile{
			Path:    path,
			Content: replaced.Content,
			Mode:    mode,
		})
		if len(replaced.Replacements) > 0 {
			result.ReplacementSummaries = append(result.ReplacementSummaries, FileReplacementSummary{
				Path:         path,
				Replacements: replaced.Replacements,
			})
		}
		if len(replaced.Ambiguities) > 0 {
			result.Ambiguities = append(result.Ambiguities, FileReplacementAmbiguity{
				Path:        path,
				Ambiguities: replaced.Ambiguities,
			})
		}
	}

	return result, nil
}

func runtimeCompanionPath(orbitID string, configuredPath string, userScope []string) (string, error) {
	hostedPath, err := orbit.HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return "", fmt.Errorf("build hosted companion path: %w", err)
	}
	legacyPath, err := orbit.DefinitionRelativePath(orbitID)
	if err != nil {
		return "", fmt.Errorf("build legacy companion path: %w", err)
	}
	if strings.TrimSpace(configuredPath) != "" {
		normalizedPath, err := ids.NormalizeRepoRelativePath(configuredPath)
		if err != nil {
			return "", fmt.Errorf("normalize runtime companion path %q: %w", configuredPath, err)
		}
		switch normalizedPath {
		case hostedPath, legacyPath:
			return normalizedPath, nil
		default:
			return "", fmt.Errorf("runtime companion path %q does not match orbit %q", normalizedPath, orbitID)
		}
	}

	for _, rawPath := range userScope {
		normalizedPath, err := ids.NormalizeRepoRelativePath(rawPath)
		if err != nil {
			return "", fmt.Errorf("normalize user scope path %q: %w", rawPath, err)
		}
		switch normalizedPath {
		case hostedPath, legacyPath:
			return normalizedPath, nil
		}
	}

	return legacyPath, nil
}

func isForbiddenTemplatePath(path string) bool {
	switch {
	case path == ".orbit/config.yaml":
		return true
	case path == sourceManifestRelativePath:
		return true
	case path == sharedFilePathAgents:
		return true
	case path == runtimeHumansRepoPath:
		return true
	case path == runtimeBootstrapRepoPath:
		return true
	case strings.HasPrefix(path, ".harness/"):
		return true
	case strings.HasPrefix(path, ".git/orbit/state/"):
		return true
	default:
		return false
	}
}
