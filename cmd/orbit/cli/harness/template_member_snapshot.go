package harness

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	templateMemberSnapshotSchemaVersion = 1
	TemplateMemberSnapshotKind          = "harness_template_member_snapshot"
)

// TemplateMemberSnapshot stores one member-level harness template snapshot.
type TemplateMemberSnapshot struct {
	SchemaVersion int                        `yaml:"schema_version"`
	Kind          string                     `yaml:"kind"`
	OrbitID       string                     `yaml:"orbit_id"`
	MemberSource  string                     `yaml:"member_source"`
	Snapshot      TemplateMemberSnapshotData `yaml:"snapshot"`
}

// TemplateMemberSnapshotData stores one member's exported payload view.
type TemplateMemberSnapshotData struct {
	ExportedPaths []string                        `yaml:"exported_paths"`
	FileDigests   map[string]string               `yaml:"file_digests"`
	Variables     map[string]TemplateVariableSpec `yaml:"variables"`
}

type rawTemplateMemberSnapshot struct {
	SchemaVersion *int                           `yaml:"schema_version"`
	Kind          *string                        `yaml:"kind"`
	OrbitID       *string                        `yaml:"orbit_id"`
	MemberSource  *string                        `yaml:"member_source"`
	Snapshot      *rawTemplateMemberSnapshotData `yaml:"snapshot"`
}

type rawTemplateMemberSnapshotData struct {
	ExportedPaths []string                           `yaml:"exported_paths"`
	FileDigests   map[string]string                  `yaml:"file_digests"`
	Variables     map[string]rawTemplateVariableSpec `yaml:"variables"`
}

// ParseTemplateMemberSnapshotData decodes and validates one template-member snapshot.
func ParseTemplateMemberSnapshotData(data []byte) (TemplateMemberSnapshot, error) {
	var raw rawTemplateMemberSnapshot
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return TemplateMemberSnapshot{}, fmt.Errorf("decode template member snapshot: %w", err)
	}

	snapshot, err := raw.toTemplateMemberSnapshot()
	if err != nil {
		return TemplateMemberSnapshot{}, err
	}

	return snapshot, nil
}

// MarshalTemplateMemberSnapshot validates and encodes one template-member snapshot with stable ordering.
func MarshalTemplateMemberSnapshot(snapshot TemplateMemberSnapshot) ([]byte, error) {
	if err := ValidateTemplateMemberSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("validate template member snapshot: %w", err)
	}

	data, err := contractutil.EncodeYAMLDocument(templateMemberSnapshotNode(snapshot))
	if err != nil {
		return nil, fmt.Errorf("encode template member snapshot: %w", err)
	}

	return data, nil
}

// ValidateTemplateMemberSnapshot validates the template-member snapshot contract.
func ValidateTemplateMemberSnapshot(snapshot TemplateMemberSnapshot) error {
	if snapshot.SchemaVersion != templateMemberSnapshotSchemaVersion {
		return fmt.Errorf("schema_version must be %d", templateMemberSnapshotSchemaVersion)
	}
	if snapshot.Kind != TemplateMemberSnapshotKind {
		return fmt.Errorf("kind must be %q", TemplateMemberSnapshotKind)
	}
	if err := ids.ValidateOrbitID(snapshot.OrbitID); err != nil {
		return fmt.Errorf("orbit_id: %w", err)
	}

	switch snapshot.MemberSource {
	case MemberSourceManual, MemberSourceInstallOrbit, MemberSourceInstallBundle:
	default:
		return fmt.Errorf(
			"member_source must be one of %q, %q, or %q",
			MemberSourceManual,
			MemberSourceInstallOrbit,
			MemberSourceInstallBundle,
		)
	}

	if snapshot.Snapshot.ExportedPaths == nil {
		return fmt.Errorf("snapshot.exported_paths must be present")
	}
	if snapshot.Snapshot.FileDigests == nil {
		return fmt.Errorf("snapshot.file_digests must be present")
	}
	if snapshot.Snapshot.Variables == nil {
		return fmt.Errorf("snapshot.variables must be present")
	}

	exportedPathSet := make(map[string]struct{}, len(snapshot.Snapshot.ExportedPaths))
	for index, rawPath := range snapshot.Snapshot.ExportedPaths {
		if rawPath == "" {
			return fmt.Errorf("snapshot.exported_paths[%d] must not be empty", index)
		}
		normalizedPath, err := ids.NormalizeRepoRelativePath(rawPath)
		if err != nil {
			return fmt.Errorf("snapshot.exported_paths[%d]: %w", index, err)
		}
		if normalizedPath != rawPath {
			return fmt.Errorf("snapshot.exported_paths[%d] must be normalized", index)
		}
		if _, ok := exportedPathSet[rawPath]; ok {
			return fmt.Errorf("snapshot.exported_paths[%d] must be unique", index)
		}
		exportedPathSet[rawPath] = struct{}{}
	}

	for path, digest := range snapshot.Snapshot.FileDigests {
		if path == "" {
			return fmt.Errorf("snapshot.file_digests path must not be empty")
		}
		if _, ok := exportedPathSet[path]; !ok {
			return fmt.Errorf("snapshot.file_digests.%s must reference an exported path", path)
		}
		if strings.TrimSpace(digest) == "" {
			return fmt.Errorf("snapshot.file_digests.%s must not be empty", path)
		}
	}
	for path := range exportedPathSet {
		if _, ok := snapshot.Snapshot.FileDigests[path]; !ok {
			return fmt.Errorf("snapshot.file_digests.%s must be present", path)
		}
	}

	for _, name := range contractutil.SortedKeys(snapshot.Snapshot.Variables) {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("snapshot.variables.%s: %w", name, err)
		}
	}

	return nil
}

