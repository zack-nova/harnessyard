package harness

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const (
	manifestSchemaVersion = 1

	ManifestKindSource          = "source"
	ManifestKindRuntime         = "runtime"
	ManifestKindOrbitTemplate   = "orbit_template"
	ManifestKindHarnessTemplate = "harness_template"

	ManifestMemberSourceManual        = "manual"
	ManifestMemberSourceInstallOrbit  = "install_orbit"
	ManifestMemberSourceInstallBundle = "install_bundle"
)

// ManifestFile is the schema-backed single-control-plane manifest stored in .harness/manifest.yaml.
type ManifestFile struct {
	SchemaVersion int                             `yaml:"schema_version"`
	Kind          string                          `yaml:"kind"`
	Source        *ManifestSourceMetadata         `yaml:"source,omitempty"`
	Runtime       *ManifestRuntimeMetadata        `yaml:"runtime,omitempty"`
	Template      *ManifestTemplateMetadata       `yaml:"template,omitempty"`
	Members       []ManifestMember                `yaml:"members,omitempty"`
	Variables     map[string]ManifestVariableSpec `yaml:"variables,omitempty"`
	RootGuidance  RootGuidanceMetadata            `yaml:"root_guidance,omitempty"`
}

// ManifestSourceMetadata stores source-branch authoring identity.
type ManifestSourceMetadata struct {
	Package      ids.PackageIdentity `yaml:"package"`
	OrbitID      string              `yaml:"orbit_id"`
	SourceBranch string              `yaml:"source_branch"`
}

// ManifestRuntimeMetadata stores runtime identity and timestamps.
type ManifestRuntimeMetadata struct {
	Package   ids.PackageIdentity `yaml:"package"`
	ID        string              `yaml:"id"`
	Name      string              `yaml:"name,omitempty"`
	CreatedAt time.Time           `yaml:"created_at"`
	UpdatedAt time.Time           `yaml:"updated_at"`
}

// ManifestTemplateMetadata stores template provenance for either orbit or harness templates.
type ManifestTemplateMetadata struct {
	Package           ids.PackageIdentity `yaml:"package"`
	OrbitID           string              `yaml:"orbit_id,omitempty"`
	HarnessID         string              `yaml:"harness_id,omitempty"`
	DefaultTemplate   bool                `yaml:"default_template,omitempty"`
	CreatedFromBranch string              `yaml:"created_from_branch"`
	CreatedFromCommit string              `yaml:"created_from_commit"`
	CreatedAt         time.Time           `yaml:"created_at"`
}

// ManifestMember stores one declared member in either runtime or harness-template manifests.
type ManifestMember struct {
	Package              ids.PackageIdentity   `yaml:"package"`
	OrbitID              string                `yaml:"orbit_id"`
	Source               string                `yaml:"source,omitempty"`
	IncludedIn           *ids.PackageIdentity  `yaml:"included_in,omitempty"`
	OwnerHarnessID       string                `yaml:"owner_harness_id,omitempty"`
	AddedAt              time.Time             `yaml:"added_at,omitempty"`
	LastStandaloneOrigin *orbittemplate.Source `yaml:"last_standalone_origin,omitempty"`
}

// ManifestVariableSpec captures template variable metadata embedded into the single-control-plane branch manifest.
type ManifestVariableSpec struct {
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required"`
}

type rawManifestFile struct {
	SchemaVersion      *int                               `yaml:"schema_version"`
	Kind               *string                            `yaml:"kind"`
	Source             *rawManifestSource                 `yaml:"source"`
	Runtime            *rawManifestRuntime                `yaml:"runtime"`
	Template           *rawManifestTemplate               `yaml:"template"`
	Members            []rawManifestMember                `yaml:"members"`
	Packages           []rawManifestMember                `yaml:"packages"`
	Variables          map[string]rawManifestVariableSpec `yaml:"variables"`
	IncludesRootAgents *bool                              `yaml:"includes_root_agents"`
	RootGuidance       *rawRootGuidanceMetadata           `yaml:"root_guidance"`
}

type rawManifestSource struct {
	Package      *ids.PackageIdentity `yaml:"package"`
	OrbitID      *string              `yaml:"orbit_id"`
	SourceBranch *string              `yaml:"source_branch"`
}

type rawManifestRuntime struct {
	Package   *ids.PackageIdentity `yaml:"package"`
	ID        *string              `yaml:"id"`
	Name      *string              `yaml:"name"`
	CreatedAt *time.Time           `yaml:"created_at"`
	UpdatedAt *time.Time           `yaml:"updated_at"`
}

type rawManifestTemplate struct {
	Package           *ids.PackageIdentity `yaml:"package"`
	OrbitID           *string              `yaml:"orbit_id"`
	HarnessID         *string              `yaml:"harness_id"`
	DefaultTemplate   *bool                `yaml:"default_template"`
	CreatedFromBranch *string              `yaml:"created_from_branch"`
	CreatedFromCommit *string              `yaml:"created_from_commit"`
	CreatedAt         *time.Time           `yaml:"created_at"`
}

