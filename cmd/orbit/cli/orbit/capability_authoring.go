package orbit

import (
	"fmt"
	"path"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// CapabilityKind identifies one authored capability category.
type CapabilityKind string

const (
	CapabilityKindCommand CapabilityKind = "command"
	CapabilityKindSkill   CapabilityKind = "skill"
)

// ParseCapabilityKind parses one supported authored capability category.
func ParseCapabilityKind(raw string) (CapabilityKind, error) {
	switch CapabilityKind(raw) {
	case CapabilityKindCommand, CapabilityKindSkill:
		return CapabilityKind(raw), nil
	default:
		return "", fmt.Errorf("invalid capability kind %q", raw)
	}
}

// AddCapability appends one authored capability through the temporary compatibility command surface.
func AddCapability(spec OrbitSpec, kind CapabilityKind, id string, capabilityPath string, _ string) (OrbitSpec, error) {
	capabilities := cloneCapabilities(spec.Capabilities)

	switch kind {
	case CapabilityKindCommand:
		normalizedPath, name, err := normalizeCommandCapabilityInput(id, capabilityPath)
		if err != nil {
			return OrbitSpec{}, err
		}
		if hasCommandCapability(capabilities, name) {
			return OrbitSpec{}, fmt.Errorf("command capability %q already exists", id)
		}
		capabilities.Commands = ensureCommandCapabilityPaths(capabilities.Commands)
		capabilities.Commands.Paths.Include = append(capabilities.Commands.Paths.Include, normalizedPath)
	case CapabilityKindSkill:
		normalizedRootPath, name, err := normalizeLocalSkillCapabilityInput(id, capabilityPath)
		if err != nil {
			return OrbitSpec{}, err
		}
		if hasLocalSkillCapability(capabilities, name) {
			return OrbitSpec{}, fmt.Errorf("skill capability %q already exists", id)
		}
		capabilities.Skills = ensureSkillCapabilities(capabilities.Skills)
		capabilities.Skills.Local = ensureLocalSkillCapabilityPaths(capabilities.Skills.Local)
		capabilities.Skills.Local.Paths.Include = append(capabilities.Skills.Local.Paths.Include, normalizedRootPath)
	default:
		return OrbitSpec{}, fmt.Errorf("unsupported capability kind %q", kind)
	}

	spec.Capabilities = normalizeCapabilities(capabilities)

	return spec, nil
}

// SetCapability overwrites one authored capability through the temporary compatibility command surface.
func SetCapability(spec OrbitSpec, kind CapabilityKind, id string, capabilityPath string, _ string) (OrbitSpec, error) {
	capabilities := cloneCapabilities(spec.Capabilities)

	switch kind {
	case CapabilityKindCommand:
		normalizedPath, name, err := normalizeCommandCapabilityInput(id, capabilityPath)
		if err != nil {
			return OrbitSpec{}, err
		}
		capabilities.Commands = ensureCommandCapabilityPaths(capabilities.Commands)
		index := indexOfCommandCapability(capabilities, name)
		if index < 0 {
			return OrbitSpec{}, fmt.Errorf("command capability %q not found", id)
		}
		capabilities.Commands.Paths.Include[index] = normalizedPath
	case CapabilityKindSkill:
		normalizedRootPath, name, err := normalizeLocalSkillCapabilityInput(id, capabilityPath)
		if err != nil {
			return OrbitSpec{}, err
		}
		capabilities.Skills = ensureSkillCapabilities(capabilities.Skills)
		capabilities.Skills.Local = ensureLocalSkillCapabilityPaths(capabilities.Skills.Local)
		index := indexOfLocalSkillCapability(capabilities, name)
		if index < 0 {
			return OrbitSpec{}, fmt.Errorf("skill capability %q not found", id)
		}
		capabilities.Skills.Local.Paths.Include[index] = normalizedRootPath
	default:
		return OrbitSpec{}, fmt.Errorf("unsupported capability kind %q", kind)
	}

	spec.Capabilities = normalizeCapabilities(capabilities)

	return spec, nil
}

// RemoveCapability removes one authored capability through the temporary compatibility command surface.
func RemoveCapability(spec OrbitSpec, kind CapabilityKind, id string) (OrbitSpec, error) {
	capabilities := cloneCapabilities(spec.Capabilities)

	switch kind {
	case CapabilityKindCommand:
		index := indexOfCommandCapability(capabilities, id)
		if index < 0 {
			return OrbitSpec{}, fmt.Errorf("command capability %q not found", id)
		}
		capabilities.Commands.Paths.Include = append(capabilities.Commands.Paths.Include[:index], capabilities.Commands.Paths.Include[index+1:]...)
	case CapabilityKindSkill:
		index := indexOfLocalSkillCapability(capabilities, id)
		if index < 0 {
			return OrbitSpec{}, fmt.Errorf("skill capability %q not found", id)
		}
		capabilities.Skills.Local.Paths.Include = append(capabilities.Skills.Local.Paths.Include[:index], capabilities.Skills.Local.Paths.Include[index+1:]...)
	default:
		return OrbitSpec{}, fmt.Errorf("unsupported capability kind %q", kind)
	}

	spec.Capabilities = normalizeCapabilities(capabilities)

	return spec, nil
}

func cloneCapabilities(capabilities *OrbitCapabilities) OrbitCapabilities {
	if capabilities == nil {
		return OrbitCapabilities{}
	}

	cloned := OrbitCapabilities{}
	if capabilities.Commands != nil {
		cloned.Commands = &OrbitCommandCapabilityPaths{
			Paths: cloneOrbitMemberPaths(capabilities.Commands.Paths),
		}
	}
	if capabilities.Skills != nil {
		cloned.Skills = &OrbitSkillCapabilities{}
		if capabilities.Skills.Local != nil {
			cloned.Skills.Local = &OrbitLocalSkillCapabilityPaths{
				Paths: cloneOrbitMemberPaths(capabilities.Skills.Local.Paths),
			}
		}
		if capabilities.Skills.Remote != nil {
			cloned.Skills.Remote = &OrbitRemoteSkillCapabilities{
				URIs:         append([]string(nil), capabilities.Skills.Remote.URIs...),
				Dependencies: cloneRemoteSkillDependencies(capabilities.Skills.Remote.Dependencies),
			}
		}
	}

	return cloned
}

func normalizeCapabilities(capabilities OrbitCapabilities) *OrbitCapabilities {
	if capabilities.Commands == nil && capabilities.Skills == nil {
		return nil
	}

	normalized := OrbitCapabilities{}
	if capabilities.Commands != nil && (len(capabilities.Commands.Paths.Include) > 0 || len(capabilities.Commands.Paths.Exclude) > 0) {
		normalized.Commands = &OrbitCommandCapabilityPaths{
			Paths: cloneOrbitMemberPaths(capabilities.Commands.Paths),
		}
	}
	if capabilities.Skills != nil {
		skills := &OrbitSkillCapabilities{}
		if capabilities.Skills.Local != nil && (len(capabilities.Skills.Local.Paths.Include) > 0 || len(capabilities.Skills.Local.Paths.Exclude) > 0) {
			skills.Local = &OrbitLocalSkillCapabilityPaths{
				Paths: cloneOrbitMemberPaths(capabilities.Skills.Local.Paths),
			}
		}
		if capabilities.Skills.Remote != nil && (len(capabilities.Skills.Remote.URIs) > 0 || len(capabilities.Skills.Remote.Dependencies) > 0) {
			skills.Remote = &OrbitRemoteSkillCapabilities{
				URIs:         append([]string(nil), capabilities.Skills.Remote.URIs...),
				Dependencies: cloneRemoteSkillDependencies(capabilities.Skills.Remote.Dependencies),
			}
		}
		if skills.Local != nil || skills.Remote != nil {
			normalized.Skills = skills
		}
	}

	if normalized.Commands == nil && normalized.Skills == nil {
		return nil
	}

	return &normalized
}

func cloneOrbitMemberPaths(paths OrbitMemberPaths) OrbitMemberPaths {
	return OrbitMemberPaths{
		Include: append([]string(nil), paths.Include...),
		Exclude: append([]string(nil), paths.Exclude...),
	}
}

func cloneRemoteSkillDependencies(dependencies []OrbitRemoteSkillDependency) []OrbitRemoteSkillDependency {
	cloned := make([]OrbitRemoteSkillDependency, 0, len(dependencies))
	cloned = append(cloned, dependencies...)

	return cloned
}

func ensureCommandCapabilityPaths(paths *OrbitCommandCapabilityPaths) *OrbitCommandCapabilityPaths {
	if paths != nil {
		return paths
	}

	return &OrbitCommandCapabilityPaths{}
}

func ensureSkillCapabilities(skills *OrbitSkillCapabilities) *OrbitSkillCapabilities {
	if skills != nil {
		return skills
	}

	return &OrbitSkillCapabilities{}
}

func ensureLocalSkillCapabilityPaths(paths *OrbitLocalSkillCapabilityPaths) *OrbitLocalSkillCapabilityPaths {
	if paths != nil {
		return paths
	}

	return &OrbitLocalSkillCapabilityPaths{}
}

func hasCommandCapability(capabilities OrbitCapabilities, expectedName string) bool {
	return indexOfCommandCapability(capabilities, expectedName) >= 0
}

func indexOfCommandCapability(capabilities OrbitCapabilities, expectedName string) int {
	if capabilities.Commands == nil {
		return -1
	}
	for index, includePath := range capabilities.Commands.Paths.Include {
		name := strings.TrimSuffix(path.Base(includePath), ".md")
		if name == expectedName {
			return index
		}
	}

	return -1
}

func hasLocalSkillCapability(capabilities OrbitCapabilities, expectedName string) bool {
	return indexOfLocalSkillCapability(capabilities, expectedName) >= 0
}

func indexOfLocalSkillCapability(capabilities OrbitCapabilities, expectedName string) int {
	if capabilities.Skills == nil || capabilities.Skills.Local == nil {
		return -1
	}
	for index, includePath := range capabilities.Skills.Local.Paths.Include {
		if path.Base(includePath) == expectedName {
			return index
		}
	}

	return -1
}

func normalizeCommandCapabilityInput(id string, capabilityPath string) (string, string, error) {
	normalizedPath, err := ids.NormalizeRepoRelativePath(capabilityPath)
	if err != nil {
		return "", "", fmt.Errorf("normalize capability path: %w", err)
	}
	if err := validateCapabilityAssetRef(normalizedPath); err != nil {
		return "", "", fmt.Errorf("capabilities.commands.paths.include[0]: %w", err)
	}
	if !strings.HasSuffix(normalizedPath, ".md") {
		return "", "", fmt.Errorf(`command capability path %q must end with ".md"`, normalizedPath)
	}

	name := strings.TrimSuffix(path.Base(normalizedPath), ".md")
	if err := ids.ValidateOrbitID(name); err != nil {
		return "", "", fmt.Errorf("validate command capability name: %w", err)
	}
	if id != "" && id != name {
		return "", "", fmt.Errorf("command capability %q must match command basename %q", id, name)
	}

	return normalizedPath, name, nil
}

func normalizeLocalSkillCapabilityInput(id string, capabilityPath string) (string, string, error) {
	normalizedPath, err := ids.NormalizeRepoRelativePath(capabilityPath)
	if err != nil {
		return "", "", fmt.Errorf("normalize capability path: %w", err)
	}

	rootPath := normalizedPath
	if strings.HasSuffix(rootPath, "/SKILL.md") {
		rootPath = path.Dir(rootPath)
	}
	if err := validateCapabilityAssetRef(rootPath); err != nil {
		return "", "", fmt.Errorf("capabilities.skills.local.paths.include[0]: %w", err)
	}

	name := path.Base(rootPath)
	if err := ids.ValidateOrbitID(name); err != nil {
		return "", "", fmt.Errorf("validate skill capability name: %w", err)
	}
	if id != "" && id != name {
		return "", "", fmt.Errorf("skill capability %q must match skill basename %q", id, name)
	}

	return rootPath, name, nil
}

// SetCommandCapabilityPaths overwrites the canonical command capability path truth.
func SetCommandCapabilityPaths(spec OrbitSpec, include []string, exclude []string, clearAll bool) (OrbitSpec, error) {
	if clearAll && (len(include) > 0 || len(exclude) > 0) {
		return OrbitSpec{}, fmt.Errorf("commands-paths: --clear cannot be combined with --include or --exclude")
	}

	capabilities := cloneCapabilities(spec.Capabilities)
	if clearAll {
		capabilities.Commands = nil
		spec.Capabilities = normalizeCapabilities(capabilities)
		return spec, nil
	}

	normalizedPaths, err := normalizeCapabilityPathScope(include, exclude, "capabilities.commands.paths")
	if err != nil {
		return OrbitSpec{}, err
	}
	capabilities.Commands = &OrbitCommandCapabilityPaths{Paths: normalizedPaths}
	spec.Capabilities = normalizeCapabilities(capabilities)

	return spec, nil
}

// SetLocalSkillCapabilityPaths overwrites the canonical local-skill capability path truth.
func SetLocalSkillCapabilityPaths(spec OrbitSpec, include []string, exclude []string, clearAll bool) (OrbitSpec, error) {
	if clearAll && (len(include) > 0 || len(exclude) > 0) {
		return OrbitSpec{}, fmt.Errorf("skills-local-paths: --clear cannot be combined with --include or --exclude")
	}

	capabilities := cloneCapabilities(spec.Capabilities)
	if clearAll {
		capabilities.Skills = ensureSkillCapabilities(capabilities.Skills)
		capabilities.Skills.Local = nil
		spec.Capabilities = normalizeCapabilities(capabilities)
		return spec, nil
	}

	normalizedPaths, err := normalizeCapabilityPathScope(include, exclude, "capabilities.skills.local.paths")
	if err != nil {
		return OrbitSpec{}, err
	}
	capabilities.Skills = ensureSkillCapabilities(capabilities.Skills)
	capabilities.Skills.Local = &OrbitLocalSkillCapabilityPaths{Paths: normalizedPaths}
	spec.Capabilities = normalizeCapabilities(capabilities)

	return spec, nil
}

// SetRemoteSkillCapabilityURIs overwrites the canonical remote-skill capability URI truth.
func SetRemoteSkillCapabilityURIs(spec OrbitSpec, uris []string, clearAll bool) (OrbitSpec, error) {
	if clearAll && len(uris) > 0 {
		return OrbitSpec{}, fmt.Errorf("skills-remote-uris: --clear cannot be combined with --uri")
	}

	capabilities := cloneCapabilities(spec.Capabilities)
	if clearAll {
		capabilities.Skills = ensureSkillCapabilities(capabilities.Skills)
		capabilities.Skills.Remote = nil
		spec.Capabilities = normalizeCapabilities(capabilities)
		return spec, nil
	}
	if len(uris) == 0 {
		return OrbitSpec{}, fmt.Errorf("capabilities.skills.remote.uris must not be empty")
	}

	normalizedURIs := make([]string, 0, len(uris))
	for index, rawURI := range uris {
		normalizedURI, err := normalizeRemoteSkillURI(rawURI)
		if err != nil {
			return OrbitSpec{}, fmt.Errorf("capabilities.skills.remote.uris[%d]: %w", index, err)
		}
		normalizedURIs = append(normalizedURIs, normalizedURI)
	}

	capabilities.Skills = ensureSkillCapabilities(capabilities.Skills)
	capabilities.Skills.Remote = &OrbitRemoteSkillCapabilities{URIs: normalizedURIs}
	spec.Capabilities = normalizeCapabilities(capabilities)

	return spec, nil
}

// LinkRemoteSkillDependency adds or updates one structured remote skill dependency.
func LinkRemoteSkillDependency(spec OrbitSpec, uri string, required bool) (OrbitSpec, OrbitRemoteSkillDependency, error) {
	normalizedURI, err := normalizeRemoteSkillURI(uri)
	if err != nil {
		return OrbitSpec{}, OrbitRemoteSkillDependency{}, err
	}

	capabilities := cloneCapabilities(spec.Capabilities)
	capabilities.Skills = ensureSkillCapabilities(capabilities.Skills)
	if capabilities.Skills.Remote == nil {
		capabilities.Skills.Remote = &OrbitRemoteSkillCapabilities{}
	}
	if len(capabilities.Skills.Remote.URIs) > 0 {
		return OrbitSpec{}, OrbitRemoteSkillDependency{}, fmt.Errorf("capabilities.skills.remote.uris must be migrated before linking structured skill dependencies")
	}

	dependency := OrbitRemoteSkillDependency{URI: normalizedURI, Required: required}
	for index, existing := range capabilities.Skills.Remote.Dependencies {
		existingURI, err := normalizeRemoteSkillURI(existing.URI)
		if err != nil {
			return OrbitSpec{}, OrbitRemoteSkillDependency{}, fmt.Errorf("capabilities.skills.remote.dependencies[%d].uri: %w", index, err)
		}
		if existingURI != normalizedURI {
			continue
		}
		capabilities.Skills.Remote.Dependencies[index] = dependency
		spec.Capabilities = normalizeCapabilities(capabilities)
		return spec, dependency, nil
	}
	capabilities.Skills.Remote.Dependencies = append(capabilities.Skills.Remote.Dependencies, dependency)
	spec.Capabilities = normalizeCapabilities(capabilities)

	return spec, dependency, nil
}

// UnlinkRemoteSkillDependency removes one structured remote skill dependency.
func UnlinkRemoteSkillDependency(spec OrbitSpec, uri string) (OrbitSpec, OrbitRemoteSkillDependency, error) {
	normalizedURI, err := normalizeRemoteSkillURI(uri)
	if err != nil {
		return OrbitSpec{}, OrbitRemoteSkillDependency{}, err
	}
	if spec.Capabilities == nil || spec.Capabilities.Skills == nil || spec.Capabilities.Skills.Remote == nil {
		return OrbitSpec{}, OrbitRemoteSkillDependency{}, fmt.Errorf("remote skill dependency %q is not linked", normalizedURI)
	}

	capabilities := cloneCapabilities(spec.Capabilities)
	if len(capabilities.Skills.Remote.URIs) > 0 {
		return OrbitSpec{}, OrbitRemoteSkillDependency{}, fmt.Errorf("capabilities.skills.remote.uris must be migrated before unlinking structured skill dependencies")
	}
	dependencies := capabilities.Skills.Remote.Dependencies
	for index, existing := range dependencies {
		existingURI, err := normalizeRemoteSkillURI(existing.URI)
		if err != nil {
			return OrbitSpec{}, OrbitRemoteSkillDependency{}, fmt.Errorf("capabilities.skills.remote.dependencies[%d].uri: %w", index, err)
		}
		if existingURI != normalizedURI {
			continue
		}
		removed := OrbitRemoteSkillDependency{URI: existingURI, Required: existing.Required}
		capabilities.Skills.Remote.Dependencies = append(dependencies[:index], dependencies[index+1:]...)
		spec.Capabilities = normalizeCapabilities(capabilities)
		return spec, removed, nil
	}

	return OrbitSpec{}, OrbitRemoteSkillDependency{}, fmt.Errorf("remote skill dependency %q is not linked", normalizedURI)
}

func normalizeCapabilityPathScope(include []string, exclude []string, field string) (OrbitMemberPaths, error) {
	if len(include) == 0 {
		return OrbitMemberPaths{}, fmt.Errorf("%s.include must not be empty", field)
	}

	normalized := OrbitMemberPaths{
		Include: make([]string, 0, len(include)),
		Exclude: make([]string, 0, len(exclude)),
	}
	for index, pattern := range include {
		normalizedPattern, err := normalizePattern(pattern)
		if err != nil {
			return OrbitMemberPaths{}, fmt.Errorf("%s.include[%d]: %w", field, index, err)
		}
		if err := validateCapabilityAssetRef(normalizedPattern); err != nil {
			return OrbitMemberPaths{}, fmt.Errorf("%s.include[%d]: %w", field, index, err)
		}
		normalized.Include = append(normalized.Include, normalizedPattern)
	}
	for index, pattern := range exclude {
		normalizedPattern, err := normalizePattern(pattern)
		if err != nil {
			return OrbitMemberPaths{}, fmt.Errorf("%s.exclude[%d]: %w", field, index, err)
		}
		if err := validateCapabilityAssetRef(normalizedPattern); err != nil {
			return OrbitMemberPaths{}, fmt.Errorf("%s.exclude[%d]: %w", field, index, err)
		}
		normalized.Exclude = append(normalized.Exclude, normalizedPattern)
	}

	return normalized, nil
}
