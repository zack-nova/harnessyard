package harness

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
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const bundleRecordSchemaVersion = 1

// BundleRecord stores one harness template bundle installation provenance record.
type BundleRecord struct {
	SchemaVersion        int                                     `yaml:"schema_version"`
	HarnessID            string                                  `yaml:"harness_id"`
	Template             orbittemplate.Source                    `yaml:"template"`
	RecommendedFramework string                                  `yaml:"recommended_framework,omitempty"`
	AgentConfig          *AgentConfigFile                        `yaml:"agent_config,omitempty"`
	AgentOverlays        map[string]string                       `yaml:"agent_overlays,omitempty"`
	AgentAddons          *orbittemplate.AgentAddonsSnapshot      `yaml:"agent_addons,omitempty"`
	MemberIDs            []string                                `yaml:"member_ids"`
	AppliedAt            time.Time                               `yaml:"applied_at"`
	IncludesRootAgents   bool                                    `yaml:"includes_root_agents"`
	OwnedPaths           []string                                `yaml:"owned_paths"`
	OwnedPathDigests     map[string]string                       `yaml:"owned_path_digests,omitempty"`
	RootAgentsDigest     string                                  `yaml:"root_agents_digest,omitempty"`
	Variables            *orbittemplate.InstallVariablesSnapshot `yaml:"variables,omitempty"`
}

type rawBundleRecord struct {
	SchemaVersion        *int                               `yaml:"schema_version"`
	HarnessID            *string                            `yaml:"harness_id"`
	Template             *rawBundleTemplateSource           `yaml:"template"`
	RecommendedFramework *string                            `yaml:"recommended_framework"`
	AgentConfig          *rawAgentConfigFile                `yaml:"agent_config"`
	AgentOverlays        map[string]string                  `yaml:"agent_overlays"`
	AgentAddons          *orbittemplate.AgentAddonsSnapshot `yaml:"agent_addons"`
	MemberIDs            *[]string                          `yaml:"member_ids"`
	AppliedAt            *time.Time                         `yaml:"applied_at"`
	IncludesRootAgents   *bool                              `yaml:"includes_root_agents"`
	OwnedPaths           *[]string                          `yaml:"owned_paths"`
	OwnedPathDigests     map[string]string                  `yaml:"owned_path_digests"`
	RootAgentsDigest     *string                            `yaml:"root_agents_digest"`
	Variables            *rawBundleVariablesSnapshot        `yaml:"variables"`
}

type rawBundleTemplateSource struct {
	SourceKind     *string `yaml:"source_kind"`
	SourceRepo     *string `yaml:"source_repo"`
	SourceRef      *string `yaml:"source_ref"`
	TemplateCommit *string `yaml:"template_commit"`
}

type rawBundleVariablesSnapshot struct {
	Declarations              map[string]rawBundleVariableDeclaration `yaml:"declarations"`
	Namespaces                map[string]string                       `yaml:"namespaces"`
	ResolvedAtApply           map[string]rawBundleVariableBinding     `yaml:"resolved_at_apply"`
	UnresolvedAtApply         []string                                `yaml:"unresolved_at_apply"`
	ObservedRuntimeUnresolved []string                                `yaml:"observed_runtime_unresolved"`
}

type rawBundleVariableDeclaration struct {
	Description *string `yaml:"description"`
	Required    *bool   `yaml:"required"`
}

type rawBundleVariableBinding struct {
	Value       *string `yaml:"value"`
	Description *string `yaml:"description"`
}

// LoadBundleRecord reads, decodes, and validates one bundle record from the fixed host path.
func LoadBundleRecord(repoRoot string, harnessID string) (BundleRecord, error) {
	filename, err := BundleRecordPath(repoRoot, harnessID)
	if err != nil {
		return BundleRecord{}, fmt.Errorf("build bundle record path: %w", err)
	}

	record, err := LoadBundleRecordFile(filename)
	if err != nil {
		return BundleRecord{}, err
	}
	if record.HarnessID != harnessID {
		return BundleRecord{}, fmt.Errorf("validate %s: harness_id must match bundle path", filename)
	}

	return record, nil
}