type rawManifestMember struct {
	Package              *ids.PackageIdentity  `yaml:"package"`
	OrbitID              *string               `yaml:"orbit_id"`
	Source               *string               `yaml:"source"`
	IncludedIn           *ids.PackageIdentity  `yaml:"included_in"`
	OwnerHarnessID       *string               `yaml:"owner_harness_id"`
	AddedAt              *time.Time            `yaml:"added_at"`
	LastStandaloneOrigin *orbittemplate.Source `yaml:"last_standalone_origin"`
}

type rawManifestVariableSpec struct {
	Description *string `yaml:"description"`
	Required    *bool   `yaml:"required"`
}

// LoadManifestFile reads, decodes, and validates .harness/manifest.yaml.
// It falls back to HEAD when sparse-checkout currently hides the control file.
func LoadManifestFile(repoRoot string) (ManifestFile, error) {
	data, err := gitpkg.ReadFileWorktreeOrHEAD(context.Background(), repoRoot, ManifestRepoPath())
	if err != nil {
		return ManifestFile{}, fmt.Errorf("read %s: %w", ManifestPath(repoRoot), err)
	}

	file, err := ParseManifestFileData(data)
	if err != nil {
		return ManifestFile{}, fmt.Errorf("validate %s: %w", ManifestPath(repoRoot), err)
	}

	return file, nil
}

// LoadManifestFileAtPath reads, decodes, and validates one single-control-plane manifest.
func LoadManifestFileAtPath(filename string) (ManifestFile, error) {
	//nolint:gosec // The path is repo-local and built from the fixed manifest contract path.
	data, err := os.ReadFile(filename)
	if err != nil {
		return ManifestFile{}, fmt.Errorf("read %s: %w", filename, err)
	}

	file, err := ParseManifestFileData(data)
	if err != nil {
		return ManifestFile{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return file, nil
}

// ParseManifestFileData decodes and validates .harness/manifest.yaml bytes.
func ParseManifestFileData(data []byte) (ManifestFile, error) {
	var raw rawManifestFile
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return ManifestFile{}, fmt.Errorf("decode manifest file: %w", err)
	}

	file, err := raw.toManifestFile()
	if err != nil {
		return ManifestFile{}, err
	}

	return file, nil
}

// ValidateManifestFile validates the top-level manifest schema contract.
func ValidateManifestFile(file ManifestFile) error {
	if file.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("schema_version must be %d", manifestSchemaVersion)
	}

	switch file.Kind {
	case ManifestKindSource:
		return ValidateSourceManifestFile(file)
	case ManifestKindRuntime:
		return ValidateRuntimeManifestFile(file)
	case ManifestKindOrbitTemplate:
		return ValidateOrbitTemplateManifestFile(file)
	case ManifestKindHarnessTemplate:
		return ValidateHarnessTemplateManifestFile(file)
	default:
		return fmt.Errorf(
			"kind must be one of %q, %q, %q, or %q",
			ManifestKindSource,
			ManifestKindRuntime,
			ManifestKindOrbitTemplate,
			ManifestKindHarnessTemplate,
		)
	}
}

// ValidateSourceManifestFile validates the source branch form of .harness/manifest.yaml.
func ValidateSourceManifestFile(file ManifestFile) error {
	if file.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("schema_version must be %d", manifestSchemaVersion)
	}
	if file.Kind != ManifestKindSource {
		return fmt.Errorf("kind must be %q", ManifestKindSource)
	}
	if file.Source == nil {
		return fmt.Errorf("source must be present")
	}
	if file.Runtime != nil {
		return fmt.Errorf("runtime must not be present")
	}
	if file.Template != nil {
		return fmt.Errorf("template must not be present")
	}
	if file.Members != nil {
		return fmt.Errorf("members must not be present")
	}
	if !isZeroRootGuidance(file.RootGuidance) {
		return fmt.Errorf("root_guidance must not be present")
	}
	if err := ids.ValidatePackageIdentity(
		ensureOrbitPackageIdentity(file.Source.Package, file.Source.OrbitID),
		ids.PackageTypeOrbit,
		"source.package",
	); err != nil {
		return fmt.Errorf("validate source package: %w", err)
	}
	if file.Source.SourceBranch == "" {
		return fmt.Errorf("source.source_branch must not be empty")
	}

	return nil
}

