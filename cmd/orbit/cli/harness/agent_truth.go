package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	agentConfigSchemaVersion  = 1
	agentOverlaySchemaVersion = 1

	// AgentOverlayModeRawPassthrough preserves raw agent-specific input as versioned truth.
	AgentOverlayModeRawPassthrough = "raw_passthrough"
)

// AgentConfigFile stores the canonical shared agent truth for one runtime or harness template.
type AgentConfigFile struct {
	SchemaVersion int `yaml:"schema_version"`
}

type rawAgentConfigFile struct {
	SchemaVersion *int `yaml:"schema_version"`
}

// AgentOverlayFile stores one validated per-agent overlay file plus its exact payload.
type AgentOverlayFile struct {
	SchemaVersion int    `yaml:"schema_version"`
	Mode          string `yaml:"mode"`
	Content       []byte `yaml:"-"`
}

// AgentConfigRepoPath returns the repo-relative canonical agent config path.
func AgentConfigRepoPath() string {
	return ".harness/agents/agent.yaml"
}

// AgentConfigPath returns the absolute canonical agent config path for one repo root.
func AgentConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".harness", "agents", "agent.yaml")
}

// AgentOverlaysDirRepoPath returns the repo-relative overlay directory host.
func AgentOverlaysDirRepoPath() string {
	return ".harness/agents/overlays"
}

// AgentOverlaysDirPath returns the absolute overlay directory host for one repo root.
func AgentOverlaysDirPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".harness", "agents", "overlays")
}

// AgentOverlayRepoPath returns the repo-relative overlay host for one supported agent id.
func AgentOverlayRepoPath(agentID string) string {
	return filepath.ToSlash(filepath.Join(".harness", "agents", "overlays", agentID+".yaml"))
}

// AgentOverlayPath returns the absolute overlay host for one supported agent id.
func AgentOverlayPath(repoRoot string, agentID string) string {
	return filepath.Join(repoRoot, ".harness", "agents", "overlays", agentID+".yaml")
}

// LoadAgentConfigFile reads and validates .harness/agents/agent.yaml.
func LoadAgentConfigFile(repoRoot string) (AgentConfigFile, error) {
	return LoadAgentConfigFileAtPath(AgentConfigPath(repoRoot))
}

// LoadOptionalAgentConfigFile reads the canonical agent config when present.
func LoadOptionalAgentConfigFile(repoRoot string) (AgentConfigFile, bool, error) {
	file, err := LoadAgentConfigFile(repoRoot)
	if err == nil {
		return file, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return AgentConfigFile{}, false, nil
	}

	return AgentConfigFile{}, false, err
}