// LoadBundleRecordFile reads, decodes, and validates one bundle record from an absolute path.
func LoadBundleRecordFile(filename string) (BundleRecord, error) {
	//nolint:gosec // Path is repo-local and built from a validated harness id.
	data, err := os.ReadFile(filename)
	if err != nil {
		return BundleRecord{}, fmt.Errorf("read %s: %w", filename, err)
	}

	record, err := ParseBundleRecordData(data)
	if err != nil {
		return BundleRecord{}, fmt.Errorf("parse %s: %w", filename, err)
	}

	return record, nil
}

// ParseBundleRecordData decodes and validates bundle-record bytes.
func ParseBundleRecordData(data []byte) (BundleRecord, error) {
	var raw rawBundleRecord
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return BundleRecord{}, fmt.Errorf("decode bundle record: %w", err)
	}

	record, err := raw.toBundleRecord()
	if err != nil {
		return BundleRecord{}, fmt.Errorf("validate bundle record: %w", err)
	}

	return record, nil
}

// ValidateBundleRecord validates the bundle-record schema contract.
func ValidateBundleRecord(record BundleRecord) error {
	if record.SchemaVersion != bundleRecordSchemaVersion {
		return fmt.Errorf("schema_version must be %d", bundleRecordSchemaVersion)
	}
	if err := ids.ValidateOrbitID(record.HarnessID); err != nil {
		return fmt.Errorf("harness_id: %w", err)
	}
	if err := orbittemplate.ValidateInstallRecord(orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       record.HarnessID,
		Template:      record.Template,
		AppliedAt:     record.AppliedAt,
		Variables:     record.Variables,
	}); err != nil {
		// Re-wrap to keep bundle schema field names stable.
		message := err.Error()
		message = stringsReplacePrefix(message, "orbit_id:", "harness_id:")
		message = stringsReplacePrefix(message, "template.", "template.")
		return fmt.Errorf("%s", message)
	}
	if record.MemberIDs == nil {
		return fmt.Errorf("member_ids must be present")
	}
	if len(record.MemberIDs) == 0 {
		return fmt.Errorf("member_ids must not be empty")
	}
	if strings.TrimSpace(record.RecommendedFramework) != "" {
		if err := ids.ValidateOrbitID(record.RecommendedFramework); err != nil {
			return fmt.Errorf("recommended_framework: %w", err)
		}
	}
	if record.AgentConfig != nil {
		if err := ValidateAgentConfigFile(*record.AgentConfig); err != nil {
			return fmt.Errorf("agent_config: %w", err)
		}
	}
	for agentID, content := range record.AgentOverlays {
		if err := validateAgentOverlayID(agentID); err != nil {
			return fmt.Errorf("agent_overlays.%s: %w", agentID, err)
		}
		file, err := ParseAgentOverlayFileData([]byte(content))
		if err != nil {
			return fmt.Errorf("agent_overlays.%s: %w", agentID, err)
		}
		if err := ValidateAgentOverlayFile(file); err != nil {
			return fmt.Errorf("agent_overlays.%s: %w", agentID, err)
		}
	}
	if record.AgentAddons != nil {
		if err := orbittemplate.ValidateAgentAddonsSnapshot(*record.AgentAddons); err != nil {
			return fmt.Errorf("agent_addons: %w", err)
		}
	}
	if record.AppliedAt.IsZero() {
		return fmt.Errorf("applied_at must be set")
	}

	seenMemberIDs := make(map[string]struct{}, len(record.MemberIDs))
	for index, memberID := range record.MemberIDs {
		if err := ids.ValidateOrbitID(memberID); err != nil {
			return fmt.Errorf("member_ids[%d]: %w", index, err)
		}
		if _, ok := seenMemberIDs[memberID]; ok {
			return fmt.Errorf("member_ids[%d] must be unique", index)
		}
		seenMemberIDs[memberID] = struct{}{}
	}

	ownedPathSet := make(map[string]struct{}, len(record.OwnedPaths))
	rootAgentsOwned := false
	for index, ownedPath := range record.OwnedPaths {
		if ownedPath == "" {
			return fmt.Errorf("owned_paths[%d] must not be empty", index)
		}
		ownedPathSet[ownedPath] = struct{}{}
		if ownedPath == rootAgentsPath {
			rootAgentsOwned = true
		}
	}
	for path, digest := range record.OwnedPathDigests {
		if path == "" {
			return fmt.Errorf("owned_path_digests path must not be empty")
		}
		if path == rootAgentsPath {
			return fmt.Errorf("owned_path_digests.%s must use root_agents_digest", path)
		}
		if _, ok := ownedPathSet[path]; !ok {
			return fmt.Errorf("owned_path_digests.%s must reference an owned path", path)
		}
		if strings.TrimSpace(digest) == "" {
			return fmt.Errorf("owned_path_digests.%s must not be empty", path)
		}
	}
	if strings.TrimSpace(record.RootAgentsDigest) != "" && !rootAgentsOwned {
		return fmt.Errorf("root_agents_digest requires AGENTS.md in owned_paths")
	}

	return nil
}