// ValidateRuntimeManifestFile validates the runtime branch form of .harness/manifest.yaml.
func ValidateRuntimeManifestFile(file ManifestFile) error {
	if file.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("schema_version must be %d", manifestSchemaVersion)
	}
	if file.Kind != ManifestKindRuntime {
		return fmt.Errorf("kind must be %q", ManifestKindRuntime)
	}
	if file.Runtime == nil {
		return fmt.Errorf("runtime must be present")
	}
	if file.Template != nil {
		return fmt.Errorf("template must not be present")
	}
	if file.Members == nil {
		return fmt.Errorf("packages must be present")
	}
	if !isZeroRootGuidance(file.RootGuidance) {
		return fmt.Errorf("root_guidance must not be present")
	}
	if err := ids.ValidatePackageIdentity(
		ensureHarnessPackageIdentity(file.Runtime.Package, file.Runtime.ID),
		ids.PackageTypeHarness,
		"runtime.package",
	); err != nil {
		return fmt.Errorf("validate runtime package: %w", err)
	}
	if file.Runtime.CreatedAt.IsZero() {
		return fmt.Errorf("runtime.created_at must be set")
	}
	if file.Runtime.UpdatedAt.IsZero() {
		return fmt.Errorf("runtime.updated_at must be set")
	}

	seenOrbitIDs := make(map[string]struct{}, len(file.Members))
	for index, member := range file.Members {
		identity := ensureOrbitPackageIdentity(member.Package, member.OrbitID)
		if err := ids.ValidatePackageIdentity(identity, ids.PackageTypeOrbit, fmt.Sprintf("packages[%d].package", index)); err != nil {
			return fmt.Errorf("validate runtime package member: %w", err)
		}
		if _, ok := seenOrbitIDs[identity.Name]; ok {
			return fmt.Errorf("packages[%d].package.name must be unique", index)
		}
		seenOrbitIDs[identity.Name] = struct{}{}

		switch member.Source {
		case ManifestMemberSourceManual, ManifestMemberSourceInstallOrbit, ManifestMemberSourceInstallBundle:
		default:
			return fmt.Errorf(
				"packages[%d].source must be one of %q, %q, or %q",
				index,
				ManifestMemberSourceManual,
				ManifestMemberSourceInstallOrbit,
				ManifestMemberSourceInstallBundle,
			)
		}
		if member.AddedAt.IsZero() {
			return fmt.Errorf("packages[%d].added_at must be set", index)
		}
		ownerHarnessID := member.OwnerHarnessID
		if member.IncludedIn != nil {
			if err := ids.ValidatePackageIdentity(*member.IncludedIn, ids.PackageTypeHarness, fmt.Sprintf("packages[%d].included_in", index)); err != nil {
				return fmt.Errorf("validate runtime package affiliation: %w", err)
			}
			ownerHarnessID = member.IncludedIn.Name
		}
		if err := validateRuntimePackageAffiliation(index, member.Source, ownerHarnessID, member.LastStandaloneOrigin); err != nil {
			return err
		}
	}

	return nil
}

// ValidateOrbitTemplateManifestFile validates the orbit-template branch form of .harness/manifest.yaml.
func ValidateOrbitTemplateManifestFile(file ManifestFile) error {
	if file.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("schema_version must be %d", manifestSchemaVersion)
	}
	if file.Kind != ManifestKindOrbitTemplate {
		return fmt.Errorf("kind must be %q", ManifestKindOrbitTemplate)
	}
	if file.Runtime != nil {
		return fmt.Errorf("runtime must not be present")
	}
	if file.Template == nil {
		return fmt.Errorf("template must be present")
	}
	if file.Members != nil {
		return fmt.Errorf("members must not be present")
	}
	if !isZeroRootGuidance(file.RootGuidance) {
		return fmt.Errorf("root_guidance must not be present")
	}
	if err := ids.ValidatePackageIdentity(
		ensureOrbitPackageIdentity(file.Template.Package, file.Template.OrbitID),
		ids.PackageTypeOrbit,
		"template.package",
	); err != nil {
		return fmt.Errorf("validate orbit template package: %w", err)
	}
	if file.Template.HarnessID != "" {
		return fmt.Errorf("template.harness_id must not be present")
	}
	if file.Template.CreatedFromBranch == "" {
		return fmt.Errorf("template.created_from_branch must not be empty")
	}
	if file.Template.CreatedFromCommit == "" {
		return fmt.Errorf("template.created_from_commit must not be empty")
	}
	if file.Template.CreatedAt.IsZero() {
		return fmt.Errorf("template.created_at must be set")
	}
	if file.Variables != nil {
		for _, name := range contractutil.SortedKeys(file.Variables) {
			if err := contractutil.ValidateVariableName(name); err != nil {
				return fmt.Errorf("variables.%s: %w", name, err)
			}
		}
	}

	return nil
}

