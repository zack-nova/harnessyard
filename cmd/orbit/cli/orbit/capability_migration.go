package orbit

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

type capabilityMigrationSpecFile struct {
	Package      *ids.PackageIdentity          `yaml:"package,omitempty"`
	ID           string                        `yaml:"id"`
	Name         string                        `yaml:"name,omitempty"`
	Description  string                        `yaml:"description,omitempty"`
	Include      []string                      `yaml:"include,omitempty"`
	Exclude      []string                      `yaml:"exclude,omitempty"`
	Meta         *OrbitMeta                    `yaml:"meta,omitempty"`
	Capabilities *capabilityMigrationContainer `yaml:"capabilities,omitempty"`
	Members      []OrbitMember                 `yaml:"members,omitempty"`
	Content      []OrbitMember                 `yaml:"content,omitempty"`
	Behavior     *OrbitBehavior                `yaml:"behavior,omitempty"`
	Rules        *OrbitBehavior                `yaml:"rules,omitempty"`
}

type capabilityMigrationContainer struct {
	Commands yaml.Node `yaml:"commands,omitempty"`
	Skills   yaml.Node `yaml:"skills,omitempty"`
}

type legacyCommandCapabilityEntry struct {
	ID          string `yaml:"id"`
	Path        string `yaml:"path"`
	Description string `yaml:"description,omitempty"`
}

