package bindings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeBindingSource identifies the selected source category for one runtime variable.
type RuntimeBindingSource string

const (
	RuntimeBindingSourceScoped     RuntimeBindingSource = "scoped"
	RuntimeBindingSourceGlobal     RuntimeBindingSource = "global"
	RuntimeBindingSourceDefault    RuntimeBindingSource = "default"
	RuntimeBindingSourceUnresolved RuntimeBindingSource = "unresolved"
)

// RuntimeBindingInput describes one selected Package Variable resolution.
type RuntimeBindingInput struct {
	Name          string
	SelectedScope string
	Declaration   VariableDeclaration
	VarsFile      VarsFile
	FileRoot      string
	LookupEnv     func(string) (string, bool)
	ReadFile      func(string) ([]byte, error)
}

// RuntimeBindingResolution reports the selected value and source for one variable.
type RuntimeBindingResolution struct {
	Name          string
	Value         string
	ValueSource   string
	Source        RuntimeBindingSource
	SelectedScope string
	Required      bool
	Sensitive     bool
	Resolved      bool
}

// ResolveRuntimeBinding selects a Runtime Binding using public P0 precedence:
// scoped Runtime Binding, global Runtime Binding, declaration default, unresolved.
func ResolveRuntimeBinding(input RuntimeBindingInput) (RuntimeBindingResolution, error) {
	result := RuntimeBindingResolution{
		Name:      input.Name,
		Required:  input.Declaration.Required,
		Sensitive: input.Declaration.Sensitive,
	}

	if strings.TrimSpace(input.SelectedScope) != "" {
		if binding, ok := ScopedVariablesForNamespace(input.VarsFile, input.SelectedScope)[input.Name]; ok {
			result.Source = RuntimeBindingSourceScoped
			result.SelectedScope = input.SelectedScope
			return resolveRuntimeBindingValue(input, result, binding)
		}
	}

	if binding, ok := input.VarsFile.Variables[input.Name]; ok {
		result.Source = RuntimeBindingSourceGlobal
		result.SelectedScope = "global"
		return resolveRuntimeBindingValue(input, result, binding)
	}

	if input.Declaration.Default != nil {
		result.Value = *input.Declaration.Default
		result.ValueSource = "default"
		result.Source = RuntimeBindingSourceDefault
		result.SelectedScope = "default"
		result.Resolved = true
		return result, nil
	}

	result.Source = RuntimeBindingSourceUnresolved
	return result, nil
}

func resolveRuntimeBindingValue(
	input RuntimeBindingInput,
	result RuntimeBindingResolution,
	binding VariableBinding,
) (RuntimeBindingResolution, error) {
	if binding.ValueFrom == nil {
		if input.Declaration.Sensitive {
			return RuntimeBindingResolution{}, fmt.Errorf("%s: sensitive variables must use value_from.env", input.Name)
		}
		result.Value = binding.Value
		result.ValueSource = ".harness/vars.yaml"
		result.Resolved = true
		return result, nil
	}

	source := *binding.ValueFrom
	switch {
	case strings.TrimSpace(source.Env) != "":
		result.ValueSource = "env:" + source.Env
		lookupEnv := input.LookupEnv
		if lookupEnv == nil {
			lookupEnv = os.LookupEnv
		}
		value, ok := lookupEnv(source.Env)
		if !ok {
			result.Resolved = false
			return result, nil
		}
		result.Value = value
		result.Resolved = true
		return result, nil
	case strings.TrimSpace(source.File) != "":
		result.ValueSource = "file:" + source.File
		if input.Declaration.Sensitive {
			return RuntimeBindingResolution{}, fmt.Errorf("%s: sensitive variables must not use value_from.file", input.Name)
		}
		readFile := input.ReadFile
		if readFile == nil {
			readFile = os.ReadFile
		}
		readPath := source.File
		if input.FileRoot != "" && !filepath.IsAbs(readPath) {
			readPath = filepath.Join(input.FileRoot, filepath.FromSlash(readPath))
		}
		data, err := readFile(readPath)
		if err != nil {
			if os.IsNotExist(err) {
				result.Resolved = false
				return result, nil
			}
			return RuntimeBindingResolution{}, fmt.Errorf("%s: read value_from.file %s: %w", input.Name, source.File, err)
		}
		result.Value = string(data)
		result.Resolved = true
		return result, nil
	default:
		return RuntimeBindingResolution{}, fmt.Errorf("%s: value_from must set env or file", input.Name)
	}
}
