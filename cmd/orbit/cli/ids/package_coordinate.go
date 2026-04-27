package ids

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// PackageCoordinateKind describes the user-facing shape of a package coordinate.
type PackageCoordinateKind string

const (
	PackageCoordinateName       PackageCoordinateKind = "name"
	PackageCoordinateRelease    PackageCoordinateKind = "release"
	PackageCoordinateWorkspace  PackageCoordinateKind = "workspace"
	PackageCoordinateGitLocator PackageCoordinateKind = "git_locator"
)

// PackageCoordinateOptions controls parsing strictness for user-layer commands.
type PackageCoordinateOptions struct {
	StrictUserLayer bool
}

// PackageCoordinate is the shared parsed form consumed by user-layer package commands.
type PackageCoordinate struct {
	Raw     string
	Name    string
	Kind    PackageCoordinateKind
	Version string
	Locator string
}

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// ParsePackageCoordinate parses a user-facing package coordinate such as
// "execute", "execute@0.1.0", "execute@workspace", or "execute@git:<mesh>".
func ParsePackageCoordinate(input string, options PackageCoordinateOptions) (PackageCoordinate, error) {
	raw := input
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return PackageCoordinate{}, errors.New("package coordinate must not be empty")
	}
	if trimmed != input {
		return PackageCoordinate{}, errors.New("package coordinate must not contain leading or trailing whitespace")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return PackageCoordinate{}, errors.New("package coordinate must not contain NUL bytes")
	}

	name, suffix, hasSuffix := strings.Cut(trimmed, "@")
	if strings.Contains(suffix, "@") {
		return PackageCoordinate{}, errors.New("package coordinate must contain at most one @")
	}
	if err := validatePackageName(name); err != nil {
		return PackageCoordinate{}, err
	}
	if !hasSuffix {
		return PackageCoordinate{
			Raw:  raw,
			Name: name,
			Kind: PackageCoordinateName,
		}, nil
	}
	if suffix == "" {
		return PackageCoordinate{}, errors.New("package coordinate must include a version or locator after @")
	}
	if suffix == "workspace" {
		return PackageCoordinate{
			Raw:  raw,
			Name: name,
			Kind: PackageCoordinateWorkspace,
		}, nil
	}
	if strings.HasPrefix(suffix, "git:") {
		locator := strings.TrimPrefix(suffix, "git:")
		if locator == "" {
			return PackageCoordinate{}, errors.New("package coordinate git locator must not be empty")
		}
		if strings.TrimSpace(locator) != locator {
			return PackageCoordinate{}, errors.New("package coordinate git locator must not contain leading or trailing whitespace")
		}
		return PackageCoordinate{
			Raw:     raw,
			Name:    name,
			Kind:    PackageCoordinateGitLocator,
			Locator: locator,
		}, nil
	}

	version := strings.TrimPrefix(suffix, "v")
	if semverPattern.MatchString(version) {
		return PackageCoordinate{
			Raw:     raw,
			Name:    name,
			Kind:    PackageCoordinateRelease,
			Version: version,
		}, nil
	}
	if options.StrictUserLayer && barePackageLocatorPattern.MatchString(suffix) && !looksVersionLike(suffix) {
		return PackageCoordinate{}, fmt.Errorf("package coordinate locator must be explicit; use %s@git:%s for Git refs", name, suffix)
	}
	return PackageCoordinate{}, errors.New("package coordinate version must use SemVer")
}

var barePackageLocatorPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// String returns the normalized user-facing coordinate.
func (coordinate PackageCoordinate) String() string {
	switch coordinate.Kind {
	case PackageCoordinateName:
		return coordinate.Name
	case PackageCoordinateRelease:
		return coordinate.Name + "@" + coordinate.Version
	case PackageCoordinateWorkspace:
		return coordinate.Name + "@workspace"
	case PackageCoordinateGitLocator:
		return coordinate.Name + "@git:" + coordinate.Locator
	default:
		return coordinate.Raw
	}
}

// IsRelease reports whether the coordinate identifies a versioned release.
func (coordinate PackageCoordinate) IsRelease() bool {
	return coordinate.Kind == PackageCoordinateRelease
}

// IsUnversionedSnapshot reports whether the coordinate selects the default channel snapshot.
func (coordinate PackageCoordinate) IsUnversionedSnapshot() bool {
	return coordinate.Kind == PackageCoordinateName
}

func validatePackageName(name string) error {
	switch {
	case name == "":
		return errors.New("package name must not be empty")
	case len(name) > maxOrbitIDLength:
		return fmt.Errorf("package name must be at most %d characters", maxOrbitIDLength)
	case strings.TrimSpace(name) != name:
		return errors.New("package name must not contain leading or trailing whitespace")
	case !orbitIDPattern.MatchString(name):
		return errors.New("package name must use lowercase letters, digits, hyphens, or underscores, and must start and end with an alphanumeric character")
	default:
		return nil
	}
}

func looksVersionLike(value string) bool {
	if value == "" {
		return false
	}
	if value == "v" {
		return true
	}
	withoutPrefix := strings.TrimPrefix(value, "v")
	if withoutPrefix == "" {
		return false
	}
	first := withoutPrefix[0]
	return first >= '0' && first <= '9'
}
