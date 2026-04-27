package orbittemplate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	installRecordDirName         = ".orbit/installs"
	installRecordSchemaVersion   = 1
	InstallSourceKindLocalBranch = "local_branch"
	InstallSourceKindExternalGit = "external_git"
	InstallRecordStatusActive    = "active"
	InstallRecordStatusDetached  = "detached"
)

const legacyInstallSourceKindRemoteGit = "remote_git"

// InstallRecord stores the schema-backed template source for one orbit installation.
type InstallRecord struct {
	SchemaVersion int                       `yaml:"schema_version"`
	OrbitID       string                    `yaml:"orbit_id"`
	Status        string                    `yaml:"status,omitempty"`
	Template      Source                    `yaml:"template"`
	AppliedAt     time.Time                 `yaml:"applied_at"`
	Variables     *InstallVariablesSnapshot `yaml:"variables,omitempty"`
	AgentAddons   *AgentAddonsSnapshot      `yaml:"agent_addons,omitempty"`
}

// Source captures where the installed template came from.
type Source struct {
	SourceKind     string `yaml:"source_kind"`
	SourceRepo     string `yaml:"source_repo"`
	SourceRef      string `yaml:"source_ref"`
	TemplateCommit string `yaml:"template_commit"`
}

// InstallVariablesSnapshot stores the variable contract and values that produced one install.
type InstallVariablesSnapshot struct {
	Declarations              map[string]bindings.VariableDeclaration `yaml:"declarations,omitempty"`
	Namespaces                map[string]string                       `yaml:"namespaces,omitempty"`
	ResolvedAtApply           map[string]bindings.VariableBinding     `yaml:"resolved_at_apply,omitempty"`
	UnresolvedAtApply         []string                                `yaml:"unresolved_at_apply,omitempty"`
	ObservedRuntimeUnresolved []string                                `yaml:"observed_runtime_unresolved,omitempty"`
}

type rawInstallRecord struct {
	SchemaVersion *int                         `yaml:"schema_version"`
	OrbitID       *string                      `yaml:"orbit_id"`
	Status        *string                      `yaml:"status"`
	Template      *rawTemplateSource           `yaml:"template"`
	AppliedAt     *time.Time                   `yaml:"applied_at"`
	Variables     *rawInstallVariablesSnapshot `yaml:"variables"`
	AgentAddons   *AgentAddonsSnapshot         `yaml:"agent_addons"`
}

type rawTemplateSource struct {
	SourceKind     *string `yaml:"source_kind"`
	SourceRepo     *string `yaml:"source_repo"`
	SourceRef      *string `yaml:"source_ref"`
	TemplateCommit *string `yaml:"template_commit"`
}

type rawInstallVariablesSnapshot struct {
	Declarations              map[string]rawInstallVariableDeclaration `yaml:"declarations"`
	Namespaces                map[string]string                        `yaml:"namespaces"`
	ResolvedAtApply           map[string]rawInstallVariableBinding     `yaml:"resolved_at_apply"`
	UnresolvedAtApply         []string                                 `yaml:"unresolved_at_apply"`
	ObservedRuntimeUnresolved []string                                 `yaml:"observed_runtime_unresolved"`
}

type rawInstallVariableDeclaration struct {
	Description *string `yaml:"description"`
	Required    *bool   `yaml:"required"`
}

type rawInstallVariableBinding struct {
	Value       *string `yaml:"value"`
	Description *string `yaml:"description"`
}

// InstallRecordPath returns the absolute path to one install record.
func InstallRecordPath(repoRoot string, orbitID string) (string, error) {
	repoPath, err := InstallRecordRepoPath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(repoPath)), nil
}

// InstallRecordRepoPath returns the repository-relative path to one install record.
func InstallRecordRepoPath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return installRecordDirName + "/" + orbitID + ".yaml", nil
}

// LoadInstallRecord reads, decodes, and validates one install record from the fixed Phase 2 host path.
func LoadInstallRecord(repoRoot string, orbitID string) (InstallRecord, error) {
	filename, err := InstallRecordPath(repoRoot, orbitID)
	if err != nil {
		return InstallRecord{}, fmt.Errorf("build install record path: %w", err)
	}

	record, err := LoadInstallRecordFile(filename)
	if err != nil {
		return InstallRecord{}, err
	}
	if record.OrbitID != orbitID {
		return InstallRecord{}, fmt.Errorf("validate %s: orbit_id must match install path", filename)
	}

	return record, nil
}

// LoadInstallRecordFile reads, decodes, and validates one install record from an absolute path.
func LoadInstallRecordFile(filename string) (InstallRecord, error) {
	//nolint:gosec // The path is repo-local and built from a validated orbit id.
	data, err := os.ReadFile(filename)
	if err != nil {
		return InstallRecord{}, fmt.Errorf("read %s: %w", filename, err)
	}

	record, err := ParseInstallRecordData(data)
	if err != nil {
		return InstallRecord{}, fmt.Errorf("parse %s: %w", filename, err)
	}

	return record, nil
}

