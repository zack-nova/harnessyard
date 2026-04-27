package view

import (
	"fmt"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// CurrentDefinition resolves the current orbit state to a concrete orbit definition.
func CurrentDefinition(
	config orbitpkg.RepositoryConfig,
	current statepkg.CurrentOrbitState,
) (orbitpkg.Definition, error) {
	definition, found := config.OrbitByID(current.Orbit)
	if !found {
		return orbitpkg.Definition{}, fmt.Errorf("current orbit %q is stale; definition not found", current.Orbit)
	}

	return definition, nil
}
