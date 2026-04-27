package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	packageNameFlag        = "package-name"
	packageVersionFlag     = "package-version"
	packageCoordinateFlag  = "package-coordinate"
	packagePublishKindFlag = "package-publish-kind"
	packageLocatorKindFlag = "package-locator-kind"
	packageLocatorFlag     = "package-locator"

	packageVersionNone    = "none"
	packageKindRelease    = "release"
	packageKindSnapshot   = "snapshot"
	packageKindGitLocator = "git_locator"
	packageLocatorKindGit = "git"
)

type packageMetadata struct {
	name        string
	version     string
	publishKind string
	coordinate  string
	locatorKind string
	locator     string
}

func parseHyardPackageCoordinate(raw string) (ids.PackageCoordinate, error) {
	coordinate, err := ids.ParsePackageCoordinate(raw, ids.PackageCoordinateOptions{
		StrictUserLayer: true,
	})
	if err != nil {
		return ids.PackageCoordinate{}, fmt.Errorf("parse package coordinate: %w", err)
	}

	return coordinate, nil
}

func shouldParseHyardPackageCoordinateArg(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	name, suffix, ok := strings.Cut(trimmed, "@")
	if !ok || name == "" || suffix == "" {
		return false
	}
	if strings.ContainsAny(name, `:/\`) {
		return false
	}
	if strings.Contains(suffix, ":") && !strings.HasPrefix(suffix, "git:") {
		return false
	}

	return true
}

func normalizePackageGitLocatorRef(locator string) string {
	return strings.TrimPrefix(strings.TrimSpace(locator), "refs/heads/")
}

func packageMetadataFromCoordinate(coordinate ids.PackageCoordinate) packageMetadata {
	metadata := packageMetadata{
		name:        coordinate.Name,
		version:     packageVersionNone,
		publishKind: packageKindSnapshot,
		coordinate:  coordinate.String(),
	}
	if coordinate.Kind == ids.PackageCoordinateRelease {
		metadata.version = coordinate.Version
		metadata.publishKind = packageKindRelease
	}
	if coordinate.Kind == ids.PackageCoordinateWorkspace {
		metadata.version = "workspace"
	}
	if coordinate.Kind == ids.PackageCoordinateGitLocator {
		metadata.publishKind = packageKindGitLocator
		metadata.locatorKind = packageLocatorKindGit
		metadata.locator = coordinate.Locator
	}

	return metadata
}

func packageMetadataFromCoordinateWithLocator(coordinate ids.PackageCoordinate, locator string) packageMetadata {
	metadata := packageMetadataFromCoordinate(coordinate)
	if coordinate.Kind == ids.PackageCoordinateGitLocator {
		metadata.locator = locator
		metadata.coordinate = coordinate.Name + "@git:" + locator
	}

	return metadata
}

func bindPackageMetadata(cmd *cobra.Command, metadata packageMetadata) error {
	for flagName, value := range map[string]string{
		packageNameFlag:        metadata.name,
		packageVersionFlag:     metadata.version,
		packageCoordinateFlag:  metadata.coordinate,
		packagePublishKindFlag: metadata.publishKind,
		packageLocatorKindFlag: metadata.locatorKind,
		packageLocatorFlag:     metadata.locator,
	} {
		if err := setFlagString(cmd, flagName, value); err != nil {
			return err
		}
	}

	return nil
}

func setFlagString(cmd *cobra.Command, name string, value string) error {
	if err := cmd.Flags().Set(name, value); err != nil {
		return fmt.Errorf("set --%s flag: %w", name, err)
	}

	return nil
}