// ValidateHarnessTemplateManifestFile validates the harness-template branch form of .harness/manifest.yaml.
func ValidateHarnessTemplateManifestFile(file ManifestFile) error {
	if file.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("schema_version must be %d", manifestSchemaVersion)
	}
	if file.Kind != ManifestKindHarnessTemplate {
		return fmt.Errorf("kind must be %q", ManifestKindHarnessTemplate)
	}
	if file.Runtime != nil {
		return fmt.Errorf("runtime must not be present")
	}
	if file.Template == nil {
		return fmt.Errorf("template must be present")
	}
	if file.Members == nil {
		return fmt.Errorf("packages must be present")
	}
	if file.Variables != nil {
		return fmt.Errorf("variables must not be present")
	}
	if err := ids.ValidatePackageIdentity(
		ensureHarnessPackageIdentity(file.Template.Package, file.Template.HarnessID),
		ids.PackageTypeHarness,
		"template.package",
	); err != nil {
		return fmt.Errorf("validate harness template package: %w", err)
	}
	if file.Template.OrbitID != "" {
		return fmt.Errorf("template.orbit_id must not be present")
	}
	if file.Template.CreatedFromBranch == "" {
		return fmt.Errorf("template.created_from_branch must not be empty")
	}
	if file.Template.CreatedFromCommit == "" {
		return fmt.Errorf("template.created_from_commit must not be empty")
	}
	if file.Template.CreatedAt.IsZero() {
		return fmt.Errorf("template.created_at must be set")
	}

	seenOrbitIDs := make(map[string]struct{}, len(file.Members))
	for index, member := range file.Members {
		identity := ensureOrbitPackageIdentity(member.Package, member.OrbitID)
		if err := ids.ValidatePackageIdentity(identity, ids.PackageTypeOrbit, fmt.Sprintf("packages[%d].package", index)); err != nil {
			return fmt.Errorf("validate harness template package member: %w", err)
		}
		if _, ok := seenOrbitIDs[identity.Name]; ok {
			return fmt.Errorf("packages[%d].package.name must be unique", index)
		}
		seenOrbitIDs[identity.Name] = struct{}{}

		if member.Source != "" {
			return fmt.Errorf("packages[%d].source must not be present", index)
		}
		if member.OwnerHarnessID != "" {
			return fmt.Errorf("packages[%d].included_in must not be present", index)
		}
		if member.IncludedIn != nil {
			return fmt.Errorf("packages[%d].included_in must not be present", index)
		}
		if !member.AddedAt.IsZero() {
			return fmt.Errorf("packages[%d].added_at must not be present", index)
		}
		if member.LastStandaloneOrigin != nil {
			return fmt.Errorf("packages[%d].last_standalone_origin must not be present", index)
		}
	}

	return nil
}

// MarshalManifestFile validates and encodes one single-control-plane manifest with stable ordering.
func MarshalManifestFile(file ManifestFile) ([]byte, error) {
	if err := ValidateManifestFile(file); err != nil {
		return nil, fmt.Errorf("validate manifest file: %w", err)
	}

	data, err := contractutil.EncodeYAMLDocument(manifestFileNode(file))
	if err != nil {
		return nil, fmt.Errorf("encode manifest file: %w", err)
	}

	return data, nil
}

// WriteManifestFile validates and writes .harness/manifest.yaml with stable ordering.
func WriteManifestFile(repoRoot string, file ManifestFile) (string, error) {
	return WriteManifestFileAtPath(ManifestPath(repoRoot), file)
}

