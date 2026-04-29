package harness

import (
	"fmt"
	"os"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	templateSchemaVersion = 1
	TemplateKind          = "harness_template"
)

// TemplateManifest is the schema-backed harness template marker stored in .harness/template.yaml.
type TemplateManifest struct {
	SchemaVersion int                             `yaml:"schema_version"`
	Kind          string                          `yaml:"kind"`
	Template      TemplateMetadata                `yaml:"template"`
	Members       []TemplateMember                `yaml:"members"`
	Variables     map[string]TemplateVariableSpec `yaml:"variables"`
}

// TemplateMetadata stores harness template provenance and root guidance inclusion state.
type TemplateMetadata struct {
	HarnessID         string               `yaml:"harness_id"`
	DefaultTemplate   bool                 `yaml:"default_template"`
	CreatedFromBranch string               `yaml:"created_from_branch"`
	CreatedFromCommit string               `yaml:"created_from_commit"`
	CreatedAt         time.Time            `yaml:"created_at"`
	RootGuidance      RootGuidanceMetadata `yaml:"root_guidance"`
}

// RootGuidanceMetadata records which root guidance artifacts are carried by one harness template.
type RootGuidanceMetadata struct {
	Agents    bool `yaml:"agents" json:"agents"`
	Humans    bool `yaml:"humans" json:"humans"`
	Bootstrap bool `yaml:"bootstrap" json:"bootstrap"`
}

// TemplateMember stores one member orbit in a harness template manifest.
type TemplateMember struct {
	OrbitID string `yaml:"orbit_id"`
}

// TemplateVariableSpec captures manifest-level variable metadata only.
type TemplateVariableSpec struct {
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required"`
}

type rawTemplateManifest struct {
	SchemaVersion *int                               `yaml:"schema_version"`
	Kind          *string                            `yaml:"kind"`
	Template      *rawTemplateMetadata               `yaml:"template"`
	Members       []rawTemplateMember                `yaml:"members"`
	Variables     map[string]rawTemplateVariableSpec `yaml:"variables"`
}

type rawTemplateMetadata struct {
	HarnessID         *string                  `yaml:"harness_id"`
	DefaultTemplate   *bool                    `yaml:"default_template"`
	CreatedFromBranch *string                  `yaml:"created_from_branch"`
	CreatedFromCommit *string                  `yaml:"created_from_commit"`
	CreatedAt         *time.Time               `yaml:"created_at"`
	RootGuidance      *rawRootGuidanceMetadata `yaml:"root_guidance"`
}

type rawRootGuidanceMetadata struct {
	Agents    *bool `yaml:"agents"`
	Humans    *bool `yaml:"humans"`
	Bootstrap *bool `yaml:"bootstrap"`
}

type rawTemplateMember struct {
	OrbitID *string `yaml:"orbit_id"`
}

type rawTemplateVariableSpec struct {
	Description *string `yaml:"description"`
	Required    *bool   `yaml:"required"`
}

// LoadTemplateManifest reads, decodes, and validates .harness/template.yaml.
func LoadTemplateManifest(repoRoot string) (TemplateManifest, error) {
	return LoadTemplateManifestAtPath(TemplatePath(repoRoot))
}

// LoadTemplateManifestAtPath reads, decodes, and validates one harness template manifest.
func LoadTemplateManifestAtPath(filename string) (TemplateManifest, error) {
	//nolint:gosec // The path is repo-local and built from the fixed template contract path.
	data, err := os.ReadFile(filename)
	if err != nil {
		return TemplateManifest{}, fmt.Errorf("read %s: %w", filename, err)
	}

	manifest, err := ParseTemplateManifestData(data)
	if err != nil {
		return TemplateManifest{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return manifest, nil
}

// ParseTemplateManifestData decodes and validates .harness/template.yaml bytes.
func ParseTemplateManifestData(data []byte) (TemplateManifest, error) {
	var raw rawTemplateManifest
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return TemplateManifest{}, fmt.Errorf("decode template manifest: %w", err)
	}

	manifest, err := raw.toTemplateManifest()
	if err != nil {
		return TemplateManifest{}, err
	}

	return manifest, nil
}

// ValidateTemplateManifest validates the harness template manifest contract.
func ValidateTemplateManifest(manifest TemplateManifest) error {
	if manifest.SchemaVersion != templateSchemaVersion {
		return fmt.Errorf("schema_version must be %d", templateSchemaVersion)
	}
	if manifest.Kind != TemplateKind {
		return fmt.Errorf("kind must be %q", TemplateKind)
	}
	if err := ids.ValidateOrbitID(manifest.Template.HarnessID); err != nil {
		return fmt.Errorf("template.harness_id: %w", err)
	}
	if manifest.Template.CreatedFromBranch == "" {
		return fmt.Errorf("template.created_from_branch must not be empty")
	}
	if manifest.Template.CreatedFromCommit == "" {
		return fmt.Errorf("template.created_from_commit must not be empty")
	}
	if manifest.Template.CreatedAt.IsZero() {
		return fmt.Errorf("template.created_at must be set")
	}
	if manifest.Members == nil {
		return fmt.Errorf("members must be present")
	}
	if manifest.Variables == nil {
		return fmt.Errorf("variables must be present")
	}

	seenOrbitIDs := make(map[string]struct{}, len(manifest.Members))
	for index, member := range manifest.Members {
		if err := ids.ValidateOrbitID(member.OrbitID); err != nil {
			return fmt.Errorf("members[%d].orbit_id: %w", index, err)
		}
		if _, ok := seenOrbitIDs[member.OrbitID]; ok {
			return fmt.Errorf("members[%d].orbit_id must be unique", index)
		}
		seenOrbitIDs[member.OrbitID] = struct{}{}
	}

	for _, name := range contractutil.SortedKeys(manifest.Variables) {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("variables.%s: %w", name, err)
		}
	}

	return nil
}

