package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

const (
	RuntimeBindingExplainStatusResolved   = "resolved"
	RuntimeBindingExplainStatusUnresolved = "unresolved"
)

// RuntimeBindingExplainInput selects one Package Variable for explanation.
type RuntimeBindingExplainInput struct {
	RepoRoot  string
	Name      string
	LookupEnv func(string) (string, bool)
	ReadFile  func(string) ([]byte, error)
}

// RuntimeBindingExplainResult reports declaration facts and selected resolution.
type RuntimeBindingExplainResult struct {
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	ValueSource     string   `json:"value_source,omitempty"`
	Value           string   `json:"value,omitempty"`
	Required        bool     `json:"required"`
	Sensitive       bool     `json:"sensitive"`
	SelectedScope   string   `json:"selected_scope,omitempty"`
	DeclaringOrbits []string `json:"declaring_orbits"`
}

// ExplainRuntimeBinding reports the P0 selected resolution and declaration facts for one variable.
func ExplainRuntimeBinding(ctx context.Context, input RuntimeBindingExplainInput) (RuntimeBindingExplainResult, error) {
	_ = ctx

	result := RuntimeBindingExplainResult{
		Name:            input.Name,
		Status:          RuntimeBindingExplainStatusUnresolved,
		DeclaringOrbits: []string{},
	}

	varsFile, err := LoadVarsFile(input.RepoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			varsFile = bindings.VarsFile{
				SchemaVersion: bindings.VarsSchemaVersion,
				Variables:     map[string]bindings.VariableBinding{},
			}
		} else {
			return RuntimeBindingExplainResult{}, fmt.Errorf("load Runtime Bindings file: %w", err)
		}
	}

	declarations, err := loadRuntimeBindingDeclarations(input.RepoRoot)
	if err != nil {
		return RuntimeBindingExplainResult{}, err
	}

	var selected *bindings.RuntimeBindingResolution
	selectedRank := 999
	declaring := map[string]bool{}
	for _, item := range declarations {
		if item.Name != input.Name {
			continue
		}
		declaring[item.OrbitID] = true
		result.Required = result.Required || item.Declaration.Required
		result.Sensitive = result.Sensitive || item.Declaration.Sensitive

		resolution, err := bindings.ResolveRuntimeBinding(bindings.RuntimeBindingInput{
			Name:          item.Name,
			SelectedScope: item.OrbitID,
			Declaration:   item.Declaration,
			VarsFile:      varsFile,
			FileRoot:      input.RepoRoot,
			LookupEnv:     input.LookupEnv,
			ReadFile:      input.ReadFile,
		})
		if err != nil {
			return RuntimeBindingExplainResult{}, fmt.Errorf("resolve runtime binding %q for %q: %w", item.Name, item.OrbitID, err)
		}
		rank := runtimeBindingExplainRank(resolution)
		if rank < selectedRank {
			selectedRank = rank
			next := resolution
			selected = &next
		}
	}

	result.DeclaringOrbits = sortedRuntimeBindingOrbitIDs(declaring)
	if selected == nil {
		return result, nil
	}

	result.ValueSource = selected.ValueSource
	result.SelectedScope = selected.SelectedScope
	if selected.Resolved {
		result.Status = RuntimeBindingExplainStatusResolved
		result.Value = bindings.RedactRuntimeBindingValue(selected.Value, result.Sensitive)
	}

	return result, nil
}

func runtimeBindingExplainRank(resolution bindings.RuntimeBindingResolution) int {
	if !resolution.Resolved {
		return 100
	}
	switch resolution.Source {
	case bindings.RuntimeBindingSourceScoped:
		return 0
	case bindings.RuntimeBindingSourceGlobal:
		return 1
	case bindings.RuntimeBindingSourceDefault:
		return 2
	default:
		return 50
	}
}

func sortedRuntimeBindingOrbitIDs(values map[string]bool) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		if strings.TrimSpace(id) == "" {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
