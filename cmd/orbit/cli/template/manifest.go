package orbittemplate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	manifestRelativePath   = ".orbit/template.yaml"
	branchManifestPath     = ".harness/manifest.yaml"
	manifestSchemaVersion  = 1
	TemplateKind           = "template"
	manifestVariablesField = "variables"
	sharedFilesField       = "shared_files"

	sharedFilePathAgents            = "AGENTS.md"
	SharedFileKindAgentsFragment    = "agents_fragment"
	SharedFileMergeModeReplaceBlock = "replace-block"
)

// Manifest is the schema-backed template branch marker stored in .orbit/template.yaml.
type Manifest struct {
	SchemaVersion int                     `yaml:"schema_version"`
	Kind          string                  `yaml:"kind"`
	Template      Metadata                `yaml:"template"`
	Variables     map[string]VariableSpec `yaml:"variables"`
	SharedFiles   []SharedFileSpec        `yaml:"shared_files,omitempty"`
}

// Metadata stores template branch provenance and default-template status.
type Metadata struct {
	OrbitID           string    `yaml:"orbit_id"`
	DefaultTemplate   bool      `yaml:"default_template"`
	CreatedFromBranch string    `yaml:"created_from_branch"`
	CreatedFromCommit string    `yaml:"created_from_commit"`
	CreatedAt         time.Time `yaml:"created_at"`
}

// VariableSpec captures manifest-level variable metadata only.
type VariableSpec struct {
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required"`
}

// SharedFileSpec captures the minimal V0.2 shared-file declaration contract.
type SharedFileSpec struct {
	Path                   string `yaml:"path"`
	Kind                   string `yaml:"kind"`
	MergeMode              string `yaml:"merge_mode"`
	IncludeUnmarkedContent bool   `yaml:"include_unmarked_content"`
}

type rawManifest struct {
	SchemaVersion *int                       `yaml:"schema_version"`
	Kind          *string                    `yaml:"kind"`
	Template      *rawTemplateMetadata       `yaml:"template"`
	Variables     map[string]rawVariableSpec `yaml:"variables"`
	SharedFiles   []rawSharedFileSpec        `yaml:"shared_files"`
}

type rawTemplateMetadata struct {
	OrbitID           *string    `yaml:"orbit_id"`
	DefaultTemplate   *bool      `yaml:"default_template"`
	CreatedFromBranch *string    `yaml:"created_from_branch"`
	CreatedFromCommit *string    `yaml:"created_from_commit"`
	CreatedAt         *time.Time `yaml:"created_at"`
}

type rawVariableSpec struct {
	Description *string `yaml:"description"`
	Required    *bool   `yaml:"required"`
}

type rawSharedFileSpec struct {
	Path                   *string `yaml:"path"`
	Kind                   *string `yaml:"kind"`
	MergeMode              *string `yaml:"merge_mode"`
	IncludeUnmarkedContent *bool   `yaml:"include_unmarked_content"`
}

// ManifestPath returns the absolute path to .orbit/template.yaml.
func ManifestPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(manifestRelativePath))
}

// LoadManifest reads, decodes, and validates .orbit/template.yaml.
func LoadManifest(repoRoot string) (Manifest, error) {
	filename := ManifestPath(repoRoot)
	//nolint:gosec // The path is repo-local and built from the fixed template manifest contract path.
	data, err := os.ReadFile(filename)
	if err != nil {
		return Manifest{}, fmt.Errorf("read %s: %w", filename, err)
	}

	var raw rawManifest
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return Manifest{}, fmt.Errorf("unmarshal %s: %w", filename, err)
	}

	manifest, err := raw.toManifest()
	if err != nil {
		return Manifest{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return manifest, nil
}

// ParseManifestData decodes and validates .orbit/template.yaml bytes.
func ParseManifestData(data []byte) (Manifest, error) {
	var raw rawManifest
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	manifest, err := raw.toManifest()
	if err != nil {
		return Manifest{}, fmt.Errorf("validate manifest: %w", err)
	}

	return manifest, nil
}

// ValidateManifest validates the template manifest contract.
func ValidateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("schema_version must be %d", manifestSchemaVersion)
	}
	if manifest.Kind != TemplateKind {
		return fmt.Errorf("kind must be %q", TemplateKind)
	}
	if err := ids.ValidateOrbitID(manifest.Template.OrbitID); err != nil {
		return fmt.Errorf("template.orbit_id: %w", err)
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
	if manifest.Variables == nil {
		return fmt.Errorf("%s must be present", manifestVariablesField)
	}

	for _, name := range contractutil.SortedKeys(manifest.Variables) {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("variables.%s: %w", name, err)
		}
	}
	seenSharedPaths := make(map[string]struct{}, len(manifest.SharedFiles))
	for index, sharedFile := range manifest.SharedFiles {
		switch sharedFile.Path {
		case sharedFilePathAgents:
		default:
			return fmt.Errorf("shared_files[%d].path must be %q", index, sharedFilePathAgents)
		}
		if _, ok := seenSharedPaths[sharedFile.Path]; ok {
			return fmt.Errorf("shared_files[%d].path must be unique", index)
		}
		seenSharedPaths[sharedFile.Path] = struct{}{}

		if sharedFile.Kind != SharedFileKindAgentsFragment {
			return fmt.Errorf("shared_files[%d].kind must be %q", index, SharedFileKindAgentsFragment)
		}
		if sharedFile.MergeMode != SharedFileMergeModeReplaceBlock {
			return fmt.Errorf("shared_files[%d].merge_mode must be %q", index, SharedFileMergeModeReplaceBlock)
		}
	}

	return nil
}