// WriteManifestFileAtPath validates and writes one single-control-plane manifest.
func WriteManifestFileAtPath(filename string, file ManifestFile) (string, error) {
	data, err := MarshalManifestFile(file)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

func (raw rawManifestFile) toManifestFile() (ManifestFile, error) {
	switch {
	case raw.SchemaVersion == nil:
		return ManifestFile{}, fmt.Errorf("schema_version must be present")
	case raw.Kind == nil:
		return ManifestFile{}, fmt.Errorf("kind must be present")
	}

	file := ManifestFile{
		SchemaVersion: *raw.SchemaVersion,
		Kind:          *raw.Kind,
	}

	switch file.Kind {
	case ManifestKindSource:
		if raw.Source == nil {
			return ManifestFile{}, fmt.Errorf("source must be present")
		}
		if raw.Runtime != nil {
			return ManifestFile{}, fmt.Errorf("runtime must not be present")
		}
		if raw.Template != nil {
			return ManifestFile{}, fmt.Errorf("template must not be present")
		}
		if raw.Members != nil {
			return ManifestFile{}, fmt.Errorf("members must not be present")
		}
		if raw.Packages != nil {
			return ManifestFile{}, fmt.Errorf("packages must not be present")
		}
		if raw.Variables != nil {
			return ManifestFile{}, fmt.Errorf("variables must not be present")
		}
		if raw.IncludesRootAgents != nil {
			return ManifestFile{}, fmt.Errorf("includes_root_agents must not be present")
		}
		if raw.RootGuidance != nil {
			return ManifestFile{}, fmt.Errorf("root_guidance must not be present")
		}

		sourceMetadata, err := raw.Source.toManifestSourceMetadata()
		if err != nil {
			return ManifestFile{}, err
		}
		file.Source = &sourceMetadata

	case ManifestKindRuntime:
		if raw.Runtime == nil {
			return ManifestFile{}, fmt.Errorf("runtime must be present")
		}
		if raw.Template != nil {
			return ManifestFile{}, fmt.Errorf("template must not be present")
		}
		rawPackages := raw.Packages
		if rawPackages == nil {
			rawPackages = raw.Members
		}
		if rawPackages == nil {
			return ManifestFile{}, fmt.Errorf("packages must be present")
		}
		if raw.Variables != nil {
			return ManifestFile{}, fmt.Errorf("variables must not be present")
		}
		if raw.IncludesRootAgents != nil {
			return ManifestFile{}, fmt.Errorf("includes_root_agents must not be present")
		}
		if raw.RootGuidance != nil {
			return ManifestFile{}, fmt.Errorf("root_guidance must not be present")
		}

		runtimeMetadata, err := raw.Runtime.toManifestRuntimeMetadata()
		if err != nil {
			return ManifestFile{}, err
		}
		file.Runtime = &runtimeMetadata
		file.Members = make([]ManifestMember, 0, len(rawPackages))
		for index, rawMember := range rawPackages {
			member, err := rawMember.toRuntimeManifestMember(index)
			if err != nil {
				return ManifestFile{}, err
			}
			file.Members = append(file.Members, member)
		}

	case ManifestKindOrbitTemplate:
		if raw.Runtime != nil {
			return ManifestFile{}, fmt.Errorf("runtime must not be present")
		}
		if raw.Template == nil {
			return ManifestFile{}, fmt.Errorf("template must be present")
		}
		if raw.Members != nil {
			return ManifestFile{}, fmt.Errorf("members must not be present")
		}
		if raw.Packages != nil {
			return ManifestFile{}, fmt.Errorf("packages must not be present")
		}
		if raw.IncludesRootAgents != nil {
			return ManifestFile{}, fmt.Errorf("includes_root_agents must not be present")
		}
		if raw.RootGuidance != nil {
			return ManifestFile{}, fmt.Errorf("root_guidance must not be present")
		}

		templateMetadata, err := raw.Template.toOrbitTemplateMetadata()
		if err != nil {
			return ManifestFile{}, err
		}
		file.Template = &templateMetadata
		if raw.Variables != nil {
			file.Variables = make(map[string]ManifestVariableSpec, len(raw.Variables))
			for name, rawSpec := range raw.Variables {
				if rawSpec.Required == nil {
					return ManifestFile{}, fmt.Errorf("variables.%s.required must be present", name)
				}

				spec := ManifestVariableSpec{
					Required: *rawSpec.Required,
				}
				if rawSpec.Description != nil {
					spec.Description = *rawSpec.Description
				}
				file.Variables[name] = spec
			}
		}

	case ManifestKindHarnessTemplate:
		if raw.Runtime != nil {
			return ManifestFile{}, fmt.Errorf("runtime must not be present")
		}
		if raw.Template == nil {
			return ManifestFile{}, fmt.Errorf("template must be present")
		}
		rawPackages := raw.Packages
		if rawPackages == nil {
			rawPackages = raw.Members
		}
		if rawPackages == nil {
			return ManifestFile{}, fmt.Errorf("packages must be present")
		}
		if raw.Variables != nil {
			return ManifestFile{}, fmt.Errorf("variables must not be present")
		}
		if raw.IncludesRootAgents != nil {
			return ManifestFile{}, fmt.Errorf("includes_root_agents must not be present")
		}

		templateMetadata, err := raw.Template.toHarnessTemplateMetadata()
		if err != nil {
			return ManifestFile{}, err
		}
		file.Template = &templateMetadata
		file.Members = make([]ManifestMember, 0, len(rawPackages))
		for index, rawMember := range rawPackages {
			member, err := rawMember.toTemplateManifestMember(index)
			if err != nil {
				return ManifestFile{}, err
			}
			file.Members = append(file.Members, member)
		}
		if raw.RootGuidance == nil {
			return ManifestFile{}, fmt.Errorf("root_guidance must be present")
		}
		rootGuidance, err := raw.RootGuidance.toRootGuidanceMetadata()
		if err != nil {
			return ManifestFile{}, err
		}
		file.RootGuidance = rootGuidance

	default:
		return ManifestFile{}, fmt.Errorf(
			"kind must be one of %q, %q, %q, or %q",
			ManifestKindSource,
			ManifestKindRuntime,
			ManifestKindOrbitTemplate,
			ManifestKindHarnessTemplate,
		)
	}

	if err := ValidateManifestFile(file); err != nil {
		return ManifestFile{}, err
	}

	return file, nil
}

func (raw rawManifestSource) toManifestSourceMetadata() (ManifestSourceMetadata, error) {
	switch {
	case raw.Package == nil && raw.OrbitID == nil:
		return ManifestSourceMetadata{}, fmt.Errorf("source.package must be present")
	case raw.SourceBranch == nil:
		return ManifestSourceMetadata{}, fmt.Errorf("source.source_branch must be present")
	}
	identity, name, err := manifestPackageName(raw.Package, raw.OrbitID, ids.PackageTypeOrbit, "source.package")
	if err != nil {
		return ManifestSourceMetadata{}, err
	}

	return ManifestSourceMetadata{
		Package:      identity,
		OrbitID:      name,
		SourceBranch: *raw.SourceBranch,
	}, nil
}

func (raw rawManifestRuntime) toManifestRuntimeMetadata() (ManifestRuntimeMetadata, error) {
	switch {
	case raw.Package == nil && raw.ID == nil:
		return ManifestRuntimeMetadata{}, fmt.Errorf("runtime.package must be present")
	case raw.CreatedAt == nil:
		return ManifestRuntimeMetadata{}, fmt.Errorf("runtime.created_at must be present")
	case raw.UpdatedAt == nil:
		return ManifestRuntimeMetadata{}, fmt.Errorf("runtime.updated_at must be present")
	}

	identity, name, err := manifestPackageName(raw.Package, raw.ID, ids.PackageTypeHarness, "runtime.package")
	if err != nil {
		return ManifestRuntimeMetadata{}, err
	}

	metadata := ManifestRuntimeMetadata{
		Package:   identity,
		ID:        name,
		CreatedAt: raw.CreatedAt.UTC(),
		UpdatedAt: raw.UpdatedAt.UTC(),
	}
	if raw.Name != nil {
		metadata.Name = *raw.Name
	}

	return metadata, nil
}

func (raw rawManifestTemplate) toOrbitTemplateMetadata() (ManifestTemplateMetadata, error) {
	switch {
	case raw.Package == nil && raw.OrbitID == nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.package must be present")
	case raw.HarnessID != nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.harness_id must not be present")
	case raw.CreatedFromBranch == nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.created_from_branch must be present")
	case raw.CreatedFromCommit == nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.created_from_commit must be present")
	case raw.CreatedAt == nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.created_at must be present")
	}

	identity, name, err := manifestPackageName(raw.Package, raw.OrbitID, ids.PackageTypeOrbit, "template.package")
	if err != nil {
		return ManifestTemplateMetadata{}, err
	}

	return ManifestTemplateMetadata{
		Package:           identity,
		OrbitID:           name,
		DefaultTemplate:   raw.DefaultTemplate != nil && *raw.DefaultTemplate,
		CreatedFromBranch: *raw.CreatedFromBranch,
		CreatedFromCommit: *raw.CreatedFromCommit,
		CreatedAt:         raw.CreatedAt.UTC(),
	}, nil
}

func (raw rawManifestTemplate) toHarnessTemplateMetadata() (ManifestTemplateMetadata, error) {
	switch {
	case raw.Package == nil && raw.HarnessID == nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.package must be present")
	case raw.OrbitID != nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.orbit_id must not be present")
	case raw.CreatedFromBranch == nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.created_from_branch must be present")
	case raw.CreatedFromCommit == nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.created_from_commit must be present")
	case raw.CreatedAt == nil:
		return ManifestTemplateMetadata{}, fmt.Errorf("template.created_at must be present")
	}

	identity, name, err := manifestPackageName(raw.Package, raw.HarnessID, ids.PackageTypeHarness, "template.package")
	if err != nil {
		return ManifestTemplateMetadata{}, err
	}

	return ManifestTemplateMetadata{
		Package:           identity,
		HarnessID:         name,
		DefaultTemplate:   raw.DefaultTemplate != nil && *raw.DefaultTemplate,
		CreatedFromBranch: *raw.CreatedFromBranch,
		CreatedFromCommit: *raw.CreatedFromCommit,
		CreatedAt:         raw.CreatedAt.UTC(),
	}, nil
}

func manifestPackageName(
	identity *ids.PackageIdentity,
	legacyName *string,
	expectedType string,
	field string,
) (ids.PackageIdentity, string, error) {
	if identity != nil {
		if err := ids.ValidatePackageIdentity(*identity, expectedType, field); err != nil {
			return ids.PackageIdentity{}, "", fmt.Errorf("validate %s: %w", field, err)
		}
		return *identity, identity.Name, nil
	}

	if legacyName == nil || *legacyName == "" {
		return ids.PackageIdentity{}, "", fmt.Errorf("%s must be present", field)
	}
	next, err := ids.NewPackageIdentity(expectedType, *legacyName, "")
	if err != nil {
		return ids.PackageIdentity{}, "", fmt.Errorf("derive %s: %w", field, err)
	}

	return next, next.Name, nil
}

func (raw rawManifestMember) toRuntimeManifestMember(index int) (ManifestMember, error) {
	switch {
	case raw.Package == nil && raw.OrbitID == nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].package must be present", index)
	case raw.Source == nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].source must be present", index)
	case raw.AddedAt == nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].added_at must be present", index)
	}
	identity, name, err := manifestPackageName(raw.Package, raw.OrbitID, ids.PackageTypeOrbit, fmt.Sprintf("packages[%d].package", index))
	if err != nil {
		return ManifestMember{}, fmt.Errorf("parse runtime package member: %w", err)
	}
	includedIn := cloneManifestPackageIdentity(raw.IncludedIn)
	ownerHarnessID := valueOrEmpty(raw.OwnerHarnessID)
	if includedIn != nil {
		if err := ids.ValidatePackageIdentity(*includedIn, ids.PackageTypeHarness, fmt.Sprintf("packages[%d].included_in", index)); err != nil {
			return ManifestMember{}, fmt.Errorf("parse runtime package affiliation: %w", err)
		}
		ownerHarnessID = includedIn.Name
	}

	return ManifestMember{
		Package:              identity,
		OrbitID:              name,
		Source:               *raw.Source,
		IncludedIn:           includedIn,
		OwnerHarnessID:       ownerHarnessID,
		AddedAt:              raw.AddedAt.UTC(),
		LastStandaloneOrigin: cloneTemplateSource(raw.LastStandaloneOrigin),
	}, nil
}