// WriteBundleRecord validates and writes one bundle record with stable ordering to the fixed host path.
func WriteBundleRecord(repoRoot string, record BundleRecord) (string, error) {
	filename, err := BundleRecordPath(repoRoot, record.HarnessID)
	if err != nil {
		return "", fmt.Errorf("build bundle record path: %w", err)
	}

	return WriteBundleRecordFile(filename, record)
}

// WriteBundleRecordFile validates and writes one bundle record with stable ordering.
func WriteBundleRecordFile(filename string, record BundleRecord) (string, error) {
	if err := ValidateBundleRecord(record); err != nil {
		return "", fmt.Errorf("validate bundle record: %w", err)
	}

	data, err := contractutil.EncodeYAMLDocument(bundleRecordNode(record))
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

// ListBundleRecordIDs returns valid bundle ids present under .harness/bundles.
func ListBundleRecordIDs(repoRoot string) ([]string, error) {
	entries, err := os.ReadDir(BundleRecordsDirPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read harness bundle records directory: %w", err)
	}

	idsList := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		harnessID := entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]
		record, err := LoadBundleRecord(repoRoot, harnessID)
		if err != nil {
			continue
		}
		idsList = append(idsList, record.HarnessID)
	}
	sort.Strings(idsList)

	return idsList, nil
}