// WriteManifest validates and writes .orbit/template.yaml with stable ordering.
func WriteManifest(repoRoot string, manifest Manifest) (string, error) {
	if err := ValidateManifest(manifest); err != nil {
		return "", fmt.Errorf("validate manifest: %w", err)
	}

	filename := ManifestPath(repoRoot)
	data, err := contractutil.EncodeYAMLDocument(manifestNode(manifest))
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

func branchManifestYAML(manifest Manifest) ([]byte, error) {
	if err := ValidateManifest(manifest); err != nil {
		return nil, fmt.Errorf("validate template manifest: %w", err)
	}

	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(1))
	contractutil.AppendMapping(root, "kind", contractutil.StringNode("orbit_template"))

	templateNode := contractutil.MappingNode()
	contractutil.AppendMapping(templateNode, "orbit_id", contractutil.StringNode(manifest.Template.OrbitID))
	contractutil.AppendMapping(templateNode, "default_template", contractutil.BoolNode(manifest.Template.DefaultTemplate))
	contractutil.AppendMapping(templateNode, "created_from_branch", contractutil.StringNode(manifest.Template.CreatedFromBranch))
	contractutil.AppendMapping(templateNode, "created_from_commit", contractutil.StringNode(manifest.Template.CreatedFromCommit))
	contractutil.AppendMapping(templateNode, "created_at", contractutil.TimestampNode(manifest.Template.CreatedAt))
	contractutil.AppendMapping(root, "template", templateNode)
	contractutil.AppendMapping(root, manifestVariablesField, manifestVariablesNode(manifest.Variables))

	data, err := contractutil.EncodeYAMLDocument(root)
	if err != nil {
		return nil, fmt.Errorf("encode branch manifest: %w", err)
	}

	return data, nil
}