type legacySkillCapabilityEntry struct {
	ID          string `yaml:"id"`
	Path        string `yaml:"path,omitempty"`
	URI         string `yaml:"uri,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type CapabilityMigrationV066Result struct {
	Spec     OrbitSpec
	Migrated bool
	File     string
}

func MigrateHostedCapabilitySpecV066(repoRoot string, orbitID string) (CapabilityMigrationV066Result, error) {
	data, filename, err := readHostedCapabilityMigrationInput(repoRoot, orbitID)
	if err != nil {
		return CapabilityMigrationV066Result{}, err
	}

	spec, migrated, err := ParseHostedCapabilityMigrationV066Data(data, filename)
	if err != nil {
		return CapabilityMigrationV066Result{}, err
	}
	if migrated {
		if _, err := WriteHostedOrbitSpec(repoRoot, spec); err != nil {
			return CapabilityMigrationV066Result{}, fmt.Errorf("write migrated hosted orbit spec: %w", err)
		}
	}

	return CapabilityMigrationV066Result{
		Spec:     spec,
		Migrated: migrated,
		File:     filename,
	}, nil
}

func ParseHostedCapabilityMigrationV066Data(data []byte, sourcePath string) (OrbitSpec, bool, error) {
	var raw capabilityMigrationSpecFile
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return OrbitSpec{}, false, fmt.Errorf("decode hosted orbit spec: %w", err)
	}

	members := append([]OrbitMember(nil), raw.Members...)
	if len(members) == 0 && len(raw.Content) > 0 {
		members = append([]OrbitMember(nil), raw.Content...)
	}
	orbitID := raw.ID
	if raw.Package != nil {
		orbitID = raw.Package.Name
	}

	spec := OrbitSpec{
		Package:     raw.Package,
		ID:          orbitID,
		Name:        raw.Name,
		Description: raw.Description,
		Include:     append([]string(nil), raw.Include...),
		Exclude:     append([]string(nil), raw.Exclude...),
		Meta:        raw.Meta,
		Members:     members,
		Behavior:    raw.Behavior,
		Rules:       raw.Rules,
		SourcePath:  sourcePath,
	}

	capabilities, migrated, err := migrateLegacyCapabilityContainer(orbitID, raw.Capabilities)
	if err != nil {
		return OrbitSpec{}, false, err
	}
	spec.Capabilities = capabilities

	if err := ValidateHostedOrbitSpec(spec); err != nil {
		return OrbitSpec{}, false, fmt.Errorf("validate migrated hosted orbit spec: %w", err)
	}

	return spec, migrated, nil
}

func migrateLegacyCapabilityContainer(orbitID string, raw *capabilityMigrationContainer) (*OrbitCapabilities, bool, error) {
	if raw == nil {
		return nil, false, nil
	}

	capabilities := OrbitCapabilities{}
	migrated := false

	if nodePresent(raw.Commands) {
		if raw.Commands.Kind == yaml.SequenceNode {
			paths, err := migrateLegacyCommandEntries(orbitID, raw.Commands)
			if err != nil {
				return nil, false, err
			}
			capabilities.Commands = paths
			migrated = true
		} else {
			var commands OrbitCommandCapabilityPaths
			if err := raw.Commands.Decode(&commands); err != nil {
				return nil, false, fmt.Errorf("decode canonical command capabilities: %w", err)
			}
			capabilities.Commands = &commands
		}
	}

	if nodePresent(raw.Skills) {
		if raw.Skills.Kind == yaml.SequenceNode {
			skills, err := migrateLegacySkillEntries(orbitID, raw.Skills)
			if err != nil {
				return nil, false, err
			}
			capabilities.Skills = skills
			migrated = true
		} else {
			var skills OrbitSkillCapabilities
			if err := raw.Skills.Decode(&skills); err != nil {
				return nil, false, fmt.Errorf("decode canonical skill capabilities: %w", err)
			}
			remoteMigrated, err := migrateRemoteSkillURIsToDependencies(&skills)
			if err != nil {
				return nil, false, err
			}
			if remoteMigrated {
				migrated = true
			}
			capabilities.Skills = &skills
		}
	}

	return normalizeCapabilities(capabilities), migrated, nil
}

func migrateLegacyCommandEntries(orbitID string, node yaml.Node) (*OrbitCommandCapabilityPaths, error) {
	var entries []legacyCommandCapabilityEntry
	if err := node.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode legacy command capabilities: %w", err)
	}
	if len(entries) == 0 {
		return &OrbitCommandCapabilityPaths{}, nil
	}

	paths := make([]string, 0, len(entries))
	seenNames := make(map[string]string, len(entries))
	for index, entry := range entries {
		normalizedPath, name, err := normalizeCommandCapabilityInput(entry.ID, entry.Path)
		if err != nil {
			return nil, fmt.Errorf("legacy capabilities.commands[%d]: %w", index, err)
		}
		if existingPath, ok := seenNames[name]; ok {
			return nil, fmt.Errorf("legacy capabilities.commands[%d]: command basename %q collides between %q and %q", index, name, existingPath, normalizedPath)
		}
		seenNames[name] = normalizedPath
		paths = append(paths, normalizedPath)
	}

	return &OrbitCommandCapabilityPaths{
		Paths: collapseMigratedCommandPaths(paths, orbitID),
	}, nil
}

func migrateLegacySkillEntries(orbitID string, node yaml.Node) (*OrbitSkillCapabilities, error) {
	var entries []legacySkillCapabilityEntry
	if err := node.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode legacy skill capabilities: %w", err)
	}
	if len(entries) == 0 {
		return &OrbitSkillCapabilities{}, nil
	}

	localRoots := make([]string, 0, len(entries))
	remoteURIs := make([]string, 0, len(entries))
	seenLocalNames := make(map[string]string, len(entries))
	seenRemote := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		switch {
		case strings.TrimSpace(entry.Path) != "" && strings.TrimSpace(entry.URI) != "":
			return nil, fmt.Errorf("legacy capabilities.skills[%d]: path and uri cannot both be set", index)
		case strings.TrimSpace(entry.URI) != "":
			normalizedURI, err := normalizeRemoteSkillURI(entry.URI)
			if err != nil {
				return nil, fmt.Errorf("legacy capabilities.skills[%d]: %w", index, err)
			}
			if _, ok := seenRemote[normalizedURI]; !ok {
				seenRemote[normalizedURI] = struct{}{}
				remoteURIs = append(remoteURIs, normalizedURI)
			}
		default:
			normalizedRoot, name, err := normalizeLocalSkillCapabilityInput(entry.ID, entry.Path)
			if err != nil {
				return nil, fmt.Errorf("legacy capabilities.skills[%d]: %w", index, err)
			}
			if existingRoot, ok := seenLocalNames[name]; ok {
				return nil, fmt.Errorf("legacy capabilities.skills[%d]: skill basename %q collides between %q and %q", index, name, existingRoot, normalizedRoot)
			}
			seenLocalNames[name] = normalizedRoot
			localRoots = append(localRoots, normalizedRoot)
		}
	}

	skills := OrbitSkillCapabilities{}
	if len(localRoots) > 0 {
		skills.Local = &OrbitLocalSkillCapabilityPaths{
			Paths: collapseMigratedLocalSkillPaths(localRoots, orbitID),
		}
	}
	if len(remoteURIs) > 0 {
		sort.Strings(remoteURIs)
		skills.Remote = &OrbitRemoteSkillCapabilities{Dependencies: remoteSkillDependenciesFromURIs(remoteURIs)}
	}
	if skills.Local == nil && skills.Remote == nil {
		return &OrbitSkillCapabilities{}, nil
	}

	return &skills, nil
}

func migrateRemoteSkillURIsToDependencies(skills *OrbitSkillCapabilities) (bool, error) {
	if skills == nil || skills.Remote == nil || len(skills.Remote.URIs) == 0 {
		return false, nil
	}
	if len(skills.Remote.Dependencies) > 0 {
		return false, fmt.Errorf("capabilities.skills.remote must not define both uris and dependencies")
	}

	remoteURIs := make([]string, 0, len(skills.Remote.URIs))
	for index, rawURI := range skills.Remote.URIs {
		normalizedURI, err := normalizeRemoteSkillURI(rawURI)
		if err != nil {
			return false, fmt.Errorf("capabilities.skills.remote.uris[%d]: %w", index, err)
		}
		remoteURIs = append(remoteURIs, normalizedURI)
	}
	skills.Remote.Dependencies = remoteSkillDependenciesFromURIs(remoteURIs)
	skills.Remote.URIs = nil

	return true, nil
}

func remoteSkillDependenciesFromURIs(uris []string) []OrbitRemoteSkillDependency {
	dependencies := make([]OrbitRemoteSkillDependency, 0, len(uris))
	for _, uri := range uris {
		dependencies = append(dependencies, OrbitRemoteSkillDependency{URI: uri})
	}

	return dependencies
}

func collapseMigratedCommandPaths(paths []string, orbitID string) OrbitMemberPaths {
	normalized := sortedUniqueStrings(paths)
	if len(normalized) > 0 && allMigratedCommandsUnderDefaultRoot(normalized, orbitID) {
		return OrbitMemberPaths{
			Include: []string{path.Join("commands", orbitID, "**", "*.md")},
		}
	}

	return OrbitMemberPaths{Include: normalized}
}

func collapseMigratedLocalSkillPaths(roots []string, orbitID string) OrbitMemberPaths {
	normalized := sortedUniqueStrings(roots)
	if len(normalized) > 0 && allMigratedLocalSkillsUnderDefaultRoot(normalized, orbitID) {
		return OrbitMemberPaths{
			Include: []string{path.Join("skills", orbitID, "*")},
		}
	}

	return OrbitMemberPaths{Include: normalized}
}

func allMigratedCommandsUnderDefaultRoot(paths []string, orbitID string) bool {
	prefix := path.Join("commands", orbitID) + "/"
	for _, candidate := range paths {
		if !strings.HasPrefix(candidate, prefix) || !strings.HasSuffix(candidate, ".md") {
			return false
		}
	}

	return true
}

func allMigratedLocalSkillsUnderDefaultRoot(roots []string, orbitID string) bool {
	prefix := path.Join("skills", orbitID) + "/"
	for _, root := range roots {
		if !strings.HasPrefix(root, prefix) {
			return false
		}
		relative := strings.TrimPrefix(root, prefix)
		if relative == "" || strings.Contains(relative, "/") {
			return false
		}
	}

	return true
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)

	return unique
}

func nodePresent(node yaml.Node) bool {
	return node.Kind != 0 && (node.Kind != yaml.ScalarNode || node.Tag != "!!null")
}

func readHostedCapabilityMigrationInput(repoRoot string, orbitID string) ([]byte, string, error) {
	filename, err := HostedDefinitionPath(repoRoot, orbitID)
	if err != nil {
		return nil, "", fmt.Errorf("build hosted orbit definition path: %w", err)
	}

	relativePath, err := HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return nil, "", fmt.Errorf("build hosted orbit definition relative path: %w", err)
	}

	data, err := gitpkg.ReadFileWorktreeOrHEAD(context.Background(), repoRoot, relativePath)
	if err != nil {
		return nil, "", fmt.Errorf("read hosted orbit definition: %w", err)
	}

	return data, filename, nil
}
