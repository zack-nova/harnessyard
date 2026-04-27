package harness

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// BindingsPlanSource describes one template source that contributed to the merged bindings plan.
type BindingsPlanSource struct {
	Kind    string
	Repo    string
	Ref     string
	Commit  string
	OrbitID string
}

// BindingsPlanResult captures the merged shared bindings skeleton for multiple template sources.
type BindingsPlanResult struct {
	Sources         []BindingsPlanSource
	Bindings        bindings.VarsFile
	MissingRequired []string
	ReusedValues    []string
}

type bindingsPlanDeclaration struct {
	OrbitID     string
	Declaration bindings.VariableDeclaration
}

// BuildBindingsPlan merges multiple bindings-init previews into one shared runtime bindings skeleton.
func BuildBindingsPlan(
	previews []orbittemplate.BindingsInitPreview,
	repoVars bindings.VarsFile,
) (BindingsPlanResult, error) {
	result := BindingsPlanResult{
		Sources:  make([]BindingsPlanSource, 0, len(previews)),
		Bindings: cloneBindingsPlanVarsFile(repoVars),
	}

	if result.Bindings.SchemaVersion == 0 {
		result.Bindings.SchemaVersion = 1
	}
	if result.Bindings.Variables == nil {
		result.Bindings.Variables = map[string]bindings.VariableBinding{}
	}

	declarationsByName := make(map[string][]bindingsPlanDeclaration)

	for _, preview := range previews {
		result.Sources = append(result.Sources, BindingsPlanSource{
			Kind:    preview.Source.SourceKind,
			Repo:    preview.Source.SourceRepo,
			Ref:     preview.Source.SourceRef,
			Commit:  preview.Source.TemplateCommit,
			OrbitID: preview.Manifest.Template.OrbitID,
		})

		for _, name := range sortedVariableNames(preview.Manifest.Variables) {
			next := preview.Manifest.Variables[name]
			declarationsByName[name] = append(declarationsByName[name], bindingsPlanDeclaration{
				OrbitID: preview.Manifest.Template.OrbitID,
				Declaration: bindings.VariableDeclaration{
					Description: next.Description,
					Required:    next.Required,
				},
			})
		}
	}

	for _, name := range sortedBindingsPlanDeclarationNames(declarationsByName) {
		declarations := declarationsByName[name]
		delete(result.Bindings.Variables, name)
		if bindingsPlanHasDeclarationConflict(name, declarations) {
			appendScopedBindingsPlanValue(&result, repoVars, name, declarations)
			continue
		}

		spec, err := mergeBindingsPlanDeclarations(name, declarations)
		if err != nil {
			return BindingsPlanResult{}, fmt.Errorf("merge shared bindings declarations: %w", err)
		}
		binding := bindings.VariableBinding{
			Value:       "",
			Description: spec.Description,
		}

		if existing, ok := repoVars.Variables[name]; ok && strings.TrimSpace(existing.Value) != "" {
			binding.Value = existing.Value
			result.ReusedValues = append(result.ReusedValues, name)
		}
		if binding.Value == "" && spec.Required {
			result.MissingRequired = append(result.MissingRequired, name)
		}

		result.Bindings.Variables[name] = binding
	}

	return result, nil
}

func cloneBindingsPlanVarsFile(file bindings.VarsFile) bindings.VarsFile {
	cloned := bindings.VarsFile{
		SchemaVersion: file.SchemaVersion,
	}

	if file.Variables != nil {
		cloned.Variables = make(map[string]bindings.VariableBinding, len(file.Variables))
		for name, binding := range file.Variables {
			cloned.Variables[name] = binding
		}
	}

	if file.ScopedVariables != nil {
		cloned.ScopedVariables = make(map[string]bindings.ScopedVariableBindings, len(file.ScopedVariables))
		for namespace, scoped := range file.ScopedVariables {
			clonedScoped := bindings.ScopedVariableBindings{}
			if scoped.Variables != nil {
				clonedScoped.Variables = make(map[string]bindings.VariableBinding, len(scoped.Variables))
				for name, binding := range scoped.Variables {
					clonedScoped.Variables[name] = binding
				}
			}
			cloned.ScopedVariables[namespace] = clonedScoped
		}
	}

	return cloned
}

func appendScopedBindingsPlanValue(
	result *BindingsPlanResult,
	repoVars bindings.VarsFile,
	name string,
	declarations []bindingsPlanDeclaration,
) {
	if result.Bindings.ScopedVariables == nil {
		result.Bindings.ScopedVariables = map[string]bindings.ScopedVariableBindings{}
	}
	for _, item := range declarations {
		namespace := item.OrbitID
		scoped := result.Bindings.ScopedVariables[namespace]
		if scoped.Variables == nil {
			scoped.Variables = map[string]bindings.VariableBinding{}
		}

		binding := bindings.VariableBinding{
			Value:       "",
			Description: item.Declaration.Description,
		}
		if existing, ok := bindings.ScopedVariablesForNamespace(repoVars, namespace)[name]; ok && strings.TrimSpace(existing.Value) != "" {
			binding.Value = existing.Value
			result.ReusedValues = append(result.ReusedValues, namespacedBindingsPlanName(namespace, name))
		} else if existing, ok := repoVars.Variables[name]; ok && strings.TrimSpace(existing.Value) != "" {
			binding.Value = existing.Value
			result.ReusedValues = append(result.ReusedValues, namespacedBindingsPlanName(namespace, name))
		}
		if binding.Value == "" && item.Declaration.Required {
			result.MissingRequired = append(result.MissingRequired, namespacedBindingsPlanName(namespace, name))
		}

		scoped.Variables[name] = binding
		result.Bindings.ScopedVariables[namespace] = scoped
	}
}

func mergeBindingsPlanDeclarations(name string, declarations []bindingsPlanDeclaration) (bindings.VariableDeclaration, error) {
	var merged bindings.VariableDeclaration
	for index, item := range declarations {
		if index == 0 {
			merged = item.Declaration
			continue
		}
		next, err := bindings.MergeVariableDeclaration(name, merged, item.Declaration)
		if err != nil {
			return bindings.VariableDeclaration{}, fmt.Errorf("merge variable %q declaration: %w", name, err)
		}
		merged = next
	}
	return merged, nil
}

func bindingsPlanHasDeclarationConflict(name string, declarations []bindingsPlanDeclaration) bool {
	for left := 0; left < len(declarations); left++ {
		for right := left + 1; right < len(declarations); right++ {
			if _, err := bindings.MergeVariableDeclaration(name, declarations[left].Declaration, declarations[right].Declaration); err != nil {
				return true
			}
		}
	}
	return false
}

func namespacedBindingsPlanName(namespace string, name string) string {
	return fmt.Sprintf("%s:%s", namespace, name)
}

func sortedVariableNames(values map[string]orbittemplate.VariableSpec) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedBindingsPlanDeclarationNames(values map[string][]bindingsPlanDeclaration) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