// ParseInstallRecordData decodes and validates install-record bytes.
func ParseInstallRecordData(data []byte) (InstallRecord, error) {
	var raw rawInstallRecord
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return InstallRecord{}, fmt.Errorf("decode install record: %w", err)
	}

	record, err := raw.toInstallRecord()
	if err != nil {
		return InstallRecord{}, fmt.Errorf("validate install record: %w", err)
	}

	return record, nil
}

// ValidateInstallRecord validates the install-record schema contract.
func ValidateInstallRecord(record InstallRecord) error {
	if record.SchemaVersion != installRecordSchemaVersion {
		return fmt.Errorf("schema_version must be %d", installRecordSchemaVersion)
	}
	if err := ids.ValidateOrbitID(record.OrbitID); err != nil {
		return fmt.Errorf("orbit_id: %w", err)
	}
	switch normalizedInstallRecordStatus(record.Status) {
	case InstallRecordStatusActive, InstallRecordStatusDetached:
	default:
		return fmt.Errorf("status must be one of %q or %q", InstallRecordStatusActive, InstallRecordStatusDetached)
	}

	switch normalizeInstallSourceKind(record.Template.SourceKind) {
	case InstallSourceKindLocalBranch:
	case InstallSourceKindExternalGit:
		if record.Template.SourceRepo == "" {
			return fmt.Errorf("template.source_repo must not be empty for %q", InstallSourceKindExternalGit)
		}
	default:
		return fmt.Errorf("template.source_kind must be one of %q or %q", InstallSourceKindLocalBranch, InstallSourceKindExternalGit)
	}

	if record.Template.SourceRef == "" {
		return fmt.Errorf("template.source_ref must not be empty")
	}
	if record.Template.TemplateCommit == "" {
		return fmt.Errorf("template.template_commit must not be empty")
	}
	if record.AppliedAt.IsZero() {
		return fmt.Errorf("applied_at must be set")
	}
	if record.Variables != nil {
		if err := validateInstallVariablesSnapshot(*record.Variables); err != nil {
			return fmt.Errorf("variables: %w", err)
		}
	}
	if record.AgentAddons != nil {
		if err := ValidateAgentAddonsSnapshot(*record.AgentAddons); err != nil {
			return fmt.Errorf("agent_addons: %w", err)
		}
	}

	return nil
}

// EffectiveInstallRecordStatus returns the runtime status for one install record, defaulting missing status to active.
func EffectiveInstallRecordStatus(record InstallRecord) string {
	return normalizedInstallRecordStatus(record.Status)
}

// ValidateInstallVariablesSnapshot validates the install-record variables schema contract.
func ValidateInstallVariablesSnapshot(snapshot InstallVariablesSnapshot) error {
	return validateInstallVariablesSnapshot(snapshot)
}

// WriteInstallRecord validates and writes one install record with stable ordering to the fixed Phase 2 host path.
func WriteInstallRecord(repoRoot string, record InstallRecord) (string, error) {
	filename, err := InstallRecordPath(repoRoot, record.OrbitID)
	if err != nil {
		return "", fmt.Errorf("build install record path: %w", err)
	}

	return WriteInstallRecordFile(filename, record)
}

// WriteInstallRecordFile validates and writes one install record with stable ordering.
func WriteInstallRecordFile(filename string, record InstallRecord) (string, error) {
	record = canonicalizeInstallRecord(record)

	if err := ValidateInstallRecord(record); err != nil {
		return "", fmt.Errorf("validate install record: %w", err)
	}

	data, err := contractutil.EncodeYAMLDocument(installRecordNode(record))
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

func (raw rawInstallRecord) toInstallRecord() (InstallRecord, error) {
	switch {
	case raw.SchemaVersion == nil:
		return InstallRecord{}, fmt.Errorf("schema_version must be present")
	case raw.OrbitID == nil:
		return InstallRecord{}, fmt.Errorf("orbit_id must be present")
	case raw.Template == nil:
		return InstallRecord{}, fmt.Errorf("template must be present")
	case raw.AppliedAt == nil:
		return InstallRecord{}, fmt.Errorf("applied_at must be present")
	}

	templateSource, err := raw.Template.toTemplateSource()
	if err != nil {
		return InstallRecord{}, err
	}

	record := InstallRecord{
		SchemaVersion: *raw.SchemaVersion,
		OrbitID:       *raw.OrbitID,
		Template:      templateSource,
		AppliedAt:     raw.AppliedAt.UTC(),
	}
	if raw.Status != nil {
		record.Status = strings.TrimSpace(*raw.Status)
	}
	if raw.Variables != nil {
		snapshot, err := raw.Variables.toInstallVariablesSnapshot()
		if err != nil {
			return InstallRecord{}, err
		}
		record.Variables = &snapshot
	}
	if raw.AgentAddons != nil {
		record.AgentAddons = raw.AgentAddons
	}

	if err := ValidateInstallRecord(record); err != nil {
		return InstallRecord{}, err
	}

	return record, nil
}

func (raw rawTemplateSource) toTemplateSource() (Source, error) {
	switch {
	case raw.SourceKind == nil:
		return Source{}, fmt.Errorf("template.source_kind must be present")
	case raw.SourceRepo == nil:
		return Source{}, fmt.Errorf("template.source_repo must be present")
	case raw.SourceRef == nil:
		return Source{}, fmt.Errorf("template.source_ref must be present")
	case raw.TemplateCommit == nil:
		return Source{}, fmt.Errorf("template.template_commit must be present")
	default:
		return Source{
			SourceKind:     normalizeInstallSourceKind(*raw.SourceKind),
			SourceRepo:     *raw.SourceRepo,
			SourceRef:      *raw.SourceRef,
			TemplateCommit: *raw.TemplateCommit,
		}, nil
	}
}

func normalizeInstallSourceKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case legacyInstallSourceKindRemoteGit:
		return InstallSourceKindExternalGit
	default:
		return strings.TrimSpace(kind)
	}
}

