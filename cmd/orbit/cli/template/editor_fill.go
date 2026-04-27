package orbittemplate

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

const bindingsEditFilePattern = "orbit-bindings-*.yaml"

func editMissingBindings(
	ctx context.Context,
	unresolved []bindings.UnresolvedBinding,
	editor Editor,
) (map[string]bindings.VariableBinding, error) {
	if editor == nil {
		return nil, fmt.Errorf("editor apply requires an editor")
	}

	declared := make(map[string]bindings.VariableDeclaration, len(unresolved))
	for _, missing := range unresolved {
		declared[missing.Name] = bindings.VariableDeclaration{
			Description: missing.Description,
			Required:    missing.Required,
		}
	}

	skeleton := bindings.SkeletonFromDeclarations(declared)
	data, err := bindings.MarshalVarsFile(skeleton)
	if err != nil {
		return nil, fmt.Errorf("encode editor bindings skeleton: %w", err)
	}

	tempFile, err := os.CreateTemp("", bindingsEditFilePattern)
	if err != nil {
		return nil, fmt.Errorf("create editor bindings temp file: %w", err)
	}
	tempName := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("close editor bindings temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tempName)
	}()

	//nolint:gosec // tempName comes from os.CreateTemp in this function and stays confined to the local temp dir.
	if err := os.WriteFile(tempName, data, 0o600); err != nil {
		return nil, fmt.Errorf("write editor bindings skeleton: %w", err)
	}
	if err := editor.Edit(ctx, tempName); err != nil {
		return nil, fmt.Errorf("run bindings editor: %w", err)
	}

	//nolint:gosec // tempName comes from os.CreateTemp in this function and is only rewritten by the configured editor process.
	editedData, err := os.ReadFile(tempName)
	if err != nil {
		return nil, fmt.Errorf("read edited bindings skeleton: %w", err)
	}
	editedFile, err := bindings.ParseVarsData(editedData)
	if err != nil {
		return nil, fmt.Errorf("parse edited bindings skeleton: %w", err)
	}

	filled := make(map[string]bindings.VariableBinding, len(editedFile.Variables))
	for name, binding := range editedFile.Variables {
		if strings.TrimSpace(binding.Value) == "" {
			continue
		}
		filled[name] = binding
	}

	return filled, nil
}
