package orbittemplate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

type renderTemplateOptions struct {
	AllowUnresolved bool
	Declared        map[string]bindings.VariableDeclaration
}

var packageTemplateReferenceIdentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// RenderTemplateFiles renders Package Template References in text files using
// the resolved bindings. Binary and invalid UTF-8 files are passed through unchanged.
func RenderTemplateFiles(files []CandidateFile, bindings map[string]string) ([]CandidateFile, error) {
	return renderTemplateFilesWithOptions(files, bindings, renderTemplateOptions{})
}

// RenderTemplateFilesWithDeclarations renders Package Template References in text
// files while validating references against the template variable declarations.
func RenderTemplateFilesWithDeclarations(
	files []CandidateFile,
	bindings map[string]string,
	declared map[string]bindings.VariableDeclaration,
) ([]CandidateFile, error) {
	return renderTemplateFilesWithOptions(files, bindings, renderTemplateOptions{
		Declared: declared,
	})
}

// RenderTemplateFilesAllowingUnresolved renders known Package Template References
// while preserving unresolved declared references in-place.
func RenderTemplateFilesAllowingUnresolved(files []CandidateFile, bindings map[string]string) ([]CandidateFile, error) {
	return renderTemplateFilesWithOptions(files, bindings, renderTemplateOptions{
		AllowUnresolved: true,
	})
}

// RenderTemplateFilesWithDeclarationsAllowingUnresolved renders known Package
// Template References and preserves unresolved declared references in-place.
func RenderTemplateFilesWithDeclarationsAllowingUnresolved(
	files []CandidateFile,
	bindings map[string]string,
	declared map[string]bindings.VariableDeclaration,
) ([]CandidateFile, error) {
	return renderTemplateFilesWithOptions(files, bindings, renderTemplateOptions{
		AllowUnresolved: true,
		Declared:        declared,
	})
}

func renderTemplateFilesWithOptions(files []CandidateFile, bindings map[string]string, options renderTemplateOptions) ([]CandidateFile, error) {
	rendered := make([]CandidateFile, 0, len(files))
	missingSet := make(map[string]struct{})
	declaredSet := renderDeclaredSet(bindings, options)

	for _, file := range files {
		if isBinaryOrInvalidText(file.Content) {
			rendered = append(rendered, CandidateFile{
				Path:    file.Path,
				Content: append([]byte(nil), file.Content...),
				Mode:    file.Mode,
			})
			continue
		}

		content, err := renderTemplateText(file.Path, string(file.Content), bindings, declaredSet, options, missingSet)
		if err != nil {
			return nil, err
		}

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

func renderDeclaredSet(values map[string]string, options renderTemplateOptions) map[string]struct{} {
	if options.Declared != nil {
		set := make(map[string]struct{}, len(options.Declared))
		for name := range options.Declared {
			set[name] = struct{}{}
		}
		return set
	}
	if options.AllowUnresolved {
		return nil
	}

	set := make(map[string]struct{}, len(values))
	for name := range values {
		set[name] = struct{}{}
	}
	return set
}

func renderTemplateText(
	path string,
	text string,
	values map[string]string,
	declared map[string]struct{},
	options renderTemplateOptions,
	missingSet map[string]struct{},
) (string, error) {
	var output strings.Builder
	cursor := 0
	for {
		startOffset := strings.Index(text[cursor:], "{{")
		if startOffset < 0 {
			output.WriteString(text[cursor:])
			return output.String(), nil
		}
		start := cursor + startOffset
		if start > 0 && text[start-1] == '$' {
			output.WriteString(text[cursor : start+2])
			cursor = start + 2
			continue
		}

		endOffset := strings.Index(text[start+2:], "}}")
		if endOffset < 0 {
			return "", malformedPackageTemplateReferenceError(path, text[start:])
		}
		end := start + 2 + endOffset
		raw := text[start : end+2]
		ref, err := parsePackageTemplateReference(strings.TrimSpace(text[start+2 : end]))
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}

		output.WriteString(text[cursor:start])
		if ref.Namespace != "vars" {
			return "", fmt.Errorf("%s: unsupported Package Template Reference namespace %q in %s", path, ref.Namespace, raw)
		}
		if declared != nil {
			if _, ok := declared[ref.Name]; !ok {
				return "", fmt.Errorf("%s: unknown Package Variable %q in %s", path, ref.Name, raw)
			}
		}
		value, ok := values[ref.Name]
		if !ok {
			missingSet[ref.Name] = struct{}{}
			if options.AllowUnresolved {
				output.WriteString(raw)
				cursor = end + 2
				continue
			}
			return "", fmt.Errorf("%s: unresolved Package Variable %q in %s", path, ref.Name, raw)
		}

		output.WriteString(value)
		cursor = end + 2
	}
}

type packageTemplateReference struct {
	Namespace string
	Name      string
}

func parsePackageTemplateReference(expr string) (packageTemplateReference, error) {
	parts := strings.Split(expr, ".")
	if len(parts) != 2 ||
		!packageTemplateReferenceIdentPattern.MatchString(parts[0]) ||
		!packageTemplateReferenceIdentPattern.MatchString(parts[1]) {
		return packageTemplateReference{}, fmt.Errorf("malformed Package Template Reference %q", "{{ "+expr+" }}")
	}

	return packageTemplateReference{
		Namespace: parts[0],
		Name:      parts[1],
	}, nil
}

func malformedPackageTemplateReferenceError(path string, raw string) error {
	snippet := strings.TrimSpace(raw)
	if len(snippet) > 80 {
		snippet = snippet[:80] + "..."
	}
	return fmt.Errorf("%s: malformed Package Template Reference %q", path, snippet)
}