func (raw rawBundleRecord) toBundleRecord() (BundleRecord, error) {
	switch {
	case raw.SchemaVersion == nil:
		return BundleRecord{}, fmt.Errorf("schema_version must be present")
	case raw.HarnessID == nil:
		return BundleRecord{}, fmt.Errorf("harness_id must be present")
	case raw.Template == nil:
		return BundleRecord{}, fmt.Errorf("template must be present")
	case raw.MemberIDs == nil:
		return BundleRecord{}, fmt.Errorf("member_ids must be present")
	case raw.AppliedAt == nil:
		return BundleRecord{}, fmt.Errorf("applied_at must be present")
	case raw.IncludesRootAgents == nil:
		return BundleRecord{}, fmt.Errorf("includes_root_agents must be present")
	}

	templateSource, err := raw.Template.toTemplateSource()
	if err != nil {
		return BundleRecord{}, err
	}

	record := BundleRecord{
		SchemaVersion:        *raw.SchemaVersion,
		HarnessID:            *raw.HarnessID,
		Template:             templateSource,
		RecommendedFramework: "",
		MemberIDs:            append([]string(nil), (*raw.MemberIDs)...),
		AppliedAt:            raw.AppliedAt.UTC(),
		IncludesRootAgents:   *raw.IncludesRootAgents,
		OwnedPaths:           []string{},
	}
	if raw.OwnedPaths != nil {
		record.OwnedPaths = append(record.OwnedPaths, (*raw.OwnedPaths)...)
	}
	if raw.RecommendedFramework != nil {
		record.RecommendedFramework = strings.TrimSpace(*raw.RecommendedFramework)
	}
	if raw.AgentConfig != nil {
		if raw.AgentConfig.SchemaVersion == nil {
			return BundleRecord{}, fmt.Errorf("agent_config.schema_version must be present")
		}
		record.AgentConfig = &AgentConfigFile{
			SchemaVersion: *raw.AgentConfig.SchemaVersion,
		}
	}
	if raw.AgentOverlays != nil {
		record.AgentOverlays = make(map[string]string, len(raw.AgentOverlays))
		for agentID, content := range raw.AgentOverlays {
			record.AgentOverlays[agentID] = content
		}
	}
	if raw.AgentAddons != nil {
		record.AgentAddons = raw.AgentAddons
	}
	if raw.OwnedPathDigests != nil {
		record.OwnedPathDigests = make(map[string]string, len(raw.OwnedPathDigests))
		for path, digest := range raw.OwnedPathDigests {
			record.OwnedPathDigests[path] = digest
		}
	}
	if raw.RootAgentsDigest != nil {
		record.RootAgentsDigest = *raw.RootAgentsDigest
	}
	if raw.Variables != nil {
		snapshot, err := raw.Variables.toInstallVariablesSnapshot()
		if err != nil {
			return BundleRecord{}, err
		}
		record.Variables = &snapshot
	}

	if err := ValidateBundleRecord(record); err != nil {
		return BundleRecord{}, err
	}

	return record, nil
}

func (raw rawBundleTemplateSource) toTemplateSource() (orbittemplate.Source, error) {
	switch {
	case raw.SourceKind == nil:
		return orbittemplate.Source{}, fmt.Errorf("template.source_kind must be present")
	case raw.SourceRepo == nil:
		return orbittemplate.Source{}, fmt.Errorf("template.source_repo must be present")
	case raw.SourceRef == nil:
		return orbittemplate.Source{}, fmt.Errorf("template.source_ref must be present")
	case raw.TemplateCommit == nil:
		return orbittemplate.Source{}, fmt.Errorf("template.template_commit must be present")
	default:
		return orbittemplate.Source{
			SourceKind:     *raw.SourceKind,
			SourceRepo:     *raw.SourceRepo,
			SourceRef:      *raw.SourceRef,
			TemplateCommit: *raw.TemplateCommit,
		}, nil
	}
}

