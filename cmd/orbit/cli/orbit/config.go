package orbit

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	configRelativePath      = ".orbit/config.yaml"
	orbitsRelativeDir       = ".orbit/orbits"
	hostedOrbitsRelativeDir = ".harness/orbits"
)

const (
	OutsideChangesModeWarn   = "warn"
	SparseCheckoutModeNoCone = "no-cone"
)

// GlobalConfig contains repository-wide Orbit configuration.
type GlobalConfig struct {
	Version           int            `yaml:"version"`
	SharedScope       []string       `yaml:"shared_scope"`
	ProjectionVisible []string       `yaml:"projection_visible"`
	Behavior          BehaviorConfig `yaml:"behavior"`
}

// BehaviorConfig captures stable Orbit MVP behavior settings.
type BehaviorConfig struct {
	OutsideChangesMode       string `yaml:"outside_changes_mode"`
	BlockSwitchIfHiddenDirty bool   `yaml:"block_switch_if_hidden_dirty"`
	CommitAppendTrailer      bool   `yaml:"commit_append_trailer"`
	SparseCheckoutMode       string `yaml:"sparse_checkout_mode"`
}

// DefaultGlobalConfig returns the documented MVP defaults.
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Version:           1,
		SharedScope:       []string{},
		ProjectionVisible: []string{},
		Behavior: BehaviorConfig{
			OutsideChangesMode:       OutsideChangesModeWarn,
			BlockSwitchIfHiddenDirty: true,
			CommitAppendTrailer:      true,
			SparseCheckoutMode:       SparseCheckoutModeNoCone,
		},
	}
}

// ConfigPath returns the absolute path to .orbit/config.yaml.
func ConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, configRelativePath)
}

// OrbitsDir returns the absolute path to .orbit/orbits.
func OrbitsDir(repoRoot string) string {
	return filepath.Join(repoRoot, orbitsRelativeDir)
}

// HostedOrbitsDir returns the absolute path to .harness/orbits.
func HostedOrbitsDir(repoRoot string) string {
	return filepath.Join(repoRoot, hostedOrbitsRelativeDir)
}

// DefinitionRelativePath returns the repo-relative path to an orbit definition.
func DefinitionRelativePath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return path.Join(orbitsRelativeDir, orbitID+".yaml"), nil
}

// HostedDefinitionRelativePath returns the repo-relative path to one hosted orbit definition.
func HostedDefinitionRelativePath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return path.Join(hostedOrbitsRelativeDir, orbitID+".yaml"), nil
}
