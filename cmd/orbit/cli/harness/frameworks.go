package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const frameworksSchemaVersion = 1

// FrameworksFile stores versioned recommended agent frameworks for one runtime.
type FrameworksFile struct {
	SchemaVersion        int    `yaml:"schema_version"`
	RecommendedFramework string `yaml:"recommended_framework,omitempty"`
}

type rawFrameworksFile struct {
	SchemaVersion        *int   `yaml:"schema_version"`
	RecommendedFramework string `yaml:"recommended_framework"`
}

// FrameworksRepoPath returns the repo-relative frameworks truth path.
func FrameworksRepoPath() string {
	return ".harness/agents/manifest.yaml"
}

// FrameworksPath returns the absolute frameworks truth path for one repo root.
func FrameworksPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".harness", "agents", "manifest.yaml")
}

func legacyFrameworksPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".harness", "frameworks.yaml")
}

// LoadFrameworksFile reads, decodes, and validates .harness/frameworks.yaml.
func LoadFrameworksFile(repoRoot string) (FrameworksFile, error) {
	filename := FrameworksPath(repoRoot)
	file, err := LoadFrameworksFileAtPath(filename)
	if err == nil {
		return file, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return FrameworksFile{}, err
	}

	return LoadFrameworksFileAtPath(legacyFrameworksPath(repoRoot))
}

// LoadOptionalFrameworksFile reads .harness/frameworks.yaml when present and otherwise returns an empty valid value.
func LoadOptionalFrameworksFile(repoRoot string) (FrameworksFile, error) {
	file, err := LoadFrameworksFile(repoRoot)
	if err == nil {
		return file, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return FrameworksFile{
			SchemaVersion:        frameworksSchemaVersion,
			RecommendedFramework: "",
		}, nil
	}

	return FrameworksFile{}, err
}

func loadOptionalFrameworksFileWithPresence(repoRoot string) (FrameworksFile, bool, error) {
	file, err := LoadFrameworksFile(repoRoot)
	if err == nil {
		return file, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return FrameworksFile{
			SchemaVersion:        frameworksSchemaVersion,
			RecommendedFramework: "",
		}, false, nil
	}

	return FrameworksFile{}, false, err
}

// LoadFrameworksFileAtPath reads, decodes, and validates one frameworks file.
func LoadFrameworksFileAtPath(filename string) (FrameworksFile, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // The path is repo-local and built from the fixed frameworks contract path.
	if err != nil {
		return FrameworksFile{}, fmt.Errorf("read %s: %w", filename, err)
	}

	file, err := ParseFrameworksFileData(data)
	if err != nil {
		return FrameworksFile{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return file, nil
}

// ParseFrameworksFileData decodes and validates frameworks.yaml bytes.
func ParseFrameworksFileData(data []byte) (FrameworksFile, error) {
	var raw rawFrameworksFile
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return FrameworksFile{}, fmt.Errorf("decode frameworks file: %w", err)
	}

	if raw.SchemaVersion == nil {
		return FrameworksFile{}, fmt.Errorf("schema_version must be present")
	}

	file := FrameworksFile{
		SchemaVersion:        *raw.SchemaVersion,
		RecommendedFramework: raw.RecommendedFramework,
	}
	if err := ValidateFrameworksFile(file); err != nil {
		return FrameworksFile{}, err
	}

	return file, nil
}

// ValidateFrameworksFile validates the frameworks truth contract.
func ValidateFrameworksFile(file FrameworksFile) error {
	if file.SchemaVersion != frameworksSchemaVersion {
		return fmt.Errorf("schema_version must be %d", frameworksSchemaVersion)
	}
	if file.RecommendedFramework != "" {
		if err := ids.ValidateOrbitID(file.RecommendedFramework); err != nil {
			return fmt.Errorf("recommended_framework: %w", err)
		}
	}

	return nil
}

// WriteFrameworksFile validates and writes .harness/frameworks.yaml.
func WriteFrameworksFile(repoRoot string, file FrameworksFile) (string, error) {
	filename := FrameworksPath(repoRoot)
	data, err := MarshalFrameworksFile(file)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}
	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}
	if err := os.Remove(legacyFrameworksPath(repoRoot)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove legacy frameworks file: %w", err)
	}

	return filename, nil
}

// MarshalFrameworksFile validates and encodes one frameworks file.
func MarshalFrameworksFile(file FrameworksFile) ([]byte, error) {
	if err := ValidateFrameworksFile(file); err != nil {
		return nil, fmt.Errorf("validate frameworks file: %w", err)
	}

	data, err := yaml.Marshal(file)
	if err != nil {
		return nil, fmt.Errorf("encode frameworks file: %w", err)
	}

	return data, nil
}