func (raw rawManifestMember) toTemplateManifestMember(index int) (ManifestMember, error) {
	switch {
	case raw.Package == nil && raw.OrbitID == nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].package must be present", index)
	case raw.Source != nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].source must not be present", index)
	case raw.OwnerHarnessID != nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].owner_harness_id must not be present", index)
	case raw.IncludedIn != nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].included_in must not be present", index)
	case raw.AddedAt != nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].added_at must not be present", index)
	case raw.LastStandaloneOrigin != nil:
		return ManifestMember{}, fmt.Errorf("packages[%d].last_standalone_origin must not be present", index)
	}
	identity, name, err := manifestPackageName(raw.Package, raw.OrbitID, ids.PackageTypeOrbit, fmt.Sprintf("packages[%d].package", index))
	if err != nil {
		return ManifestMember{}, err
	}

	return ManifestMember{
		Package: identity,
		OrbitID: name,
	}, nil
}

func manifestFileNode(file ManifestFile) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(file.SchemaVersion))
	contractutil.AppendMapping(root, "kind", contractutil.StringNode(file.Kind))

	switch file.Kind {
	case ManifestKindSource:
		contractutil.AppendMapping(root, "source", sourceNode(file.Source))

	case ManifestKindRuntime:
		runtimeNode := contractutil.MappingNode()
		contractutil.AppendMapping(runtimeNode, "package", packageIdentityNode(ensureHarnessPackageIdentity(file.Runtime.Package, file.Runtime.ID)))
		if file.Runtime.Name != "" {
			contractutil.AppendMapping(runtimeNode, "name", contractutil.StringNode(file.Runtime.Name))
		}
		contractutil.AppendMapping(runtimeNode, "created_at", contractutil.TimestampNode(file.Runtime.CreatedAt))
		contractutil.AppendMapping(runtimeNode, "updated_at", contractutil.TimestampNode(file.Runtime.UpdatedAt))
		contractutil.AppendMapping(root, "runtime", runtimeNode)
		contractutil.AppendMapping(root, "packages", runtimeMembersNode(file.Members))

	case ManifestKindOrbitTemplate:
		contractutil.AppendMapping(root, "template", orbitTemplateNode(file.Template))
		if file.Variables != nil {
			contractutil.AppendMapping(root, "variables", manifestVariablesNode(file.Variables))
		}

	case ManifestKindHarnessTemplate:
		contractutil.AppendMapping(root, "template", harnessTemplateNode(file.Template))
		contractutil.AppendMapping(root, "packages", templateMembersNode(file.Members))
		rootGuidanceNode := contractutil.MappingNode()
		contractutil.AppendMapping(rootGuidanceNode, "agents", contractutil.BoolNode(file.RootGuidance.Agents))
		contractutil.AppendMapping(rootGuidanceNode, "humans", contractutil.BoolNode(file.RootGuidance.Humans))
		contractutil.AppendMapping(rootGuidanceNode, "bootstrap", contractutil.BoolNode(file.RootGuidance.Bootstrap))
		contractutil.AppendMapping(root, "root_guidance", rootGuidanceNode)
	}

	return root
}