func (raw rawBundleVariablesSnapshot) toInstallVariablesSnapshot() (orbittemplate.InstallVariablesSnapshot, error) {
	snapshot := orbittemplate.InstallVariablesSnapshot{
		Declarations:    map[string]bindings.VariableDeclaration{},
		ResolvedAtApply: map[string]bindings.VariableBinding{},
	}
	for name, declaration := range raw.Declarations {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return orbittemplate.InstallVariablesSnapshot{}, fmt.Errorf("declarations.%s: %w", name, err)
		}
		if declaration.Required == nil {
			return orbittemplate.InstallVariablesSnapshot{}, fmt.Errorf("declarations.%s.required must be present", name)
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
			return orbittemplate.InstallVariablesSnapshot{}, fmt.Errorf("resolved_at_apply.%s: %w", name, err)
		}
		if binding.Value == nil {
			return orbittemplate.InstallVariablesSnapshot{}, fmt.Errorf("resolved_at_apply.%s.value must be present", name)
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
				return orbittemplate.InstallVariablesSnapshot{}, fmt.Errorf("namespaces.%s: %w", name, err)
			}
			if err := ids.ValidateOrbitID(namespace); err != nil {
				return orbittemplate.InstallVariablesSnapshot{}, fmt.Errorf("namespaces.%s: %w", name, err)
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
	if err := orbittemplate.ValidateInstallVariablesSnapshot(snapshot); err != nil {
		return orbittemplate.InstallVariablesSnapshot{}, fmt.Errorf("validate variables: %w", err)
	}

	return snapshot, nil
}

func bundleRecordNode(record BundleRecord) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(record.SchemaVersion))
	contractutil.AppendMapping(root, "harness_id", contractutil.StringNode(record.HarnessID))

	templateNode := contractutil.MappingNode()
	contractutil.AppendMapping(templateNode, "source_kind", contractutil.StringNode(record.Template.SourceKind))
	contractutil.AppendMapping(templateNode, "source_repo", contractutil.StringNode(record.Template.SourceRepo))
	contractutil.AppendMapping(templateNode, "source_ref", contractutil.StringNode(record.Template.SourceRef))
	contractutil.AppendMapping(templateNode, "template_commit", contractutil.StringNode(record.Template.TemplateCommit))
	contractutil.AppendMapping(root, "template", templateNode)
	if strings.TrimSpace(record.RecommendedFramework) != "" {
		contractutil.AppendMapping(root, "recommended_framework", contractutil.StringNode(record.RecommendedFramework))
	}
	if record.AgentConfig != nil {
		agentConfigNode := contractutil.MappingNode()
		contractutil.AppendMapping(agentConfigNode, "schema_version", contractutil.IntNode(record.AgentConfig.SchemaVersion))
		contractutil.AppendMapping(root, "agent_config", agentConfigNode)
	}
	if len(record.AgentOverlays) > 0 {
		overlayIDs := make([]string, 0, len(record.AgentOverlays))
		for agentID := range record.AgentOverlays {
			overlayIDs = append(overlayIDs, agentID)
		}
		sort.Strings(overlayIDs)
		overlaysNode := contractutil.MappingNode()
		for _, agentID := range overlayIDs {
			contractutil.AppendMapping(overlaysNode, agentID, contractutil.StringNode(record.AgentOverlays[agentID]))
		}
		contractutil.AppendMapping(root, "agent_overlays", overlaysNode)
	}
	if record.AgentAddons != nil {
		contractutil.AppendMapping(root, "agent_addons", orbittemplate.AgentAddonsSnapshotNode(*record.AgentAddons))
	}

	memberIDsNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, memberID := range record.MemberIDs {
		memberIDsNode.Content = append(memberIDsNode.Content, contractutil.StringNode(memberID))
	}
	contractutil.AppendMapping(root, "member_ids", memberIDsNode)

	contractutil.AppendMapping(root, "applied_at", contractutil.TimestampNode(record.AppliedAt))
	contractutil.AppendMapping(root, "includes_root_agents", contractutil.BoolNode(record.IncludesRootAgents))
	if record.Variables != nil {
		contractutil.AppendMapping(root, "variables", orbittemplate.InstallVariablesSnapshotNode(*record.Variables))
	}

	ownedPathsNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, ownedPath := range record.OwnedPaths {
		ownedPathsNode.Content = append(ownedPathsNode.Content, contractutil.StringNode(ownedPath))
	}
	contractutil.AppendMapping(root, "owned_paths", ownedPathsNode)
	if len(record.OwnedPathDigests) > 0 {
		digestsNode := contractutil.MappingNode()
		paths := make([]string, 0, len(record.OwnedPathDigests))
		for path := range record.OwnedPathDigests {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			contractutil.AppendMapping(digestsNode, path, contractutil.StringNode(record.OwnedPathDigests[path]))
		}
		contractutil.AppendMapping(root, "owned_path_digests", digestsNode)
	}
	if strings.TrimSpace(record.RootAgentsDigest) != "" {
		contractutil.AppendMapping(root, "root_agents_digest", contractutil.StringNode(record.RootAgentsDigest))
	}

	return root
}

func stringsReplacePrefix(value string, prefix string, replacement string) string {
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return replacement + value[len(prefix):]
	}
	return value
}
