package registry

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// HandleCoordinateKind describes the registry selector part of a Package Handle Coordinate.
type HandleCoordinateKind string

const (
	HandleCoordinateExactVersion HandleCoordinateKind = "exact_version"
	HandleCoordinateDistTag      HandleCoordinateKind = "dist_tag"
)

// PackageHandleCoordinate is the normalized registry-backed package handle syntax.
type PackageHandleCoordinate struct {
	Raw       string
	Namespace string
	Name      string
	Kind      HandleCoordinateKind
	Version   string
	Tag       string
}

var (
	handleSegmentPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]*[a-z0-9])?$`)
	handleSemverPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9a-z-]+(?:\.[0-9a-z-]+)*)?(?:\+[0-9a-z-]+(?:\.[0-9a-z-]+)*)?$`)
)

// LooksPackageHandleCoordinate reports whether an install argument should be
// routed through registry-backed Package Handle Coordinate parsing.
func LooksPackageHandleCoordinate(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "@") {
		return true
	}

	handle, suffix, hasSuffix := strings.Cut(trimmed, "@")
	if !hasSuffix || suffix == "" {
		return false
	}
	if strings.ContainsAny(handle, `:\`) || strings.HasPrefix(handle, ".") || strings.HasPrefix(handle, "/") {
		return false
	}

	return strings.Count(handle, "/") == 1
}

// ParsePackageHandleCoordinate parses a registry-backed Package Handle Coordinate.
func ParsePackageHandleCoordinate(input string) (PackageHandleCoordinate, error) {
	raw := input
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return PackageHandleCoordinate{}, errors.New("package handle coordinate must not be empty")
	}
	if trimmed != input {
		return PackageHandleCoordinate{}, errors.New("package handle coordinate must not contain leading or trailing whitespace")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return PackageHandleCoordinate{}, errors.New("package handle coordinate must not contain NUL bytes")
	}
	if strings.HasPrefix(trimmed, "@") {
		return PackageHandleCoordinate{}, errors.New("package handle coordinates use namespace/name[@version-or-tag], not npm-style @namespace/name")
	}

	handle, selector, hasSelector := strings.Cut(trimmed, "@")
	if strings.Contains(selector, "@") {
		return PackageHandleCoordinate{}, errors.New("package handle coordinate must contain at most one @")
	}
	if handle == "" {
		return PackageHandleCoordinate{}, errors.New("package handle coordinate handle must not be empty")
	}

	coordinate := PackageHandleCoordinate{Raw: raw}
	normalizedHandle := strings.ToLower(handle)
	namespaced := false
	switch slashCount := strings.Count(normalizedHandle, "/"); slashCount {
	case 0:
		coordinate.Name = normalizedHandle
	case 1:
		namespaced = true
		namespace, name, _ := strings.Cut(normalizedHandle, "/")
		coordinate.Namespace = namespace
		coordinate.Name = name
	default:
		return PackageHandleCoordinate{}, errors.New("package handle coordinate must use namespace/name or name")
	}
	if err := validateHandleSegment("namespace", coordinate.Namespace, !namespaced); err != nil {
		return PackageHandleCoordinate{}, err
	}
	if err := validateHandleSegment("name", coordinate.Name, false); err != nil {
		return PackageHandleCoordinate{}, err
	}

	if !hasSelector {
		coordinate.Kind = HandleCoordinateDistTag
		coordinate.Tag = "latest"
		return coordinate, nil
	}
	if selector == "" {
		return PackageHandleCoordinate{}, errors.New("package handle coordinate must include a version or dist-tag after @")
	}

	normalizedSelector := strings.ToLower(selector)
	version := strings.TrimPrefix(normalizedSelector, "v")
	if handleSemverPattern.MatchString(version) {
		coordinate.Kind = HandleCoordinateExactVersion
		coordinate.Version = version
		return coordinate, nil
	}
	if !handleSegmentPattern.MatchString(normalizedSelector) {
		return PackageHandleCoordinate{}, errors.New("package handle coordinate selector must be an exact SemVer version or path-safe dist-tag")
	}
	coordinate.Kind = HandleCoordinateDistTag
	coordinate.Tag = normalizedSelector
	return coordinate, nil
}

// IsExactVersion reports whether the coordinate selects an immutable SemVer release.
func (coordinate PackageHandleCoordinate) IsExactVersion() bool {
	return coordinate.Kind == HandleCoordinateExactVersion
}

// Handle returns the normalized package handle without the selector.
func (coordinate PackageHandleCoordinate) Handle() string {
	if coordinate.Namespace == "" {
		return coordinate.Name
	}

	return coordinate.Namespace + "/" + coordinate.Name
}

// String returns the normalized coordinate.
func (coordinate PackageHandleCoordinate) String() string {
	switch coordinate.Kind {
	case HandleCoordinateExactVersion:
		return coordinate.Handle() + "@" + coordinate.Version
	case HandleCoordinateDistTag:
		return coordinate.Handle() + "@" + coordinate.Tag
	default:
		return coordinate.Raw
	}
}

func validateHandleSegment(label string, value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("package handle coordinate %s must not be empty", label)
	}
	if !handleSegmentPattern.MatchString(value) {
		return fmt.Errorf("package handle coordinate %s must use lowercase letters, digits, hyphens, or underscores, and must start and end with an alphanumeric character", label)
	}

	return nil
}
