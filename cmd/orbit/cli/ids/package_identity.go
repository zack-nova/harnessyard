package ids

import "fmt"

const (
	PackageTypeOrbit   = "orbit"
	PackageTypeHarness = "harness"
)

// PackageIdentity is the authored package identity stored in user-visible YAML.
type PackageIdentity struct {
	Type    string `json:"type" yaml:"type"`
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

// NewPackageIdentity builds a package identity after validating the package name.
func NewPackageIdentity(packageType string, name string, version string) (PackageIdentity, error) {
	identity := PackageIdentity{
		Type:    packageType,
		Name:    name,
		Version: version,
	}
	if err := ValidatePackageIdentity(identity, packageType, "package"); err != nil {
		return PackageIdentity{}, err
	}

	return identity, nil
}

// ValidatePackageIdentity validates one user-facing YAML package identity.
func ValidatePackageIdentity(identity PackageIdentity, expectedType string, field string) error {
	if identity.Type == "" {
		return fmt.Errorf("%s.type must be present", field)
	}
	if identity.Type != expectedType {
		return fmt.Errorf("%s.type must be %q", field, expectedType)
	}
	if identity.Name == "" {
		return fmt.Errorf("%s.name must be present", field)
	}
	if err := ValidateOrbitID(identity.Name); err != nil {
		return fmt.Errorf("%s.name: %w", field, err)
	}
	if identity.Version != "" {
		coordinate, err := ParsePackageCoordinate(identity.Name+"@"+identity.Version, PackageCoordinateOptions{StrictUserLayer: true})
		if err != nil {
			return fmt.Errorf("%s.version: %w", field, err)
		}
		if coordinate.Kind != PackageCoordinateRelease {
			return fmt.Errorf("%s.version must be a SemVer release version", field)
		}
	}

	return nil
}
