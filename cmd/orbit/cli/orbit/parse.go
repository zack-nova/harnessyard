package orbit

import (
	"fmt"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

type globalConfigFile struct {
	Version           *int            `yaml:"version"`
	SharedScope       *[]string       `yaml:"shared_scope"`
	ProjectionVisible *[]string       `yaml:"projection_visible"`
	Behavior          *BehaviorConfig `yaml:"behavior"`
}

// ParseGlobalConfigData decodes and validates .orbit/config.yaml bytes.
func ParseGlobalConfigData(data []byte) (GlobalConfig, error) {
	var raw globalConfigFile
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return GlobalConfig{}, fmt.Errorf("decode global config: %w", err)
	}

	config := DefaultGlobalConfig()
	if raw.Version != nil {
		config.Version = *raw.Version
	}
	if raw.Behavior != nil {
		config.Behavior = *raw.Behavior
	}
	if raw.SharedScope != nil {
		config.SharedScope = append([]string(nil), (*raw.SharedScope)...)
	}
	if raw.ProjectionVisible != nil {
		config.ProjectionVisible = append([]string(nil), (*raw.ProjectionVisible)...)
	}

	if err := ValidateGlobalConfig(config); err != nil {
		return GlobalConfig{}, fmt.Errorf("validate global config: %w", err)
	}

	return config, nil
}

// ParseDefinitionData decodes and validates one orbit definition file.
func ParseDefinitionData(data []byte, sourcePath string) (Definition, error) {
	spec, err := ParseOrbitSpecData(data, sourcePath)
	if err != nil {
		return Definition{}, err
	}

	definition, err := compatibilityDefinitionFromOrbitSpec(spec)
	if err != nil {
		return Definition{}, fmt.Errorf("project compatibility orbit definition: %w", err)
	}

	return definition, nil
}