func canonicalizeInstallRecord(record InstallRecord) InstallRecord {
	record.Template.SourceKind = normalizeInstallSourceKind(record.Template.SourceKind)
	return record
}

func (raw rawInstallVariablesSnapshot) toInstallVariablesSnapshot() (InstallVariablesSnapshot, error) {
	snapshot := InstallVariablesSnapshot{
		Declarations:    map[string]bindings.VariableDeclaration{},
		ResolvedAtApply: map[string]bindings.VariableBinding{},
	}
	for name, declaration := range raw.Declarations {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return InstallVariablesSnapshot{}, fmt.Errorf("declarations.%s: %w", name, err)
		}
		if declaration.Required == nil {
			return InstallVariablesSnapshot{}, fmt.Errorf("declarations.%s.required must be present", name)
		}
		next := bindings.VariableDeclaration{
			Required: *declaration.Required,
		}
		if declaration.Description != nil {
			next.Description = *declaration.Description
		}
		snapshot.Declarations[name] = next
	}
	for name, binding := range raw.ResolvedAtApply {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return InstallVariablesSnapshot{}, fmt.Errorf("resolved_at_apply.%s: %w", name, err)
		}
		if binding.Value == nil {
			return InstallVariablesSnapshot{}, fmt.Errorf("resolved_at_apply.%s.value must be present", name)
		}
		next := bindings.VariableBinding{
			Value: *binding.Value,
		}
		if binding.Description != nil {
			next.Description = *binding.Description
		}
		snapshot.ResolvedAtApply[name] = next
	}
	if raw.Namespaces != nil {
		snapshot.Namespaces = make(map[string]string, len(raw.Namespaces))
		for name, namespace := range raw.Namespaces {
			if err := contractutil.ValidateVariableName(name); err != nil {
				return InstallVariablesSnapshot{}, fmt.Errorf("namespaces.%s: %w", name, err)
			}
			if err := ids.ValidateOrbitID(namespace); err != nil {
				return InstallVariablesSnapshot{}, fmt.Errorf("namespaces.%s: %w", name, err)
			}
			snapshot.Namespaces[name] = namespace
		}
	}
	if len(raw.UnresolvedAtApply) > 0 {
		snapshot.UnresolvedAtApply = append([]string(nil), raw.UnresolvedAtApply...)
	}
	if len(raw.ObservedRuntimeUnresolved) > 0 {
		snapshot.ObservedRuntimeUnresolved = append([]string(nil), raw.ObservedRuntimeUnresolved...)
	}

	if err := validateInstallVariablesSnapshot(snapshot); err != nil {
		return InstallVariablesSnapshot{}, err
	}

	return snapshot, nil
}

func validateInstallVariablesSnapshot(snapshot InstallVariablesSnapshot) error {
	for _, name := range contractutil.SortedKeys(snapshot.Declarations) {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("declarations.%s: %w", name, err)
		}
	}
	for _, name := range contractutil.SortedKeys(snapshot.Namespaces) {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("namespaces.%s: %w", name, err)
		}
		if _, ok := snapshot.Declarations[name]; !ok {
			return fmt.Errorf("namespaces.%s must reference a declared variable", name)
		}
		if err := ids.ValidateOrbitID(snapshot.Namespaces[name]); err != nil {
			return fmt.Errorf("namespaces.%s: %w", name, err)
		}
	}
	for _, name := range contractutil.SortedKeys(snapshot.ResolvedAtApply) {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("resolved_at_apply.%s: %w", name, err)
		}
		if _, ok := snapshot.Declarations[name]; !ok {
			return fmt.Errorf("resolved_at_apply.%s must reference a declared variable", name)
		}
	}
	for _, name := range snapshot.UnresolvedAtApply {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("unresolved_at_apply: %w", err)
		}
		if _, ok := snapshot.Declarations[name]; !ok {
			return fmt.Errorf("unresolved_at_apply.%s must reference a declared variable", name)
		}
	}
	for _, name := range snapshot.ObservedRuntimeUnresolved {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("observed_runtime_unresolved: %w", err)
		}
	}

	return nil
}