// LoadAgentConfigFileAtPath reads and validates one canonical agent config file.
func LoadAgentConfigFileAtPath(filename string) (AgentConfigFile, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // Path is repo-local and built from the fixed agent config contract path.
	if err != nil {
		return AgentConfigFile{}, fmt.Errorf("read %s: %w", filename, err)
	}

	file, err := ParseAgentConfigFileData(data)
	if err != nil {
		return AgentConfigFile{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return file, nil
}

// ParseAgentConfigFileData decodes and validates agent.yaml bytes.
func ParseAgentConfigFileData(data []byte) (AgentConfigFile, error) {
	var raw rawAgentConfigFile
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return AgentConfigFile{}, fmt.Errorf("decode agent config file: %w", err)
	}
	if raw.SchemaVersion == nil {
		return AgentConfigFile{}, fmt.Errorf("schema_version must be present")
	}

	file := AgentConfigFile{SchemaVersion: *raw.SchemaVersion}
	if err := ValidateAgentConfigFile(file); err != nil {
		return AgentConfigFile{}, err
	}

	return file, nil
}

// ValidateAgentConfigFile validates the current canonical agent config contract.
func ValidateAgentConfigFile(file AgentConfigFile) error {
	if file.SchemaVersion != agentConfigSchemaVersion {
		return fmt.Errorf("schema_version must be %d", agentConfigSchemaVersion)
	}

	return nil
}

// WriteAgentConfigFile validates and writes .harness/agents/agent.yaml.
func WriteAgentConfigFile(repoRoot string, file AgentConfigFile) (string, error) {
	filename := AgentConfigPath(repoRoot)
	data, err := MarshalAgentConfigFile(file)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}
	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

// MarshalAgentConfigFile validates and encodes one canonical agent config file.
func MarshalAgentConfigFile(file AgentConfigFile) ([]byte, error) {
	if err := ValidateAgentConfigFile(file); err != nil {
		return nil, fmt.Errorf("validate agent config file: %w", err)
	}

	data, err := yaml.Marshal(file)
	if err != nil {
		return nil, fmt.Errorf("encode agent config file: %w", err)
	}

	return data, nil
}

// LoadAgentOverlayFile reads and validates one per-agent overlay file.
func LoadAgentOverlayFile(repoRoot string, agentID string) (AgentOverlayFile, error) {
	if err := validateAgentOverlayID(agentID); err != nil {
		return AgentOverlayFile{}, err
	}

	return LoadAgentOverlayFileAtPath(AgentOverlayPath(repoRoot, agentID))
}

// LoadAgentOverlayFileAtPath reads and validates one per-agent overlay file from an absolute path.
func LoadAgentOverlayFileAtPath(filename string) (AgentOverlayFile, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // Path is repo-local and built from the fixed agent overlay contract path.
	if err != nil {
		return AgentOverlayFile{}, fmt.Errorf("read %s: %w", filename, err)
	}

	file, err := ParseAgentOverlayFileData(data)
	if err != nil {
		return AgentOverlayFile{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return file, nil
}

// ParseAgentOverlayFileData validates one overlay payload while preserving exact bytes.
func ParseAgentOverlayFileData(data []byte) (AgentOverlayFile, error) {
	schemaVersion, mode, err := decodeAgentOverlayMetadata(data)
	if err != nil {
		return AgentOverlayFile{}, err
	}

	file := AgentOverlayFile{
		SchemaVersion: schemaVersion,
		Mode:          mode,
		Content:       append([]byte(nil), data...),
	}
	if err := ValidateAgentOverlayFile(file); err != nil {
		return AgentOverlayFile{}, err
	}

	return file, nil
}

// ValidateAgentOverlayFile validates one per-agent overlay contract.
func ValidateAgentOverlayFile(file AgentOverlayFile) error {
	if len(file.Content) == 0 {
		return fmt.Errorf("content must not be empty")
	}

	schemaVersion, mode, err := decodeAgentOverlayMetadata(file.Content)
	if err != nil {
		return err
	}
	if file.SchemaVersion != 0 && file.SchemaVersion != schemaVersion {
		return fmt.Errorf("schema_version must match encoded content")
	}
	if strings.TrimSpace(file.Mode) != "" && strings.TrimSpace(file.Mode) != strings.TrimSpace(mode) {
		return fmt.Errorf("mode must match encoded content")
	}

	return nil
}

// WriteAgentOverlayFile validates and writes one per-agent overlay file.
func WriteAgentOverlayFile(repoRoot string, agentID string, file AgentOverlayFile) (string, error) {
	if err := validateAgentOverlayID(agentID); err != nil {
		return "", err
	}
	if err := ValidateAgentOverlayFile(file); err != nil {
		return "", fmt.Errorf("validate agent overlay file: %w", err)
	}

	filename := AgentOverlayPath(repoRoot, agentID)
	if err := contractutil.AtomicWriteFile(filename, file.Content); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

// ListAgentOverlayIDs returns valid overlay ids present under .harness/agents/overlays.
func ListAgentOverlayIDs(repoRoot string) ([]string, error) {
	entries, err := os.ReadDir(AgentOverlaysDirPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read agent overlays directory: %w", err)
	}

	idsList := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		agentID := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if err := validateAgentOverlayID(agentID); err != nil {
			return nil, err
		}
		idsList = append(idsList, agentID)
	}
	sort.Strings(idsList)

	return idsList, nil
}

func validateAgentOverlayID(agentID string) error {
	if err := ids.ValidateOrbitID(agentID); err != nil {
		return fmt.Errorf("agent id: %w", err)
	}
	if _, ok := LookupFrameworkAdapter(agentID); !ok {
		return fmt.Errorf("agent id %q is not supported by this build", agentID)
	}

	return nil
}

func decodeAgentOverlayMetadata(data []byte) (int, string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return 0, "", fmt.Errorf("decode agent overlay file: %w", err)
	}
	if len(document.Content) != 1 {
		return 0, "", fmt.Errorf("agent overlay document must contain exactly one YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return 0, "", fmt.Errorf("agent overlay file must be a YAML mapping")
	}

	var (
		schemaVersionNode *yaml.Node
		modeNode          *yaml.Node
		rawNode           *yaml.Node
	)
	for index := 0; index < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		valueNode := root.Content[index+1]
		switch keyNode.Value {
		case "schema_version":
			schemaVersionNode = valueNode
		case "mode":
			modeNode = valueNode
		case "raw":
			rawNode = valueNode
		default:
			return 0, "", fmt.Errorf("unknown top-level field %q", keyNode.Value)
		}
	}
	if schemaVersionNode == nil {
		return 0, "", fmt.Errorf("schema_version must be present")
	}
	schemaVersion, err := strconv.Atoi(strings.TrimSpace(schemaVersionNode.Value))
	if err != nil {
		return 0, "", fmt.Errorf("schema_version must be an integer")
	}
	if schemaVersion != agentOverlaySchemaVersion {
		return 0, "", fmt.Errorf("schema_version must be %d", agentOverlaySchemaVersion)
	}
	if modeNode == nil {
		return 0, "", fmt.Errorf("mode must be present")
	}
	mode := strings.TrimSpace(modeNode.Value)
	if mode != AgentOverlayModeRawPassthrough {
		return 0, "", fmt.Errorf("mode must be %q", AgentOverlayModeRawPassthrough)
	}
	if rawNode == nil {
		return 0, "", fmt.Errorf("raw must be present for mode %q", AgentOverlayModeRawPassthrough)
	}

	return schemaVersion, mode, nil
}
