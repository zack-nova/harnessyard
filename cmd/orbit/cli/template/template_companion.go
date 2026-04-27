package orbittemplate

import (
	"context"
	"fmt"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func templateCompanionPaths(orbitID string) (string, string, error) {
	hostedPath, err := orbitpkg.HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return "", "", fmt.Errorf("build hosted companion path: %w", err)
	}
	legacyPath, err := orbitpkg.DefinitionRelativePath(orbitID)
	if err != nil {
		return "", "", fmt.Errorf("build legacy companion path: %w", err)
	}

	return hostedPath, legacyPath, nil
}

func loadTemplateCompanionAtRevision(
	ctx context.Context,
	repoRoot string,
	revision string,
	orbitID string,
) (string, orbitpkg.OrbitSpec, error) {
	hostedPath, legacyPath, err := templateCompanionPaths(orbitID)
	if err != nil {
		return "", orbitpkg.OrbitSpec{}, err
	}

	hostedExists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, revision, hostedPath)
	if err != nil {
		return "", orbitpkg.OrbitSpec{}, fmt.Errorf("check template definition %s from %q: %w", hostedPath, revision, err)
	}
	if hostedExists {
		data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, hostedPath)
		if err != nil {
			return "", orbitpkg.OrbitSpec{}, fmt.Errorf("read template definition %s from %q: %w", hostedPath, revision, err)
		}
		spec, err := orbitpkg.ParseHostedOrbitSpecData(data, hostedPath)
		if err != nil {
			return "", orbitpkg.OrbitSpec{}, fmt.Errorf("parse template definition %s from %q: %w", hostedPath, revision, err)
		}

		return hostedPath, spec, nil
	}

	// Compatibility-only fallback for pre-hosted template revisions.
	data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, legacyPath)
	if err != nil {
		return "", orbitpkg.OrbitSpec{}, fmt.Errorf("read template definition %s from %q: %w", legacyPath, revision, err)
	}
	spec, err := orbitpkg.ParseOrbitSpecData(data, legacyPath)
	if err != nil {
		return "", orbitpkg.OrbitSpec{}, fmt.Errorf("parse template definition %s from %q: %w", legacyPath, revision, err)
	}

	return legacyPath, spec, nil
}