func isZeroRootGuidance(rootGuidance RootGuidanceMetadata) bool {
	return !rootGuidance.Agents && !rootGuidance.Humans && !rootGuidance.Bootstrap
}

func sourceNode(source *ManifestSourceMetadata) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "package", packageIdentityNode(ensureOrbitPackageIdentity(source.Package, source.OrbitID)))
	contractutil.AppendMapping(node, "source_branch", contractutil.StringNode(source.SourceBranch))
	return node
}

func orbitTemplateNode(template *ManifestTemplateMetadata) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "package", packageIdentityNode(ensureOrbitPackageIdentity(template.Package, template.OrbitID)))
	contractutil.AppendMapping(node, "default_template", contractutil.BoolNode(template.DefaultTemplate))
	contractutil.AppendMapping(node, "created_from_branch", contractutil.StringNode(template.CreatedFromBranch))
	contractutil.AppendMapping(node, "created_from_commit", contractutil.StringNode(template.CreatedFromCommit))
	contractutil.AppendMapping(node, "created_at", contractutil.TimestampNode(template.CreatedAt))
	return node
}

func harnessTemplateNode(template *ManifestTemplateMetadata) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "package", packageIdentityNode(ensureHarnessPackageIdentity(template.Package, template.HarnessID)))
	contractutil.AppendMapping(node, "default_template", contractutil.BoolNode(template.DefaultTemplate))
	contractutil.AppendMapping(node, "created_from_branch", contractutil.StringNode(template.CreatedFromBranch))
	contractutil.AppendMapping(node, "created_from_commit", contractutil.StringNode(template.CreatedFromCommit))
	contractutil.AppendMapping(node, "created_at", contractutil.TimestampNode(template.CreatedAt))
	return node
}

