package orbit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	orbitDirName    = ".orbit"
	orbitsDirName   = "orbits"
	orbitDirPerm    = 0o750
	orbitFilePerm   = 0o600
	tempFilePattern = ".orbit-*"
)

// InitResult describes the repo-local artifacts ensured by orbit init.
type InitResult struct {
	ConfigPath       string
	OrbitsDir        string
	ConfigCreated    bool
	OrbitsDirCreated bool
}

// EnsureInitialized creates the versioned Orbit configuration layout if needed.
func EnsureInitialized(repoRoot string) (InitResult, error) {
	result := InitResult{
		ConfigPath: ConfigPath(repoRoot),
		OrbitsDir:  OrbitsDir(repoRoot),
	}

	if result.ConfigPath == "" || result.OrbitsDir == "" {
		return InitResult{}, errors.New("repo root must not be empty")
	}

	rootDir := filepath.Join(repoRoot, orbitDirName)
	if err := os.MkdirAll(rootDir, orbitDirPerm); err != nil {
		return InitResult{}, fmt.Errorf("create %s: %w", rootDir, err)
	}

	if err := ensureDirectory(result.OrbitsDir, &result.OrbitsDirCreated); err != nil {
		return InitResult{}, err
	}

	if err := ensureConfigFile(result.ConfigPath, &result.ConfigCreated); err != nil {
		return InitResult{}, err
	}

	return result, nil
}

// DefinitionPath returns the absolute path to a versioned orbit definition file.
func DefinitionPath(repoRoot string, orbitID string) (string, error) {
	relativePath, err := DefinitionRelativePath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(relativePath)), nil
}

// HostedDefinitionPath returns the absolute path to a hosted orbit definition file.
func HostedDefinitionPath(repoRoot string, orbitID string) (string, error) {
	relativePath, err := HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(relativePath)), nil
}

// DefaultDefinition returns a minimal valid orbit skeleton for the requested id.
func DefaultDefinition(orbitID string) (Definition, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return Definition{}, fmt.Errorf("validate orbit id: %w", err)
	}

	return Definition{
		ID:          orbitID,
		Description: orbitID + " orbit",
		Include: []string{
			orbitID + "/**",
		},
		Exclude: []string{},
	}, nil
}

// DefaultMemberSchemaSpec returns a minimal valid member-schema orbit skeleton.
func DefaultMemberSchemaSpec(orbitID string) (OrbitSpec, error) {
	return defaultMemberSchemaSpec(orbitID, DefinitionRelativePath, ValidateOrbitSpec)
}

// DefaultHostedMemberSchemaSpec returns a minimal valid member-schema orbit skeleton for the hosted control plane.
func DefaultHostedMemberSchemaSpec(orbitID string) (OrbitSpec, error) {
	return defaultMemberSchemaSpec(orbitID, HostedDefinitionRelativePath, ValidateHostedOrbitSpec)
}

func defaultMemberSchemaSpec(
	orbitID string,
	pathBuilder func(string) (string, error),
	validator func(OrbitSpec) error,
) (OrbitSpec, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return OrbitSpec{}, fmt.Errorf("validate orbit id: %w", err)
	}

	metaFile, err := pathBuilder(orbitID)
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("build meta file path: %w", err)
	}

	spec := OrbitSpec{
		Package:     &ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: orbitID},
		ID:          orbitID,
		Description: orbitID + " orbit",
		Meta: &OrbitMeta{
			File:                              metaFile,
			IncludeInProjection:               true,
			IncludeInWrite:                    true,
			IncludeInExport:                   true,
			IncludeDescriptionInOrchestration: true,
		},
		Behavior: &OrbitBehavior{
			Scope: OrbitBehaviorScope{
				ProjectionRoles:    []OrbitMemberRole{OrbitMemberMeta, OrbitMemberSubject, OrbitMemberRule, OrbitMemberProcess},
				WriteRoles:         []OrbitMemberRole{OrbitMemberMeta, OrbitMemberRule},
				ExportRoles:        []OrbitMemberRole{OrbitMemberMeta, OrbitMemberRule},
				OrchestrationRoles: []OrbitMemberRole{OrbitMemberMeta, OrbitMemberRule, OrbitMemberProcess},
			},
			Orchestration: OrbitBehaviorOrchestration{
				IncludeOrbitDescription:   true,
				MaterializeAgentsFromMeta: true,
			},
		},
	}

	if err := validator(spec); err != nil {
		return OrbitSpec{}, fmt.Errorf("validate orbit spec: %w", err)
	}

	return spec, nil
}

