package packageops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

const manifestRelativePath = ".harness/manifest.yaml"

// RenameHostedOrbitPackageResult summarizes a hosted orbit package rename.
type RenameHostedOrbitPackageResult struct {
	RepoRoot          string `json:"repo_root"`
	OldPackage        string `json:"old_package"`
	NewPackage        string `json:"new_package"`
	OldDefinitionPath string `json:"old_definition_path"`
	NewDefinitionPath string `json:"new_definition_path"`
	ManifestPath      string `json:"manifest_path,omitempty"`
	ManifestChanged   bool   `json:"manifest_changed"`
}

type manifestRenamePlan struct {
	path    string
	changed bool
	file    harness.ManifestFile
}

// RenameHostedOrbitPackage renames the authored hosted OrbitSpec identity and
// the current branch manifest identity when it points at the same orbit package.
func RenameHostedOrbitPackage(ctx context.Context, repoRoot string, oldPackage string, newPackage string) (RenameHostedOrbitPackageResult, error) {
	oldPackage = strings.TrimSpace(oldPackage)
	newPackage = strings.TrimSpace(newPackage)
	if err := ids.ValidateOrbitID(oldPackage); err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("validate old package: %w", err)
	}
	if err := ids.ValidateOrbitID(newPackage); err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("validate new package: %w", err)
	}
	if oldPackage == newPackage {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("new package must differ from old package %q", oldPackage)
	}

	oldRelativePath, err := orbit.HostedDefinitionRelativePath(oldPackage)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("build old definition path: %w", err)
	}
	newRelativePath, err := orbit.HostedDefinitionRelativePath(newPackage)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("build new definition path: %w", err)
	}
	oldPath := filepath.Join(repoRoot, filepath.FromSlash(oldRelativePath))
	newPath := filepath.Join(repoRoot, filepath.FromSlash(newRelativePath))

	if _, err := os.Stat(oldPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("orbit package %q not found", oldPackage)
		}
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("stat old orbit package %q: %w", oldPackage, err)
	}
	if _, err := os.Stat(newPath); err == nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("orbit package %q already exists", newPackage)
	} else if !errors.Is(err, os.ErrNotExist) {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("stat new orbit package %q: %w", newPackage, err)
	}

	config, err := orbit.LoadHostedRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("load hosted orbit config: %w", err)
	}
	if _, found := config.OrbitByID(oldPackage); !found {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("orbit package %q not found", oldPackage)
	}
	if _, found := config.OrbitByID(newPackage); found {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("orbit package %q already exists", newPackage)
	}

	manifestPlan, err := planManifestRename(repoRoot, oldPackage, newPackage)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, err
	}

	spec, err := orbit.LoadHostedOrbitSpec(ctx, repoRoot, oldPackage)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}
	renamedSpec, err := renamedHostedSpec(spec, newPackage, newRelativePath)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, err
	}

	if _, err := orbit.WriteHostedOrbitSpec(repoRoot, renamedSpec); err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed orbit package: %w", err)
	}
	if manifestPlan.changed {
		if _, err := harness.WriteManifestFile(repoRoot, manifestPlan.file); err != nil {
			if cleanupErr := os.Remove(newPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed manifest package: %w; rollback new orbit package %q: %w", err, newPackage, cleanupErr)
			}
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed manifest package: %w", err)
		}
	}
	if err := os.Remove(oldPath); err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("remove old orbit package %q: %w", oldPackage, err)
	}

	return RenameHostedOrbitPackageResult{
		RepoRoot:          repoRoot,
		OldPackage:        oldPackage,
		NewPackage:        newPackage,
		OldDefinitionPath: oldRelativePath,
		NewDefinitionPath: newRelativePath,
		ManifestPath:      manifestPlan.path,
		ManifestChanged:   manifestPlan.changed,
	}, nil
}

func renamedHostedSpec(spec orbit.OrbitSpec, newPackage string, newRelativePath string) (orbit.OrbitSpec, error) {
	version := ""
	if spec.Package != nil {
		version = spec.Package.Version
	}
	identity := ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: newPackage, Version: version}
	if err := ids.ValidatePackageIdentity(identity, ids.PackageTypeOrbit, "package"); err != nil {
		return orbit.OrbitSpec{}, fmt.Errorf("validate renamed orbit package: %w", err)
	}

	spec.Package = &identity
	spec.ID = newPackage
	spec.SourcePath = ""
	if spec.Meta != nil {
		spec.Meta.File = newRelativePath
	}

	return spec, nil
}

func planManifestRename(repoRoot string, oldPackage string, newPackage string) (manifestRenamePlan, error) {
	file, err := harness.LoadManifestFile(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifestRenamePlan{}, nil
		}
		return manifestRenamePlan{}, fmt.Errorf("load %s: %w", manifestRelativePath, err)
	}

	plan := manifestRenamePlan{path: manifestRelativePath, file: file}
	switch file.Kind {
	case harness.ManifestKindSource:
		current := manifestOrbitName(file.Source.Package, file.Source.OrbitID)
		if current != oldPackage {
			return manifestRenamePlan{}, fmt.Errorf("source package %q must match old package %q", current, oldPackage)
		}
		plan.file.Source.Package = renamedOrbitIdentity(file.Source.Package, newPackage)
		plan.file.Source.OrbitID = newPackage
		plan.changed = true
	case harness.ManifestKindOrbitTemplate:
		current := manifestOrbitName(file.Template.Package, file.Template.OrbitID)
		if current != oldPackage {
			return manifestRenamePlan{}, fmt.Errorf("template package %q must match old package %q", current, oldPackage)
		}
		plan.file.Template.Package = renamedOrbitIdentity(file.Template.Package, newPackage)
		plan.file.Template.OrbitID = newPackage
		plan.changed = true
	case harness.ManifestKindRuntime, harness.ManifestKindHarnessTemplate:
		for _, member := range file.Members {
			if manifestOrbitName(member.Package, member.OrbitID) == oldPackage {
				return manifestRenamePlan{}, fmt.Errorf("renaming runtime or harness-template members is not supported yet; remove or reinstall package %q instead", oldPackage)
			}
		}
	}

	return plan, nil
}

func manifestOrbitName(identity ids.PackageIdentity, fallback string) string {
	if strings.TrimSpace(identity.Name) != "" {
		return identity.Name
	}
	return strings.TrimSpace(fallback)
}

func renamedOrbitIdentity(identity ids.PackageIdentity, newPackage string) ids.PackageIdentity {
	identity.Type = ids.PackageTypeOrbit
	identity.Name = newPackage
	return identity
}