func packageIdentityNode(identity ids.PackageIdentity) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "type", contractutil.StringNode(identity.Type))
	contractutil.AppendMapping(node, "name", contractutil.StringNode(identity.Name))
	if identity.Version != "" {
		contractutil.AppendMapping(node, "version", contractutil.StringNode(identity.Version))
	}
	return node
}

func ensureOrbitPackageIdentity(identity ids.PackageIdentity, name string) ids.PackageIdentity {
	if identity.Type == "" {
		identity.Type = ids.PackageTypeOrbit
	}
	if identity.Name == "" {
		identity.Name = name
	}
	return identity
}

func ensureHarnessPackageIdentity(identity ids.PackageIdentity, name string) ids.PackageIdentity {
	if identity.Type == "" {
		identity.Type = ids.PackageTypeHarness
	}
	if identity.Name == "" {
		identity.Name = name
	}
	return identity
}

func cloneManifestPackageIdentity(identity *ids.PackageIdentity) *ids.PackageIdentity {
	if identity == nil {
		return nil
	}
	next := *identity
	return &next
}

func runtimeMembersNode(members []ManifestMember) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, member := range sortedManifestMembers(members) {
		memberNode := contractutil.MappingNode()
		contractutil.AppendMapping(memberNode, "package", packageIdentityNode(ensureOrbitPackageIdentity(member.Package, member.OrbitID)))
		contractutil.AppendMapping(memberNode, "source", contractutil.StringNode(member.Source))
		includedIn := member.IncludedIn
		if includedIn == nil && member.OwnerHarnessID != "" {
			includedIn = &ids.PackageIdentity{Type: ids.PackageTypeHarness, Name: member.OwnerHarnessID}
		}
		if includedIn != nil {
			contractutil.AppendMapping(memberNode, "included_in", packageIdentityNode(ensureHarnessPackageIdentity(*includedIn, member.OwnerHarnessID)))
		}
		contractutil.AppendMapping(memberNode, "added_at", contractutil.TimestampNode(member.AddedAt))
		if member.LastStandaloneOrigin != nil {
			contractutil.AppendMapping(memberNode, "last_standalone_origin", templateSourceNode(*member.LastStandaloneOrigin))
		}
		node.Content = append(node.Content, memberNode)
	}

	return node
}

func templateMembersNode(members []ManifestMember) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, member := range sortedManifestMembers(members) {
		memberNode := contractutil.MappingNode()
		contractutil.AppendMapping(memberNode, "package", packageIdentityNode(ensureOrbitPackageIdentity(member.Package, member.OrbitID)))
		node.Content = append(node.Content, memberNode)
	}

	return node
}

func manifestVariablesNode(variables map[string]ManifestVariableSpec) *yaml.Node {
	node := contractutil.MappingNode()
	for _, name := range contractutil.SortedKeys(variables) {
		spec := variables[name]
		specNode := contractutil.MappingNode()
		if spec.Description != "" {
			contractutil.AppendMapping(specNode, "description", contractutil.StringNode(spec.Description))
		}
		contractutil.AppendMapping(specNode, "required", contractutil.BoolNode(spec.Required))
		contractutil.AppendMapping(node, name, specNode)
	}

	return node
}

func sortedManifestMembers(members []ManifestMember) []ManifestMember {
	sorted := append([]ManifestMember(nil), members...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OrbitID < sorted[j].OrbitID
	})
	return sorted
}

func templateSourceNode(source orbittemplate.Source) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "source_kind", contractutil.StringNode(source.SourceKind))
	contractutil.AppendMapping(node, "source_repo", contractutil.StringNode(source.SourceRepo))
	contractutil.AppendMapping(node, "source_ref", contractutil.StringNode(source.SourceRef))
	contractutil.AppendMapping(node, "template_commit", contractutil.StringNode(source.TemplateCommit))
	return node
}