func installRecordNode(record InstallRecord) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(record.SchemaVersion))
	contractutil.AppendMapping(root, "orbit_id", contractutil.StringNode(record.OrbitID))
	if status := strings.TrimSpace(record.Status); status != "" {
		contractutil.AppendMapping(root, "status", contractutil.StringNode(status))
	}

	templateNode := contractutil.MappingNode()
	contractutil.AppendMapping(templateNode, "source_kind", contractutil.StringNode(record.Template.SourceKind))
	contractutil.AppendMapping(templateNode, "source_repo", contractutil.StringNode(record.Template.SourceRepo))
	contractutil.AppendMapping(templateNode, "source_ref", contractutil.StringNode(record.Template.SourceRef))
	contractutil.AppendMapping(templateNode, "template_commit", contractutil.StringNode(record.Template.TemplateCommit))
	contractutil.AppendMapping(root, "template", templateNode)

	contractutil.AppendMapping(root, "applied_at", contractutil.TimestampNode(record.AppliedAt))
	if record.Variables != nil {
		contractutil.AppendMapping(root, "variables", installVariablesSnapshotNode(*record.Variables))
	}
	if record.AgentAddons != nil {
		contractutil.AppendMapping(root, "agent_addons", AgentAddonsSnapshotNode(*record.AgentAddons))
	}

	return root
}

func installVariablesSnapshotNode(snapshot InstallVariablesSnapshot) *yaml.Node {
	root := contractutil.MappingNode()

	declarationsNode := contractutil.MappingNode()
	for _, name := range contractutil.SortedKeys(snapshot.Declarations) {
		declaration := snapshot.Declarations[name]
		declarationNode := contractutil.MappingNode()
		if declaration.Description != "" {
			contractutil.AppendMapping(declarationNode, "description", contractutil.StringNode(declaration.Description))
		}
		contractutil.AppendMapping(declarationNode, "required", contractutil.BoolNode(declaration.Required))
		contractutil.AppendMapping(declarationsNode, name, declarationNode)
	}
	contractutil.AppendMapping(root, "declarations", declarationsNode)

	if len(snapshot.Namespaces) > 0 {
		namespacesNode := contractutil.MappingNode()
		for _, name := range contractutil.SortedKeys(snapshot.Namespaces) {
			contractutil.AppendMapping(namespacesNode, name, contractutil.StringNode(snapshot.Namespaces[name]))
		}
		contractutil.AppendMapping(root, "namespaces", namespacesNode)
	}

	resolvedNode := contractutil.MappingNode()
	for _, name := range contractutil.SortedKeys(snapshot.ResolvedAtApply) {
		binding := snapshot.ResolvedAtApply[name]
		bindingNode := contractutil.MappingNode()
		contractutil.AppendMapping(bindingNode, "value", contractutil.StringNode(binding.Value))
		if binding.Description != "" {
			contractutil.AppendMapping(bindingNode, "description", contractutil.StringNode(binding.Description))
		}
		contractutil.AppendMapping(resolvedNode, name, bindingNode)
	}
	contractutil.AppendMapping(root, "resolved_at_apply", resolvedNode)

	unresolvedNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	values := append([]string(nil), snapshot.UnresolvedAtApply...)
	sort.Strings(values)
	for _, name := range values {
		unresolvedNode.Content = append(unresolvedNode.Content, contractutil.StringNode(name))
	}
	contractutil.AppendMapping(root, "unresolved_at_apply", unresolvedNode)

	observedNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	observed := append([]string(nil), snapshot.ObservedRuntimeUnresolved...)
	sort.Strings(observed)
	for _, name := range observed {
		observedNode.Content = append(observedNode.Content, contractutil.StringNode(name))
	}
	contractutil.AppendMapping(root, "observed_runtime_unresolved", observedNode)

	return root
}

// InstallVariablesSnapshotNode returns a stable YAML node for an install variables snapshot.
func InstallVariablesSnapshotNode(snapshot InstallVariablesSnapshot) *yaml.Node {
	return installVariablesSnapshotNode(snapshot)
}

func normalizedInstallRecordStatus(status string) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return InstallRecordStatusActive
	}

	return trimmed
}
