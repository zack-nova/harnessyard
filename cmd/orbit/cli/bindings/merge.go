package bindings

import (
	"fmt"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// MergeSource identifies which input supplied the resolved value.
type MergeSource string

const (
	SourceBindingsFile       MergeSource = "bindings_file"
	SourceBindingsFileScoped MergeSource = "bindings_file_scoped"
	SourceRepoVars           MergeSource = "repo_vars"
	SourceRepoVarsScoped     MergeSource = "repo_vars_scoped"
	SourceInteractive        MergeSource = "interactive"
	SourceEditor             MergeSource = "editor"
)

// VariableDeclaration is the template manifest metadata needed for merge decisions.
type VariableDeclaration struct {
	Description string
	Required    bool
}

// MergeInput is the pure-data input contract for the bindings merge engine.
type MergeInput struct {
	Declared            map[string]VariableDeclaration
	BindingsFile        map[string]VariableBinding
	BindingsFileScoped  map[string]VariableBinding
	RepoVars            map[string]VariableBinding
	RepoVarsScoped      map[string]VariableBinding
	FillIn              map[string]VariableBinding
	FillSource          MergeSource
	Namespace           string
	NamespaceByVariable map[string]string
}

// ResolvedBinding is one merged binding plus its preserved metadata and source.
type ResolvedBinding struct {
	Value       string
	Description string
	Required    bool
	Source      MergeSource
	Namespace   string
}

// UnresolvedBinding is a required variable that still needs a value.
type UnresolvedBinding struct {
	Name        string
	Description string
	Required    bool
	Namespace   string
}

// MergeResult contains the resolved bindings and any still-missing required variables.
type MergeResult struct {
	Resolved   map[string]ResolvedBinding
	Unresolved []UnresolvedBinding
}

// Merge combines declared variables with the documented precedence:
// scoped bindings file > scoped repo vars > bindings file > repo vars > interactive/editor fill.
func Merge(input MergeInput) (MergeResult, error) {
	fillSource, err := normalizeFillSource(input.FillSource, len(input.FillIn) > 0)
	if err != nil {
		return MergeResult{}, err
	}

	result := MergeResult{
		Resolved: make(map[string]ResolvedBinding),
	}

	for _, name := range contractutil.SortedKeys(input.Declared) {
		declaration := input.Declared[name]
		description := firstNonEmpty(
			declaration.Description,
			input.BindingsFileScoped[name].Description,
			input.RepoVarsScoped[name].Description,
			input.BindingsFile[name].Description,
			input.RepoVars[name].Description,
			input.FillIn[name].Description,
		)
		namespace := mergeNamespaceForVariable(input, name)

		switch {
		case hasBinding(input.BindingsFileScoped, name):
			binding := input.BindingsFileScoped[name]
			result.Resolved[name] = ResolvedBinding{
				Value:       binding.Value,
				Description: description,
				Required:    declaration.Required,
				Source:      SourceBindingsFileScoped,
				Namespace:   input.Namespace,
			}
		case hasBinding(input.RepoVarsScoped, name):
			binding := input.RepoVarsScoped[name]
			result.Resolved[name] = ResolvedBinding{
				Value:       binding.Value,
				Description: description,
				Required:    declaration.Required,
				Source:      SourceRepoVarsScoped,
				Namespace:   input.Namespace,
			}
		case hasBinding(input.BindingsFile, name):
			binding := input.BindingsFile[name]
			result.Resolved[name] = ResolvedBinding{
				Value:       binding.Value,
				Description: description,
				Required:    declaration.Required,
				Source:      SourceBindingsFile,
				Namespace:   namespace,
			}
		case hasBinding(input.RepoVars, name):
			binding := input.RepoVars[name]
			result.Resolved[name] = ResolvedBinding{
				Value:       binding.Value,
				Description: description,
				Required:    declaration.Required,
				Source:      SourceRepoVars,
				Namespace:   namespace,
			}
		case hasBinding(input.FillIn, name):
			binding := input.FillIn[name]
			result.Resolved[name] = ResolvedBinding{
				Value:       binding.Value,
				Description: description,
				Required:    declaration.Required,
				Source:      fillSource,
				Namespace:   namespace,
			}
		case declaration.Required:
			result.Unresolved = append(result.Unresolved, UnresolvedBinding{
				Name:        name,
				Description: description,
				Required:    true,
				Namespace:   namespace,
			})
		}
	}

	return result, nil
}

func mergeNamespaceForVariable(input MergeInput, name string) string {
	if input.NamespaceByVariable != nil {
		if namespace := strings.TrimSpace(input.NamespaceByVariable[name]); namespace != "" {
			return namespace
		}
	}
	return ""
}

func normalizeFillSource(source MergeSource, hasFill bool) (MergeSource, error) {
	if !hasFill {
		return source, nil
	}

	switch source {
	case "":
		return SourceInteractive, nil
	case SourceInteractive, SourceEditor:
		return source, nil
	default:
		return "", fmt.Errorf("fill source must be %q or %q", SourceInteractive, SourceEditor)
	}
}

func hasBinding(values map[string]VariableBinding, name string) bool {
	if values == nil {
		return false
	}

	binding, ok := values[name]
	return ok && strings.TrimSpace(binding.Value) != ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
