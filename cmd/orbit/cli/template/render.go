package orbittemplate

import (
	"fmt"
	"sort"
	"strings"
)

type renderTemplateOptions struct {
	AllowUnresolved bool
}

// RenderTemplateFiles renders template variables in Markdown files using the
// resolved bindings. Non-Markdown files are passed through unchanged.
func RenderTemplateFiles(files []CandidateFile, bindings map[string]string) ([]CandidateFile, error) {
	return renderTemplateFilesWithOptions(files, bindings, renderTemplateOptions{})
}

// RenderTemplateFilesAllowingUnresolved renders known template variables while
// preserving unresolved references in-place.
func RenderTemplateFilesAllowingUnresolved(files []CandidateFile, bindings map[string]string) ([]CandidateFile, error) {
	return renderTemplateFilesWithOptions(files, bindings, renderTemplateOptions{
		AllowUnresolved: true,
	})
}

func renderTemplateFilesWithOptions(files []CandidateFile, bindings map[string]string, options renderTemplateOptions) ([]CandidateFile, error) {
	rendered := make([]CandidateFile, 0, len(files))
	missingSet := make(map[string]struct{})

	for _, file := range files {
		if !isMarkdownTemplateFile(file.Path) || isBinaryOrInvalidText(file.Content) {
			rendered = append(rendered, CandidateFile{
				Path:    file.Path,
				Content: append([]byte(nil), file.Content...),
				Mode:    file.Mode,
			})
			continue
		}

		content := variableReferencePattern.ReplaceAllStringFunc(string(file.Content), func(match string) string {
			name := match[1:]
			value, ok := bindings[name]
			if !ok {
				missingSet[name] = struct{}{}
				return match
			}

			return value
		})

		rendered = append(rendered, CandidateFile{
			Path:    file.Path,
			Content: []byte(content),
			Mode:    file.Mode,
		})
	}

	if len(missingSet) > 0 && !options.AllowUnresolved {
		missing := make([]string, 0, len(missingSet))
		for name := range missingSet {
			missing = append(missing, name)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("missing binding for %s", strings.Join(missing, ", "))
	}

	return rendered, nil
}
