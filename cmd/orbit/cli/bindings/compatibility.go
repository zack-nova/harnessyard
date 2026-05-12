package bindings

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// DeclarationConflictError reports one conflicting variable declaration across multiple sources.
type DeclarationConflictError struct {
	Name    string
	Sources []string
}

func (err *DeclarationConflictError) Error() string {
	return fmt.Sprintf("variable conflict for %q (sources: %s)", err.Name, strings.Join(err.Sources, ", "))
}

// ValidateVariableDeclaration applies declaration-local Package Variable policy.
func ValidateVariableDeclaration(declaration VariableDeclaration) error {
	if declaration.Sensitive && declaration.Default != nil {
		return fmt.Errorf("sensitive variables must not define default")
	}

	return nil
}

// MergeVariableDeclaration applies the shared compatibility policy for one variable declaration.
func MergeVariableDeclaration(name string, current VariableDeclaration, next VariableDeclaration) (VariableDeclaration, error) {
	if err := ValidateVariableDeclaration(current); err != nil {
		return VariableDeclaration{}, fmt.Errorf("variable conflict for %q: %w", name, err)
	}
	if err := ValidateVariableDeclaration(next); err != nil {
		return VariableDeclaration{}, fmt.Errorf("variable conflict for %q: %w", name, err)
	}

	switch {
	case current.Description == next.Description:
	case current.Description == "":
		current.Description = next.Description
	case next.Description == "":
	default:
		return VariableDeclaration{}, fmt.Errorf("variable conflict for %q", name)
	}

	current.Required = current.Required || next.Required
	current.Sensitive = current.Sensitive || next.Sensitive
	switch {
	case current.Default == nil:
		current.Default = next.Default
	case next.Default == nil:
	case *current.Default == *next.Default:
	default:
		return VariableDeclaration{}, fmt.Errorf("variable conflict for %q", name)
	}
	if current.Sensitive && current.Default != nil {
		return VariableDeclaration{}, fmt.Errorf("variable conflict for %q: %w", name, ValidateVariableDeclaration(current))
	}

	return current, nil
}

// MergeDeclarations folds one source's variable declarations into an aggregated compatibility view.
func MergeDeclarations(
	merged map[string]VariableDeclaration,
	contributors map[string][]string,
	next map[string]VariableDeclaration,
	source string,
) error {
	for _, name := range contractutil.SortedKeys(next) {
		nextDeclaration := next[name]
		if err := ValidateVariableDeclaration(nextDeclaration); err != nil {
			return fmt.Errorf("validate variable %q declaration: %w", name, err)
		}
		currentDeclaration, ok := merged[name]
		if !ok {
			merged[name] = nextDeclaration
			contributors[name] = appendDeclarationContributor(contributors[name], source)
			continue
		}

		combined, err := MergeVariableDeclaration(name, currentDeclaration, nextDeclaration)
		if err != nil {
			return &DeclarationConflictError{
				Name:    name,
				Sources: appendDeclarationContributor(contributors[name], source),
			}
		}
		merged[name] = combined
		contributors[name] = appendDeclarationContributor(contributors[name], source)
	}

	return nil
}

func appendDeclarationContributor(existing []string, contributor string) []string {
	trimmed := strings.TrimSpace(contributor)
	if trimmed == "" {
		return existing
	}
	for _, candidate := range existing {
		if candidate == trimmed {
			return existing
		}
	}
	values := append(existing, trimmed)
	sort.Strings(values)
	return values
}