// MarshalTemplateManifest validates and encodes a harness template manifest with stable ordering.
func MarshalTemplateManifest(manifest TemplateManifest) ([]byte, error) {
	if err := ValidateTemplateManifest(manifest); err != nil {
		return nil, fmt.Errorf("validate template manifest: %w", err)
	}

	data, err := contractutil.EncodeYAMLDocument(templateManifestNode(manifest))
	if err != nil {
		return nil, fmt.Errorf("encode template manifest: %w", err)
	}

	return data, nil
}

// WriteTemplateManifest validates and writes .harness/template.yaml with stable ordering.
func WriteTemplateManifest(repoRoot string, manifest TemplateManifest) (string, error) {
	return WriteTemplateManifestAtPath(TemplatePath(repoRoot), manifest)
}

// WriteTemplateManifestAtPath validates and writes one harness template manifest.
func WriteTemplateManifestAtPath(filename string, manifest TemplateManifest) (string, error) {
	data, err := MarshalTemplateManifest(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

func (raw rawTemplateManifest) toTemplateManifest() (TemplateManifest, error) {
	switch {
	case raw.SchemaVersion == nil:
		return TemplateManifest{}, fmt.Errorf("schema_version must be present")
	case raw.Kind == nil:
		return TemplateManifest{}, fmt.Errorf("kind must be present")
	case raw.Template == nil:
		return TemplateManifest{}, fmt.Errorf("template must be present")
	case raw.Members == nil:
		return TemplateManifest{}, fmt.Errorf("members must be present")
	case raw.Variables == nil:
		return TemplateManifest{}, fmt.Errorf("variables must be present")
	}

	metadata, err := raw.Template.toTemplateMetadata()
	if err != nil {
		return TemplateManifest{}, err
	}

	manifest := TemplateManifest{
		SchemaVersion: *raw.SchemaVersion,
		Kind:          *raw.Kind,
		Template:      metadata,
		Members:       make([]TemplateMember, 0, len(raw.Members)),
		Variables:     make(map[string]TemplateVariableSpec, len(raw.Variables)),
	}

	for index, rawMember := range raw.Members {
		member, err := rawMember.toTemplateMember(index)
		if err != nil {
			return TemplateManifest{}, err
		}
		manifest.Members = append(manifest.Members, member)
	}
	for name, rawSpec := range raw.Variables {
		if rawSpec.Required == nil {
			return TemplateManifest{}, fmt.Errorf("variables.%s.required must be present", name)
		}

		spec := TemplateVariableSpec{Required: *rawSpec.Required}
		if rawSpec.Description != nil {
			spec.Description = *rawSpec.Description
		}
		manifest.Variables[name] = spec
	}

	if err := ValidateTemplateManifest(manifest); err != nil {
		return TemplateManifest{}, err
	}

	return manifest, nil
}

func (raw rawTemplateMetadata) toTemplateMetadata() (TemplateMetadata, error) {
	switch {
	case raw.HarnessID == nil:
		return TemplateMetadata{}, fmt.Errorf("template.harness_id must be present")
	case raw.DefaultTemplate == nil:
		return TemplateMetadata{}, fmt.Errorf("template.default_template must be present")
	case raw.CreatedFromBranch == nil:
		return TemplateMetadata{}, fmt.Errorf("template.created_from_branch must be present")
	case raw.CreatedFromCommit == nil:
		return TemplateMetadata{}, fmt.Errorf("template.created_from_commit must be present")
	case raw.CreatedAt == nil:
		return TemplateMetadata{}, fmt.Errorf("template.created_at must be present")
	case raw.RootGuidance == nil:
		return TemplateMetadata{}, fmt.Errorf("template.root_guidance must be present")
	}

	rootGuidance, err := raw.RootGuidance.toRootGuidanceMetadata()
	if err != nil {
		return TemplateMetadata{}, err
	}

	return TemplateMetadata{
		HarnessID:         *raw.HarnessID,
		DefaultTemplate:   *raw.DefaultTemplate,
		CreatedFromBranch: *raw.CreatedFromBranch,
		CreatedFromCommit: *raw.CreatedFromCommit,
		CreatedAt:         raw.CreatedAt.UTC(),
		RootGuidance:      rootGuidance,
	}, nil
}

func (raw rawRootGuidanceMetadata) toRootGuidanceMetadata() (RootGuidanceMetadata, error) {
	switch {
	case raw.Agents == nil:
		return RootGuidanceMetadata{}, fmt.Errorf("template.root_guidance.agents must be present")
	case raw.Humans == nil:
		return RootGuidanceMetadata{}, fmt.Errorf("template.root_guidance.humans must be present")
	case raw.Bootstrap == nil:
		return RootGuidanceMetadata{}, fmt.Errorf("template.root_guidance.bootstrap must be present")
	}

	return RootGuidanceMetadata{
		Agents:    *raw.Agents,
		Humans:    *raw.Humans,
		Bootstrap: *raw.Bootstrap,
	}, nil
}

func (raw rawTemplateMember) toTemplateMember(index int) (TemplateMember, error) {
	if raw.OrbitID == nil {
		return TemplateMember{}, fmt.Errorf("members[%d].orbit_id must be present", index)
	}

	return TemplateMember{OrbitID: *raw.OrbitID}, nil
}

func templateManifestNode(manifest TemplateManifest) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(manifest.SchemaVersion))
	contractutil.AppendMapping(root, "kind", contractutil.StringNode(manifest.Kind))

	templateNode := contractutil.MappingNode()
	contractutil.AppendMapping(templateNode, "harness_id", contractutil.StringNode(manifest.Template.HarnessID))
	contractutil.AppendMapping(templateNode, "default_template", contractutil.BoolNode(manifest.Template.DefaultTemplate))
	contractutil.AppendMapping(templateNode, "created_from_branch", contractutil.StringNode(manifest.Template.CreatedFromBranch))
	contractutil.AppendMapping(templateNode, "created_from_commit", contractutil.StringNode(manifest.Template.CreatedFromCommit))
	contractutil.AppendMapping(templateNode, "created_at", contractutil.TimestampNode(manifest.Template.CreatedAt))
	rootGuidanceNode := contractutil.MappingNode()
	contractutil.AppendMapping(rootGuidanceNode, "agents", contractutil.BoolNode(manifest.Template.RootGuidance.Agents))
	contractutil.AppendMapping(rootGuidanceNode, "humans", contractutil.BoolNode(manifest.Template.RootGuidance.Humans))
	contractutil.AppendMapping(rootGuidanceNode, "bootstrap", contractutil.BoolNode(manifest.Template.RootGuidance.Bootstrap))
	contractutil.AppendMapping(templateNode, "root_guidance", rootGuidanceNode)
	contractutil.AppendMapping(root, "template", templateNode)

	membersNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, member := range sortedTemplateMembers(manifest.Members) {
		memberNode := contractutil.MappingNode()
		contractutil.AppendMapping(memberNode, "orbit_id", contractutil.StringNode(member.OrbitID))
		membersNode.Content = append(membersNode.Content, memberNode)
	}
	contractutil.AppendMapping(root, "members", membersNode)

	variablesNode := contractutil.MappingNode()
	for _, name := range contractutil.SortedKeys(manifest.Variables) {
		spec := manifest.Variables[name]
		specNode := contractutil.MappingNode()
		if spec.Description != "" {
			contractutil.AppendMapping(specNode, "description", contractutil.StringNode(spec.Description))
		}
		contractutil.AppendMapping(specNode, "required", contractutil.BoolNode(spec.Required))
		contractutil.AppendMapping(variablesNode, name, specNode)
	}
	contractutil.AppendMapping(root, "variables", variablesNode)

	return root
}

func sortedTemplateMembers(members []TemplateMember) []TemplateMember {
	sorted := append([]TemplateMember(nil), members...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OrbitID < sorted[j].OrbitID
	})

	return sorted
}