// WriteDefinition writes an orbit definition file atomically.
func WriteDefinition(repoRoot string, definition Definition) (string, error) {
	return writeDefinitionAtPath(repoRoot, definition, DefinitionPath)
}

// WriteHostedDefinition writes a hosted orbit definition file atomically.
func WriteHostedDefinition(repoRoot string, definition Definition) (string, error) {
	return writeDefinitionAtPath(repoRoot, definition, HostedDefinitionPath)
}

func writeDefinitionAtPath(repoRoot string, definition Definition, pathBuilder func(string, string) (string, error)) (string, error) {
	if err := ValidateDefinition(definition); err != nil {
		return "", fmt.Errorf("validate orbit definition: %w", err)
	}

	filename, err := pathBuilder(repoRoot, definition.ID)
	if err != nil {
		return "", fmt.Errorf("build definition path: %w", err)
	}

	data, err := yaml.Marshal(definition)
	if err != nil {
		return "", fmt.Errorf("marshal orbit definition: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write orbit definition: %w", err)
	}

	return filename, nil
}

// WriteOrbitSpec writes a member-schema orbit spec file atomically.
func WriteOrbitSpec(repoRoot string, spec OrbitSpec) (string, error) {
	packageNormalized, err := normalizeOrbitSpecPackageIdentity(spec)
	if err != nil {
		return "", fmt.Errorf("normalize orbit package identity: %w", err)
	}
	normalized, err := normalizeOrbitSpecMemberIdentities(packageNormalized)
	if err != nil {
		return "", fmt.Errorf("normalize orbit spec: %w", err)
	}
	if err := ValidateOrbitSpec(normalized); err != nil {
		return "", fmt.Errorf("validate orbit spec: %w", err)
	}

	return writeOrbitSpecAtPath(repoRoot, normalized, DefinitionPath)
}

// WriteHostedOrbitSpec writes a hosted member-schema orbit spec file atomically.
func WriteHostedOrbitSpec(repoRoot string, spec OrbitSpec) (string, error) {
	packageNormalized, err := normalizeOrbitSpecPackageIdentity(spec)
	if err != nil {
		return "", fmt.Errorf("normalize orbit package identity: %w", err)
	}
	normalized, err := normalizeOrbitSpecMemberIdentities(packageNormalized)
	if err != nil {
		return "", fmt.Errorf("normalize orbit spec: %w", err)
	}
	if err := ValidateHostedOrbitSpec(normalized); err != nil {
		return "", fmt.Errorf("validate orbit spec: %w", err)
	}

	return writeOrbitSpecAtPath(repoRoot, normalized, HostedDefinitionPath)
}

func writeOrbitSpecAtPath(repoRoot string, spec OrbitSpec, pathBuilder func(string, string) (string, error)) (string, error) {
	spec, err := normalizeOrbitSpecBehaviorAlias(spec)
	if err != nil {
		return "", fmt.Errorf("normalize orbit behavior: %w", err)
	}
	filename, err := pathBuilder(repoRoot, spec.ID)
	if err != nil {
		return "", fmt.Errorf("build orbit spec path: %w", err)
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal orbit spec: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write orbit spec: %w", err)
	}

	return filename, nil
}

func ensureDirectory(directory string, created *bool) error {
	if info, err := os.Stat(directory); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists but is not a directory", directory)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", directory, err)
	}

	if err := os.MkdirAll(directory, orbitDirPerm); err != nil {
		return fmt.Errorf("create %s: %w", directory, err)
	}
	*created = true

	return nil
}

func ensureConfigFile(filename string, created *bool) error {
	if _, err := os.Stat(filename); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", filename, err)
	}

	data, err := yaml.Marshal(DefaultGlobalConfig())
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(filename, data); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}

	*created = true

	return nil
}

func atomicWriteFile(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), orbitDirPerm); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", filename, err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(filename), tempFilePattern)
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", filename, err)
	}

	tempName := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempName)
		}
	}()

	if err := tempFile.Chmod(orbitFilePerm); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("set temp file permissions for %s: %w", filename, err)
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp file for %s: %w", filename, err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temp file for %s: %w", filename, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", filename, err)
	}
	//nolint:gosec,nolintlint // The destination path is repo-local and derived from validated Orbit config paths; newer gosec releases no longer flag it.
	if err := os.Rename(tempName, filename); err != nil {
		return fmt.Errorf("rename temp file for %s: %w", filename, err)
	}

	cleanupTemp = false

	return nil
}
