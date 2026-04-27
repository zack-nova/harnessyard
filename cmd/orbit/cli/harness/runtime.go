package harness

import (
	"fmt"
	"os"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const (
	runtimeSchemaVersion = 1
	RuntimeKind          = "harness_runtime"

	MemberSourceManual        = "manual"
	MemberSourceInstallOrbit  = "install_orbit"
	MemberSourceInstallBundle = "install_bundle"
)

// RuntimeFile is the manifest-backed runtime-shaped compatibility view used by
// mainline runtime helpers and tests.
type RuntimeFile struct {
	SchemaVersion int             `yaml:"schema_version"`
	Kind          string          `yaml:"kind"`
	Harness       RuntimeMetadata `yaml:"harness"`
	Members       []RuntimeMember `yaml:"members"`
}

// RuntimeMetadata stores stable runtime identity and timestamps.
type RuntimeMetadata struct {
	ID        string    `yaml:"id"`
	Name      string    `yaml:"name,omitempty"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

// RuntimeMember stores one declared harness member.
type RuntimeMember struct {
	OrbitID              string                `yaml:"orbit_id"`
	Source               string                `yaml:"source"`
	OwnerHarnessID       string                `yaml:"owner_harness_id,omitempty"`
	AddedAt              time.Time             `yaml:"added_at"`
	LastStandaloneOrigin *orbittemplate.Source `yaml:"last_standalone_origin,omitempty"`
}

type rawRuntimeFile struct {
	SchemaVersion *int                `yaml:"schema_version"`
	Kind          *string             `yaml:"kind"`
	Harness       *rawRuntimeMetadata `yaml:"harness"`
	Members       []rawRuntimeMember  `yaml:"members"`
}

type rawRuntimeMetadata struct {
	ID        *string    `yaml:"id"`
	Name      *string    `yaml:"name"`
	CreatedAt *time.Time `yaml:"created_at"`
	UpdatedAt *time.Time `yaml:"updated_at"`
}

type rawRuntimeMember struct {
	OrbitID              *string               `yaml:"orbit_id"`
	Source               *string               `yaml:"source"`
	OwnerHarnessID       *string               `yaml:"owner_harness_id"`
	AddedAt              *time.Time            `yaml:"added_at"`
	LastStandaloneOrigin *orbittemplate.Source `yaml:"last_standalone_origin"`
}

// LoadRuntimeFile reads .harness/manifest.yaml and converts it into the
// manifest-backed runtime compatibility view.
func LoadRuntimeFile(repoRoot string) (RuntimeFile, error) {
	manifestFile, err := LoadManifestFile(repoRoot)
	if err != nil {
		return RuntimeFile{}, err
	}

	runtimeFile, err := RuntimeFileFromManifestFile(manifestFile)
	if err != nil {
		return RuntimeFile{}, err
	}

	return runtimeFile, nil
}

// LoadRuntimeFileAtPath reads, decodes, and validates one explicit legacy
// .harness/runtime.yaml compatibility file.
func LoadRuntimeFileAtPath(filename string) (RuntimeFile, error) {
	//nolint:gosec // The path is repo-local and built from the fixed runtime contract path.
	data, err := os.ReadFile(filename)
	if err != nil {
		return RuntimeFile{}, fmt.Errorf("read %s: %w", filename, err)
	}

	file, err := ParseRuntimeFileData(data)
	if err != nil {
		return RuntimeFile{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return file, nil
}

// ParseRuntimeFileData decodes and validates explicit legacy
// .harness/runtime.yaml compatibility bytes.
func ParseRuntimeFileData(data []byte) (RuntimeFile, error) {
	var raw rawRuntimeFile
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return RuntimeFile{}, fmt.Errorf("decode runtime file: %w", err)
	}

	file, err := raw.toRuntimeFile()
	if err != nil {
		return RuntimeFile{}, err
	}

	return file, nil
}

// ValidateRuntimeFile validates the harness runtime schema contract.
func ValidateRuntimeFile(file RuntimeFile) error {
	if file.SchemaVersion != runtimeSchemaVersion {
		return fmt.Errorf("schema_version must be %d", runtimeSchemaVersion)
	}
	if file.Kind != RuntimeKind {
		return fmt.Errorf("kind must be %q", RuntimeKind)
	}
	if err := ids.ValidateOrbitID(file.Harness.ID); err != nil {
		return fmt.Errorf("harness.id: %w", err)
	}
	if file.Harness.CreatedAt.IsZero() {
		return fmt.Errorf("harness.created_at must be set")
	}
	if file.Harness.UpdatedAt.IsZero() {
		return fmt.Errorf("harness.updated_at must be set")
	}
	if file.Members == nil {
		return fmt.Errorf("members must be present")
	}

	seenOrbitIDs := make(map[string]struct{}, len(file.Members))
	for index, member := range file.Members {
		if err := ids.ValidateOrbitID(member.OrbitID); err != nil {
			return fmt.Errorf("members[%d].orbit_id: %w", index, err)
		}
		if _, ok := seenOrbitIDs[member.OrbitID]; ok {
			return fmt.Errorf("members[%d].orbit_id must be unique", index)
		}
		seenOrbitIDs[member.OrbitID] = struct{}{}

		switch member.Source {
		case MemberSourceManual, MemberSourceInstallOrbit, MemberSourceInstallBundle:
		default:
			return fmt.Errorf(
				"members[%d].source must be one of %q, %q, or %q",
				index,
				MemberSourceManual,
				MemberSourceInstallOrbit,
				MemberSourceInstallBundle,
			)
		}
		if member.AddedAt.IsZero() {
			return fmt.Errorf("members[%d].added_at must be set", index)
		}
		if err := validateRuntimeMemberAffiliation(index, member.Source, member.OwnerHarnessID, member.LastStandaloneOrigin); err != nil {
			return err
		}
	}

	return nil
}

// MarshalRuntimeFile validates and encodes a harness runtime document with stable ordering.
func MarshalRuntimeFile(file RuntimeFile) ([]byte, error) {
	if err := ValidateRuntimeFile(file); err != nil {
		return nil, fmt.Errorf("validate runtime file: %w", err)
	}

	data, err := contractutil.EncodeYAMLDocument(runtimeFileNode(file))
	if err != nil {
		return nil, fmt.Errorf("encode runtime file: %w", err)
	}

	return data, nil
}

// WriteRuntimeFile writes the single-control-plane manifest through the
// manifest-backed runtime compatibility view.
func WriteRuntimeFile(repoRoot string, file RuntimeFile) (string, error) {
	filename, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(file))
	if err != nil {
		return "", fmt.Errorf("write %s: %w", ManifestPath(repoRoot), err)
	}

	return filename, nil
}

// WriteRuntimeFileAtPath validates and writes one explicit legacy
// .harness/runtime.yaml compatibility document with stable field ordering.
func WriteRuntimeFileAtPath(filename string, file RuntimeFile) (string, error) {
	data, err := MarshalRuntimeFile(file)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

func (raw rawRuntimeFile) toRuntimeFile() (RuntimeFile, error) {
	switch {
	case raw.SchemaVersion == nil:
		return RuntimeFile{}, fmt.Errorf("schema_version must be present")
	case raw.Kind == nil:
		return RuntimeFile{}, fmt.Errorf("kind must be present")
	case raw.Harness == nil:
		return RuntimeFile{}, fmt.Errorf("harness must be present")
	case raw.Members == nil:
		return RuntimeFile{}, fmt.Errorf("members must be present")
	}

	metadata, err := raw.Harness.toRuntimeMetadata()
	if err != nil {
		return RuntimeFile{}, err
	}

	file := RuntimeFile{
		SchemaVersion: *raw.SchemaVersion,
		Kind:          *raw.Kind,
		Harness:       metadata,
		Members:       make([]RuntimeMember, 0, len(raw.Members)),
	}

	for index, rawMember := range raw.Members {
		member, err := rawMember.toRuntimeMember(index)
		if err != nil {
			return RuntimeFile{}, err
		}
		file.Members = append(file.Members, member)
	}

	if err := ValidateRuntimeFile(file); err != nil {
		return RuntimeFile{}, err
	}

	return file, nil
}

func (raw rawRuntimeMetadata) toRuntimeMetadata() (RuntimeMetadata, error) {
	switch {
	case raw.ID == nil:
		return RuntimeMetadata{}, fmt.Errorf("harness.id must be present")
	case raw.CreatedAt == nil:
		return RuntimeMetadata{}, fmt.Errorf("harness.created_at must be present")
	case raw.UpdatedAt == nil:
		return RuntimeMetadata{}, fmt.Errorf("harness.updated_at must be present")
	}

	metadata := RuntimeMetadata{
		ID:        *raw.ID,
		CreatedAt: raw.CreatedAt.UTC(),
		UpdatedAt: raw.UpdatedAt.UTC(),
	}
	if raw.Name != nil {
		metadata.Name = *raw.Name
	}

	return metadata, nil
}

func (raw rawRuntimeMember) toRuntimeMember(index int) (RuntimeMember, error) {
	switch {
	case raw.OrbitID == nil:
		return RuntimeMember{}, fmt.Errorf("members[%d].orbit_id must be present", index)
	case raw.Source == nil:
		return RuntimeMember{}, fmt.Errorf("members[%d].source must be present", index)
	case raw.AddedAt == nil:
		return RuntimeMember{}, fmt.Errorf("members[%d].added_at must be present", index)
	}

	return RuntimeMember{
		OrbitID:              *raw.OrbitID,
		Source:               *raw.Source,
		OwnerHarnessID:       valueOrEmpty(raw.OwnerHarnessID),
		AddedAt:              raw.AddedAt.UTC(),
		LastStandaloneOrigin: cloneTemplateSource(raw.LastStandaloneOrigin),
	}, nil
}

func runtimeFileNode(file RuntimeFile) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(file.SchemaVersion))
	contractutil.AppendMapping(root, "kind", contractutil.StringNode(file.Kind))

	harnessNode := contractutil.MappingNode()
	contractutil.AppendMapping(harnessNode, "id", contractutil.StringNode(file.Harness.ID))
	if file.Harness.Name != "" {
		contractutil.AppendMapping(harnessNode, "name", contractutil.StringNode(file.Harness.Name))
	}
	contractutil.AppendMapping(harnessNode, "created_at", contractutil.TimestampNode(file.Harness.CreatedAt))
	contractutil.AppendMapping(harnessNode, "updated_at", contractutil.TimestampNode(file.Harness.UpdatedAt))
	contractutil.AppendMapping(root, "harness", harnessNode)

	membersNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, member := range sortedRuntimeMembers(file.Members) {
		memberNode := contractutil.MappingNode()
		contractutil.AppendMapping(memberNode, "orbit_id", contractutil.StringNode(member.OrbitID))
		contractutil.AppendMapping(memberNode, "source", contractutil.StringNode(member.Source))
		if member.OwnerHarnessID != "" {
			contractutil.AppendMapping(memberNode, "owner_harness_id", contractutil.StringNode(member.OwnerHarnessID))
		}
		contractutil.AppendMapping(memberNode, "added_at", contractutil.TimestampNode(member.AddedAt))
		if member.LastStandaloneOrigin != nil {
			contractutil.AppendMapping(memberNode, "last_standalone_origin", templateSourceNode(*member.LastStandaloneOrigin))
		}
		membersNode.Content = append(membersNode.Content, memberNode)
	}
	contractutil.AppendMapping(root, "members", membersNode)

	return root
}

func sortedRuntimeMembers(members []RuntimeMember) []RuntimeMember {
	sorted := append([]RuntimeMember(nil), members...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OrbitID < sorted[j].OrbitID
	})

	return sorted
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
