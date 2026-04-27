package orbittemplate

import (
	"context"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	sourceManifestRelativePath = ".harness/manifest.yaml"
	sourceSchemaVersion        = 1
	SourceKind                 = "source"
)

// SourceManifest is the source-branch contract stored in .harness/manifest.yaml with kind=source.
type SourceManifest struct {
	SchemaVersion int                  `yaml:"schema_version"`
	Kind          string               `yaml:"kind"`
	SourceBranch  string               `yaml:"source_branch"`
	Publish       *SourcePublishConfig `yaml:"publish,omitempty"`
}

type SourcePublishConfig struct {
	Package ids.PackageIdentity `yaml:"package"`
	OrbitID string              `yaml:"orbit_id"`
}

type rawSourceManifest struct {
	SchemaVersion *int              `yaml:"schema_version"`
	Kind          *string           `yaml:"kind"`
	Source        *rawSourceSection `yaml:"source"`
}

type rawSourceSection struct {
	Package      *ids.PackageIdentity `yaml:"package"`
	OrbitID      *string              `yaml:"orbit_id"`
	SourceBranch *string              `yaml:"source_branch"`
}

// SourceManifestPath returns the absolute path to the source manifest contract file.
func SourceManifestPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(sourceManifestRelativePath))
}

// LoadSourceManifest reads, decodes, and validates the source manifest file.
func LoadSourceManifest(repoRoot string) (SourceManifest, error) {
	filename := SourceManifestPath(repoRoot)
	data, err := gitpkg.ReadFileWorktreeOrHEAD(context.Background(), repoRoot, sourceManifestRelativePath)
	if err != nil {
		return SourceManifest{}, fmt.Errorf("read %s: %w", filename, err)
	}

	manifest, err := ParseSourceManifestData(data)
	if err != nil {
		return SourceManifest{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return manifest, nil
}

// ParseSourceManifestData decodes and validates source manifest bytes.
func ParseSourceManifestData(data []byte) (SourceManifest, error) {
	var raw rawSourceManifest
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return SourceManifest{}, fmt.Errorf("decode source manifest: %w", err)
	}

	manifest, err := raw.toSourceManifest()
	if err != nil {
		return SourceManifest{}, err
	}

	return manifest, nil
}

// ValidateSourceManifest validates the source branch marker contract.
func ValidateSourceManifest(manifest SourceManifest) error {
	if manifest.SchemaVersion != sourceSchemaVersion {
		return fmt.Errorf("schema_version must be %d", sourceSchemaVersion)
	}
	if manifest.Kind != SourceKind {
		return fmt.Errorf("kind must be %q", SourceKind)
	}
	if manifest.SourceBranch == "" {
		return fmt.Errorf("source_branch must not be empty")
	}
	if manifest.Publish != nil {
		if manifest.Publish.OrbitID == "" {
			return fmt.Errorf("source.package.name must not be empty")
		}
		identity := manifest.Publish.Package
		if identity.Type == "" {
			identity.Type = ids.PackageTypeOrbit
		}
		if identity.Name == "" {
			identity.Name = manifest.Publish.OrbitID
		}
		if err := ids.ValidatePackageIdentity(identity, ids.PackageTypeOrbit, "source.package"); err != nil {
			return fmt.Errorf("validate source package: %w", err)
		}
	}

	return nil
}

// WriteSourceManifest validates and writes the source manifest with stable ordering.
func WriteSourceManifest(repoRoot string, manifest SourceManifest) (string, error) {
	if err := ValidateSourceManifest(manifest); err != nil {
		return "", fmt.Errorf("validate source manifest: %w", err)
	}

	filename := SourceManifestPath(repoRoot)
	data, err := contractutil.EncodeYAMLDocument(sourceManifestNode(manifest))
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

func (raw rawSourceManifest) toSourceManifest() (SourceManifest, error) {
	switch {
	case raw.SchemaVersion == nil:
		return SourceManifest{}, fmt.Errorf("schema_version must be present")
	case raw.Kind == nil:
		return SourceManifest{}, fmt.Errorf("kind must be present")
	case raw.Source == nil:
		return SourceManifest{}, fmt.Errorf("source must be present")
	case raw.Source.SourceBranch == nil:
		return SourceManifest{}, fmt.Errorf("source.source_branch must be present")
	}

	manifest := SourceManifest{
		SchemaVersion: *raw.SchemaVersion,
		Kind:          *raw.Kind,
		SourceBranch:  *raw.Source.SourceBranch,
	}
	if raw.Source.Package != nil || raw.Source.OrbitID != nil {
		identity, err := sourceManifestPackage(raw.Source.Package, raw.Source.OrbitID)
		if err != nil {
			return SourceManifest{}, err
		}
		manifest.Publish = &SourcePublishConfig{
			Package: identity,
			OrbitID: identity.Name,
		}
	}
	if err := ValidateSourceManifest(manifest); err != nil {
		return SourceManifest{}, err
	}

	return manifest, nil
}

func sourceManifestNode(manifest SourceManifest) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(manifest.SchemaVersion))
	contractutil.AppendMapping(root, "kind", contractutil.StringNode(manifest.Kind))
	sourceNode := contractutil.MappingNode()
	if manifest.Publish != nil {
		contractutil.AppendMapping(sourceNode, "package", sourceManifestPackageNode(manifest.Publish.Package, manifest.Publish.OrbitID))
	}
	contractutil.AppendMapping(sourceNode, "source_branch", contractutil.StringNode(manifest.SourceBranch))
	contractutil.AppendMapping(root, "source", sourceNode)

	return root
}

func sourceManifestPackage(packageIdentity *ids.PackageIdentity, legacyOrbitID *string) (ids.PackageIdentity, error) {
	if packageIdentity != nil {
		if err := ids.ValidatePackageIdentity(*packageIdentity, ids.PackageTypeOrbit, "source.package"); err != nil {
			return ids.PackageIdentity{}, fmt.Errorf("validate source package: %w", err)
		}
		return *packageIdentity, nil
	}
	if legacyOrbitID == nil || *legacyOrbitID == "" {
		return ids.PackageIdentity{}, fmt.Errorf("source.package must be present")
	}
	identity, err := ids.NewPackageIdentity(ids.PackageTypeOrbit, *legacyOrbitID, "")
	if err != nil {
		return ids.PackageIdentity{}, fmt.Errorf("derive source package: %w", err)
	}
	return identity, nil
}

func sourceManifestPackageNode(identity ids.PackageIdentity, orbitID string) *yaml.Node {
	if identity.Type == "" {
		identity.Type = ids.PackageTypeOrbit
	}
	if identity.Name == "" {
		identity.Name = orbitID
	}
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "type", contractutil.StringNode(identity.Type))
	contractutil.AppendMapping(node, "name", contractutil.StringNode(identity.Name))
	if identity.Version != "" {
		contractutil.AppendMapping(node, "version", contractutil.StringNode(identity.Version))
	}
	return node
}