func (raw rawTemplateMemberSnapshot) toTemplateMemberSnapshot() (TemplateMemberSnapshot, error) {
	switch {
	case raw.SchemaVersion == nil:
		return TemplateMemberSnapshot{}, fmt.Errorf("schema_version must be present")
	case raw.Kind == nil:
		return TemplateMemberSnapshot{}, fmt.Errorf("kind must be present")
	case raw.OrbitID == nil:
		return TemplateMemberSnapshot{}, fmt.Errorf("orbit_id must be present")
	case raw.MemberSource == nil:
		return TemplateMemberSnapshot{}, fmt.Errorf("member_source must be present")
	case raw.Snapshot == nil:
		return TemplateMemberSnapshot{}, fmt.Errorf("snapshot must be present")
	}

	snapshotData, err := raw.Snapshot.toTemplateMemberSnapshotData()
	if err != nil {
		return TemplateMemberSnapshot{}, err
	}

	snapshot := TemplateMemberSnapshot{
		SchemaVersion: *raw.SchemaVersion,
		Kind:          *raw.Kind,
		OrbitID:       *raw.OrbitID,
		MemberSource:  *raw.MemberSource,
		Snapshot:      snapshotData,
	}
	if err := ValidateTemplateMemberSnapshot(snapshot); err != nil {
		return TemplateMemberSnapshot{}, err
	}

	return snapshot, nil
}

func (raw rawTemplateMemberSnapshotData) toTemplateMemberSnapshotData() (TemplateMemberSnapshotData, error) {
	if raw.ExportedPaths == nil {
		return TemplateMemberSnapshotData{}, fmt.Errorf("snapshot.exported_paths must be present")
	}
	if raw.FileDigests == nil {
		return TemplateMemberSnapshotData{}, fmt.Errorf("snapshot.file_digests must be present")
	}
	if raw.Variables == nil {
		return TemplateMemberSnapshotData{}, fmt.Errorf("snapshot.variables must be present")
	}

	data := TemplateMemberSnapshotData{
		ExportedPaths: append([]string(nil), raw.ExportedPaths...),
		FileDigests:   make(map[string]string, len(raw.FileDigests)),
		Variables:     make(map[string]TemplateVariableSpec, len(raw.Variables)),
	}
	for path, digest := range raw.FileDigests {
		data.FileDigests[path] = digest
	}
	for name, rawSpec := range raw.Variables {
		if rawSpec.Required == nil {
			return TemplateMemberSnapshotData{}, fmt.Errorf("snapshot.variables.%s.required must be present", name)
		}
		spec := TemplateVariableSpec{Required: *rawSpec.Required}
		if rawSpec.Description != nil {
			spec.Description = *rawSpec.Description
		}
		data.Variables[name] = spec
	}

	return data, nil
}

func templateMemberSnapshotNode(snapshot TemplateMemberSnapshot) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(snapshot.SchemaVersion))
	contractutil.AppendMapping(root, "kind", contractutil.StringNode(snapshot.Kind))
	contractutil.AppendMapping(root, "orbit_id", contractutil.StringNode(snapshot.OrbitID))
	contractutil.AppendMapping(root, "member_source", contractutil.StringNode(snapshot.MemberSource))

	snapshotNode := contractutil.MappingNode()
	exportedPathsNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, path := range sortedTemplateMemberSnapshotPaths(snapshot.Snapshot.ExportedPaths) {
		exportedPathsNode.Content = append(exportedPathsNode.Content, contractutil.StringNode(path))
	}
	contractutil.AppendMapping(snapshotNode, "exported_paths", exportedPathsNode)

	fileDigestsNode := contractutil.MappingNode()
	for _, path := range contractutil.SortedKeys(snapshot.Snapshot.FileDigests) {
		contractutil.AppendMapping(fileDigestsNode, path, contractutil.StringNode(snapshot.Snapshot.FileDigests[path]))
	}
	contractutil.AppendMapping(snapshotNode, "file_digests", fileDigestsNode)

	variablesNode := contractutil.MappingNode()
	for _, name := range contractutil.SortedKeys(snapshot.Snapshot.Variables) {
		spec := snapshot.Snapshot.Variables[name]
		specNode := contractutil.MappingNode()
		if spec.Description != "" {
			contractutil.AppendMapping(specNode, "description", contractutil.StringNode(spec.Description))
		}
		contractutil.AppendMapping(specNode, "required", contractutil.BoolNode(spec.Required))
		contractutil.AppendMapping(variablesNode, name, specNode)
	}
	contractutil.AppendMapping(snapshotNode, "variables", variablesNode)
	contractutil.AppendMapping(root, "snapshot", snapshotNode)

	return root
}

func sortedTemplateMemberSnapshotPaths(paths []string) []string {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	return sorted
}
