package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/registry"
)

func newInstallCommand() *cobra.Command {
	cmd := harnesscommands.NewInstallCommand()
	originalRunE := cmd.RunE
	cmd.Use = "install <package-handle@version|package@git:ref|template-branch|git-source>"
	cmd.Example = "" +
		"  hyard install acme/docs@0.1.0\n" +
		"  hyard install docs@git:orbit-template/docs\n" +
		"  hyard install orbit-template/docs --bindings .harness/vars.yaml\n" +
		"  hyard install https://example.com/acme/templates.git --ref orbit-template/docs --bindings .harness/vars.yaml\n" +
		"  hyard install orbit-template/docs --overwrite-existing --bindings .harness/vars.yaml --json\n"
	cmd.Flags().String("registry-source", "", "Git remote or local path for Package Handle Coordinate registry source")
	cmd.Flags().String("registry-ref", registry.DefaultRegistryRef, "Git ref to read from the Package Handle Coordinate registry source")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 && registry.LooksPackageHandleCoordinate(args[0]) {
			resolution, err := resolvePackageHandleInstallCoordinate(cmd, args[0])
			if err != nil {
				return err
			}
			args[0] = resolution.SourceRemote
			if err := setFlagString(cmd, "ref", resolution.SourceCommit); err != nil {
				return err
			}
			if err := bindPackageMetadata(cmd, packageMetadataFromRegistryResolution(resolution)); err != nil {
				return err
			}
			if err := bindPackageResolutionWarnings(cmd, resolution.Warnings); err != nil {
				return err
			}
		} else if len(args) == 1 && shouldParseHyardPackageCoordinateArg(args[0]) {
			coordinate, err := parseHyardPackageCoordinate(args[0])
			if err != nil {
				return err
			}
			if coordinate.Kind != ids.PackageCoordinateGitLocator {
				return fmt.Errorf("hyard install package coordinate %s is not supported yet; use %s@git:<ref> or the existing explicit source form", coordinate.String(), coordinate.Name)
			}
			if cmd.Flags().Changed("ref") {
				return fmt.Errorf("package coordinate %s cannot be combined with --ref; put the git locator after @git", coordinate.String())
			}
			locator := normalizePackageGitLocatorRef(coordinate.Locator)
			args[0] = locator
			if err := bindPackageMetadata(cmd, packageMetadataFromCoordinateWithLocator(coordinate, locator)); err != nil {
				return err
			}
		}

		return originalRunE(cmd, args)
	}

	return cmd
}

func resolvePackageHandleInstallCoordinate(cmd *cobra.Command, raw string) (registry.Resolution, error) {
	coordinate, err := registry.ParsePackageHandleCoordinate(raw)
	if err != nil {
		return registry.Resolution{}, fmt.Errorf("parse package handle coordinate: %w", err)
	}
	if !coordinate.IsExactVersion() {
		return registry.Resolution{}, fmt.Errorf("hyard install currently supports exact SemVer Package Handle Coordinates only; got %s", coordinate.String())
	}
	if cmd.Flags().Changed("ref") {
		return registry.Resolution{}, fmt.Errorf("package handle coordinate %s cannot be combined with --ref; registry versions resolve their source ref from catalog data", coordinate.String())
	}

	targetPath, err := hyardInstallTargetPathFromCommand(cmd)
	if err != nil {
		return registry.Resolution{}, err
	}
	resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
	if err != nil {
		return registry.Resolution{}, fmt.Errorf("resolve harness root: %w", err)
	}
	registrySource, err := packageRegistrySourceFromCommand(cmd)
	if err != nil {
		return registry.Resolution{}, err
	}
	cacheRoot, err := registry.DefaultCacheRoot()
	if err != nil {
		return registry.Resolution{}, fmt.Errorf("resolve registry cache root: %w", err)
	}

	resolution, err := registry.ResolveExactPackageHandleCoordinate(cmd.Context(), registry.ResolveInput{
		RepoRoot:       resolved.Repo.Root,
		Coordinate:     coordinate,
		RegistrySource: registrySource,
		CacheRoot:      cacheRoot,
	})
	if err != nil {
		return registry.Resolution{}, fmt.Errorf("resolve package handle coordinate: %w", err)
	}

	return resolution, nil
}

func packageRegistrySourceFromCommand(cmd *cobra.Command) (registry.Source, error) {
	remote, err := cmd.Flags().GetString("registry-source")
	if err != nil {
		return registry.Source{}, fmt.Errorf("read --registry-source flag: %w", err)
	}
	if !cmd.Flags().Changed("registry-source") {
		if envRemote := strings.TrimSpace(os.Getenv("HYARD_REGISTRY_SOURCE")); envRemote != "" {
			remote = envRemote
		}
	}
	if strings.TrimSpace(remote) == "" {
		remote = registry.DefaultRegistryRemote
	}

	ref, err := cmd.Flags().GetString("registry-ref")
	if err != nil {
		return registry.Source{}, fmt.Errorf("read --registry-ref flag: %w", err)
	}
	if !cmd.Flags().Changed("registry-ref") {
		if envRef := strings.TrimSpace(os.Getenv("HYARD_REGISTRY_REF")); envRef != "" {
			ref = envRef
		}
	}

	return registry.Source{Remote: remote, Ref: ref}, nil
}

func hyardInstallTargetPathFromCommand(cmd *cobra.Command) (string, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return "", err
	}
	pathValue, err := cmd.Flags().GetString("path")
	if err != nil {
		return "", fmt.Errorf("read path flag: %w", err)
	}
	if strings.TrimSpace(pathValue) == "" {
		return filepath.Clean(workingDir), nil
	}
	if filepath.IsAbs(pathValue) {
		return filepath.Clean(pathValue), nil
	}

	return filepath.Clean(filepath.Join(workingDir, pathValue)), nil
}

func packageMetadataFromRegistryResolution(resolution registry.Resolution) packageMetadata {
	return packageMetadata{
		name:        resolution.PackageIdentity,
		version:     resolution.Coordinate.Version,
		publishKind: packageKindRelease,
		coordinate:  resolution.Coordinate.String(),
		locatorKind: packageLocatorKindGit,
		locator:     resolution.SourceRemote + "@" + resolution.SourceCommit,
	}
}