func manifestVariablesNode(variables map[string]VariableSpec) *yaml.Node {
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

func (raw rawManifest) toManifest() (Manifest, error) {
	if raw.SchemaVersion == nil {
		return Manifest{}, fmt.Errorf("schema_version must be present")
	}
	if raw.Kind == nil {
		return Manifest{}, fmt.Errorf("kind must be present")
	}
	if raw.Template == nil {
		return Manifest{}, fmt.Errorf("template must be present")
	}
	if raw.Variables == nil {
		return Manifest{}, fmt.Errorf("%s must be present", manifestVariablesField)
	}

	templateMetadata, err := raw.Template.toTemplateMetadata()
	if err != nil {
		return Manifest{}, err
	}

	manifest := Manifest{
		SchemaVersion: *raw.SchemaVersion,
		Kind:          *raw.Kind,
		Template:      templateMetadata,
		Variables:     make(map[string]VariableSpec, len(raw.Variables)),
	}

	for name, rawSpec := range raw.Variables {
		if rawSpec.Required == nil {
			return Manifest{}, fmt.Errorf("variables.%s.required must be present", name)
		}

		spec := VariableSpec{
			Required: *rawSpec.Required,
		}
		if rawSpec.Description != nil {
			spec.Description = *rawSpec.Description
		}

		manifest.Variables[name] = spec
	}
	if len(raw.SharedFiles) > 0 {
		manifest.SharedFiles = make([]SharedFileSpec, 0, len(raw.SharedFiles))
		for index, rawSpec := range raw.SharedFiles {
			sharedFile, err := rawSpec.toSharedFileSpec(index)
			if err != nil {
				return Manifest{}, err
			}
			manifest.SharedFiles = append(manifest.SharedFiles, sharedFile)
		}
	}

	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func (raw rawTemplateMetadata) toTemplateMetadata() (Metadata, error) {
	switch {
	case raw.OrbitID == nil:
		return Metadata{}, fmt.Errorf("template.orbit_id must be present")
	case raw.DefaultTemplate == nil:
		return Metadata{}, fmt.Errorf("template.default_template must be present")
	case raw.CreatedFromBranch == nil:
		return Metadata{}, fmt.Errorf("template.created_from_branch must be present")
	case raw.CreatedFromCommit == nil:
		return Metadata{}, fmt.Errorf("template.created_from_commit must be present")
	case raw.CreatedAt == nil:
		return Metadata{}, fmt.Errorf("template.created_at must be present")
	default:
		return Metadata{
			OrbitID:           *raw.OrbitID,
			DefaultTemplate:   *raw.DefaultTemplate,
			CreatedFromBranch: *raw.CreatedFromBranch,
			CreatedFromCommit: *raw.CreatedFromCommit,
			CreatedAt:         raw.CreatedAt.UTC(),
		}, nil
	}
}

func manifestNode(manifest Manifest) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(manifest.SchemaVersion))
	contractutil.AppendMapping(root, "kind", contractutil.StringNode(manifest.Kind))

	templateNode := contractutil.MappingNode()
	contractutil.AppendMapping(templateNode, "orbit_id", contractutil.StringNode(manifest.Template.OrbitID))
	contractutil.AppendMapping(templateNode, "default_template", contractutil.BoolNode(manifest.Template.DefaultTemplate))
	contractutil.AppendMapping(templateNode, "created_from_branch", contractutil.StringNode(manifest.Template.CreatedFromBranch))
	contractutil.AppendMapping(templateNode, "created_from_commit", contractutil.StringNode(manifest.Template.CreatedFromCommit))
	contractutil.AppendMapping(templateNode, "created_at", contractutil.TimestampNode(manifest.Template.CreatedAt))
	contractutil.AppendMapping(root, "template", templateNode)

	variables := contractutil.MappingNode()
	for _, name := range contractutil.SortedKeys(manifest.Variables) {
		spec := manifest.Variables[name]
		specNode := contractutil.MappingNode()
		if spec.Description != "" {
			contractutil.AppendMapping(specNode, "description", contractutil.StringNode(spec.Description))
		}
		contractutil.AppendMapping(specNode, "required", contractutil.BoolNode(spec.Required))
		contractutil.AppendMapping(variables, name, specNode)
	}
	contractutil.AppendMapping(root, manifestVariablesField, variables)
	if len(manifest.SharedFiles) > 0 {
		sharedFiles := &yaml.Node{
			Kind: yaml.SequenceNode,
			Tag:  "!!seq",
		}
		for _, sharedFile := range manifest.SharedFiles {
			specNode := contractutil.MappingNode()
			contractutil.AppendMapping(specNode, "path", contractutil.StringNode(sharedFile.Path))
			contractutil.AppendMapping(specNode, "kind", contractutil.StringNode(sharedFile.Kind))
			contractutil.AppendMapping(specNode, "merge_mode", contractutil.StringNode(sharedFile.MergeMode))
			contractutil.AppendMapping(specNode, "include_unmarked_content", contractutil.BoolNode(sharedFile.IncludeUnmarkedContent))
			sharedFiles.Content = append(sharedFiles.Content, specNode)
		}
		contractutil.AppendMapping(root, sharedFilesField, sharedFiles)
	}

	return root
}

// SharedAgentsFile returns the AGENTS shared-file entry when the manifest declares it.
func (manifest Manifest) SharedAgentsFile() (SharedFileSpec, bool) {
	for _, sharedFile := range manifest.SharedFiles {
		if sharedFile.Path == sharedFilePathAgents {
			return sharedFile, true
		}
	}

	return SharedFileSpec{}, false
}

func (raw rawSharedFileSpec) toSharedFileSpec(index int) (SharedFileSpec, error) {
	switch {
	case raw.Path == nil:
		return SharedFileSpec{}, fmt.Errorf("shared_files[%d].path must be present", index)
	case raw.Kind == nil:
		return SharedFileSpec{}, fmt.Errorf("shared_files[%d].kind must be present", index)
	case raw.MergeMode == nil:
		return SharedFileSpec{}, fmt.Errorf("shared_files[%d].merge_mode must be present", index)
	case raw.IncludeUnmarkedContent == nil:
		return SharedFileSpec{}, fmt.Errorf("shared_files[%d].include_unmarked_content must be present", index)
	default:
		return SharedFileSpec{
			Path:                   *raw.Path,
			Kind:                   *raw.Kind,
			MergeMode:              *raw.MergeMode,
			IncludeUnmarkedContent: *raw.IncludeUnmarkedContent,
		}, nil
	}
}
