package orbit

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// ValidateGlobalConfig validates the documented MVP behavior configuration.
func ValidateGlobalConfig(config GlobalConfig) error {
	if config.Version <= 0 {
		return errors.New("config version must be positive")
	}

	switch config.Behavior.OutsideChangesMode {
	case OutsideChangesModeWarn:
	default:
		return fmt.Errorf("outside_changes_mode must be %q", OutsideChangesModeWarn)
	}

	switch config.Behavior.SparseCheckoutMode {
	case SparseCheckoutModeNoCone:
	default:
		return fmt.Errorf("sparse_checkout_mode must be %q", SparseCheckoutModeNoCone)
	}

	for index, pattern := range config.SharedScope {
		normalizedPattern, err := normalizePattern(pattern)
		if err != nil {
			return fmt.Errorf("shared_scope[%d]: %w", index, err)
		}
		if err := validateScopeOverlayPattern(normalizedPattern); err != nil {
			return fmt.Errorf("shared_scope[%d]: %w", index, err)
		}
	}

	for index, pattern := range config.ProjectionVisible {
		normalizedPattern, err := normalizePattern(pattern)
		if err != nil {
			return fmt.Errorf("projection_visible[%d]: %w", index, err)
		}
		if err := validateScopeOverlayPattern(normalizedPattern); err != nil {
			return fmt.Errorf("projection_visible[%d]: %w", index, err)
		}
	}

	return nil
}

// ValidateOrbitSpec validates either the legacy path-list schema or the new
// member-model schema used by the compatibility parser.
func ValidateOrbitSpec(spec OrbitSpec) error {
	return validateOrbitSpecWithPathBuilder(spec, true, DefinitionRelativePath)
}

// ValidateHostedOrbitSpec validates a hosted OrbitSpec whose steady-state control path lives under .harness/orbits/.
func ValidateHostedOrbitSpec(spec OrbitSpec) error {
	return validateOrbitSpecWithPathBuilder(spec, true, HostedDefinitionRelativePath)
}

func validateOrbitSpecWithPathBuilder(
	spec OrbitSpec,
	validateSourcePath bool,
	pathBuilder func(string) (string, error),
) error {
	normalizedSpec, err := normalizeOrbitSpecBehaviorAlias(spec)
	if err != nil {
		return fmt.Errorf("normalize orbit behavior: %w", err)
	}
	spec = normalizedSpec
	if err := ids.ValidateOrbitID(spec.ID); err != nil {
		return fmt.Errorf("validate orbit id: %w", err)
	}
	if validateSourcePath && spec.SourcePath != "" {
		expectedName := spec.ID + ".yaml"
		if filepath.Base(spec.SourcePath) != expectedName {
			return fmt.Errorf("definition filename must match orbit id: expected %q", expectedName)
		}
	}

	if !spec.HasMemberSchema() {
		return validateDefinition(spec.LegacyDefinition(), validateSourcePath)
	}
	normalized, err := normalizeOrbitSpecMemberIdentities(spec)
	if err != nil {
		return err
	}
	spec = normalized

	if len(spec.Include) > 0 || len(spec.Exclude) > 0 {
		return errors.New("legacy include/exclude cannot be combined with member schema")
	}
	if spec.Meta == nil {
		return errors.New("meta must be present when using member schema")
	}
	if err := validateOrbitAgentAddons(spec.AgentAddons); err != nil {
		return err
	}

	expectedMetaFile, err := pathBuilder(spec.ID)
	if err != nil {
		return fmt.Errorf("build expected meta.file: %w", err)
	}
	if spec.Meta.File != expectedMetaFile {
		return fmt.Errorf("meta.file must match %q", expectedMetaFile)
	}

	seenNames := make(map[string]struct{}, len(spec.Members))
	for index, member := range spec.Members {
		if strings.TrimSpace(member.Name) == "" {
			return fmt.Errorf("members[%d].name must be present", index)
		}
		if _, ok := seenNames[member.Name]; ok {
			return fmt.Errorf("members[%d].name must be unique", index)
		}
		seenNames[member.Name] = struct{}{}

		if !member.Role.IsValid() {
			return fmt.Errorf("members[%d].role: invalid orbit member role %q", index, member.Role)
		}
		if member.Lane != "" && member.Lane != OrbitMemberLaneBootstrap {
			return fmt.Errorf(`members[%d].lane must be %q when present`, index, OrbitMemberLaneBootstrap)
		}
		if len(member.Paths.Include) == 0 {
			return fmt.Errorf("members[%d].paths.include must not be empty", index)
		}

		for patternIndex, pattern := range member.Paths.Include {
			if _, err := normalizePattern(pattern); err != nil {
				return fmt.Errorf("members[%d].paths.include[%d]: %w", index, patternIndex, err)
			}
		}
		for patternIndex, pattern := range member.Paths.Exclude {
			if _, err := normalizePattern(pattern); err != nil {
				return fmt.Errorf("members[%d].paths.exclude[%d]: %w", index, patternIndex, err)
			}
		}
	}

	if spec.Behavior != nil {
		if err := validateOrbitSpecRoleList(spec.Behavior.Scope.ProjectionRoles, "behavior.scope.projection_roles"); err != nil {
			return err
		}
		if err := validateOrbitSpecRoleList(spec.Behavior.Scope.WriteRoles, "behavior.scope.write_roles"); err != nil {
			return err
		}
		if err := validateOrbitSpecRoleList(spec.Behavior.Scope.ExportRoles, "behavior.scope.export_roles"); err != nil {
			return err
		}
		if err := validateOrbitSpecRoleList(spec.Behavior.Scope.OrchestrationRoles, "behavior.scope.orchestration_roles"); err != nil {
			return err
		}
	}
	if err := validateOrbitCapabilities(spec.Capabilities); err != nil {
		return err
	}
	if err := validateOrbitAgentAddons(spec.AgentAddons); err != nil {
		return err
	}
	if err := validateMemberCapabilityPathOverlap(spec); err != nil {
		return err
	}

	return nil
}

// ValidateDefinition validates an individual orbit definition.
func ValidateDefinition(definition Definition) error {
	return validateDefinition(definition, true)
}

func validateDefinition(definition Definition, validateSourcePath bool) error {
	if err := ids.ValidateOrbitID(definition.ID); err != nil {
		return fmt.Errorf("validate orbit id: %w", err)
	}
	if validateSourcePath && definition.SourcePath != "" {
		expectedName := definition.ID + ".yaml"
		if filepath.Base(definition.SourcePath) != expectedName {
			return fmt.Errorf("definition filename must match orbit id: expected %q", expectedName)
		}
	}
	if len(definition.Include) == 0 {
		return errors.New("include must not be empty")
	}

	for index, pattern := range definition.Include {
		if _, err := normalizePattern(pattern); err != nil {
			return fmt.Errorf("include[%d]: %w", index, err)
		}
	}
	for index, pattern := range definition.Exclude {
		if _, err := normalizePattern(pattern); err != nil {
			return fmt.Errorf("exclude[%d]: %w", index, err)
		}
	}

	return nil
}

func validateOrbitSpecRoleList(roles []OrbitMemberRole, field string) error {
	for index, role := range roles {
		if !role.IsValid() {
			return fmt.Errorf("%s[%d]: invalid orbit member role %q", field, index, role)
		}
	}

	return nil
}

func validateOrbitCapabilities(capabilities *OrbitCapabilities) error {
	if capabilities == nil {
		return nil
	}
	if err := validateCommandCapabilities(capabilities.Commands); err != nil {
		return err
	}
	if err := validateSkillCapabilities(capabilities.Skills); err != nil {
		return err
	}

	return nil
}

func validateMemberCapabilityPathOverlap(spec OrbitSpec) error {
	overlaps, err := FindMemberCapabilityPathOverlaps(spec)
	if err != nil {
		return err
	}
	if len(overlaps) == 0 {
		return nil
	}

	overlap := overlaps[0]
	return fmt.Errorf(
		`members[%d].paths.include[%d] overlaps capability-owned %s path %q; paths are managed by %s`,
		overlap.MemberIndex,
		overlap.IncludeIndex,
		overlap.CapabilityKind,
		overlap.CapabilityPattern,
		overlap.CapabilityField,
	)
}

func validateCommandCapabilities(commands *OrbitCommandCapabilityPaths) error {
	if commands == nil {
		return nil
	}

	return validateCapabilityPaths(commands.Paths, "capabilities.commands.paths")
}

func validateSkillCapabilities(skills *OrbitSkillCapabilities) error {
	if skills == nil {
		return nil
	}
	if skills.Local != nil {
		if err := validateCapabilityPaths(skills.Local.Paths, "capabilities.skills.local.paths"); err != nil {
			return err
		}
	}
	if skills.Remote != nil {
		if err := validateRemoteSkillCapabilities(skills.Remote); err != nil {
			return err
		}
	}

	return nil
}

func validateRemoteSkillCapabilities(remote *OrbitRemoteSkillCapabilities) error {
	if remote == nil {
		return nil
	}
	if len(remote.URIs) > 0 && len(remote.Dependencies) > 0 {
		return fmt.Errorf("capabilities.skills.remote must not define both uris and dependencies")
	}
	seen := map[string]struct{}{}
	for index, rawURI := range remote.URIs {
		normalizedURI, err := normalizeRemoteSkillURI(rawURI)
		if err != nil {
			return fmt.Errorf("capabilities.skills.remote.uris[%d]: %w", index, err)
		}
		if _, ok := seen[normalizedURI]; ok {
			return fmt.Errorf("capabilities.skills.remote.uris[%d]: duplicate remote skill URI %q", index, normalizedURI)
		}
		seen[normalizedURI] = struct{}{}
	}
	for index, dependency := range remote.Dependencies {
		normalizedURI, err := normalizeRemoteSkillURI(dependency.URI)
		if err != nil {
			return fmt.Errorf("capabilities.skills.remote.dependencies[%d].uri: %w", index, err)
		}
		if _, ok := seen[normalizedURI]; ok {
			return fmt.Errorf("capabilities.skills.remote.dependencies[%d].uri: duplicate remote skill URI %q", index, normalizedURI)
		}
		seen[normalizedURI] = struct{}{}
	}

	return nil
}

func validateOrbitAgentAddons(addons *OrbitAgentAddons) error {
	if addons == nil || addons.Hooks == nil {
		return nil
	}

	hooks := addons.Hooks
	switch hooks.UnsupportedBehavior {
	case "", "skip", "block":
	default:
		return fmt.Errorf("agent_addons.hooks.unsupported_behavior must be skip or block")
	}
	seen := map[string]struct{}{}
	for index, entry := range hooks.Entries {
		prefix := fmt.Sprintf("agent_addons.hooks.entries[%d]", index)
		if strings.TrimSpace(entry.ID) == "" {
			return fmt.Errorf("%s.id must not be empty", prefix)
		}
		if err := ids.ValidateOrbitID(entry.ID); err != nil {
			return fmt.Errorf("%s.id: %w", prefix, err)
		}
		if _, ok := seen[entry.ID]; ok {
			return fmt.Errorf("%s.id %q is duplicated", prefix, entry.ID)
		}
		seen[entry.ID] = struct{}{}
		if strings.TrimSpace(entry.Event.Kind) == "" {
			return fmt.Errorf("%s.event.kind must not be empty", prefix)
		}
		if !isSupportedOrbitAgentHookEvent(entry.Event.Kind) {
			return fmt.Errorf("%s.event.kind %q is not supported", prefix, entry.Event.Kind)
		}
		for toolIndex, tool := range entry.Match.Tools {
			if strings.TrimSpace(tool) == "" {
				return fmt.Errorf("%s.match.tools[%d] must not be empty", prefix, toolIndex)
			}
		}
		for patternIndex, pattern := range entry.Match.CommandPatterns {
			if strings.TrimSpace(pattern) == "" {
				return fmt.Errorf("%s.match.command_patterns[%d] must not be empty", prefix, patternIndex)
			}
		}
		if entry.Handler.Type == "" {
			return fmt.Errorf("%s.handler.type must not be empty", prefix)
		}
		if entry.Handler.Type != "command" {
			return fmt.Errorf("%s.handler.type must be command", prefix)
		}
		if strings.TrimSpace(entry.Handler.Path) == "" {
			return fmt.Errorf("%s.handler.path must not be empty", prefix)
		}
		if err := validateOrbitAgentHookHandlerPath(entry.Handler.Path); err != nil {
			return fmt.Errorf("%s.handler.path: %w", prefix, err)
		}
		if entry.Handler.TimeoutSeconds < 0 {
			return fmt.Errorf("%s.handler.timeout_seconds must be positive", prefix)
		}
		for target := range entry.Targets {
			if !isSupportedOrbitAgentHookTarget(target) {
				return fmt.Errorf("%s.targets.%s is not supported by this build", prefix, target)
			}
		}
	}

	return nil
}

func isSupportedOrbitAgentHookEvent(kind string) bool {
	switch kind {
	case "session.start",
		"prompt.before_submit",
		"tool.before",
		"permission.request",
		"tool.after",
		"turn.stop",
		"compact.before",
		"compact.after":
		return true
	default:
		return false
	}
}

func isSupportedOrbitAgentHookTarget(target string) bool {
	switch target {
	case "codex", "claude", "openclaw":
		return true
	default:
		return false
	}
}

func validateOrbitAgentHookHandlerPath(repoPath string) error {
	trimmed := strings.TrimSpace(repoPath)
	if trimmed == "" {
		return fmt.Errorf("must not be empty")
	}
	if strings.Contains(trimmed, "://") {
		return fmt.Errorf("remote handler URLs are not supported")
	}
	if _, err := ids.NormalizeRepoRelativePath(trimmed); err != nil {
		return fmt.Errorf("normalize handler path: %w", err)
	}

	return nil
}

func validateCapabilityPaths(paths OrbitMemberPaths, field string) error {
	if len(paths.Include) == 0 {
		return fmt.Errorf("%s.include must not be empty", field)
	}
	for index, pattern := range paths.Include {
		normalizedPattern, err := normalizePattern(pattern)
		if err != nil {
			return fmt.Errorf("%s.include[%d]: %w", field, index, err)
		}
		if err := validateCapabilityAssetRef(normalizedPattern); err != nil {
			return fmt.Errorf("%s.include[%d]: %w", field, index, err)
		}
	}
	for index, pattern := range paths.Exclude {
		normalizedPattern, err := normalizePattern(pattern)
		if err != nil {
			return fmt.Errorf("%s.exclude[%d]: %w", field, index, err)
		}
		if err := validateCapabilityAssetRef(normalizedPattern); err != nil {
			return fmt.Errorf("%s.exclude[%d]: %w", field, index, err)
		}
	}

	return nil
}

func normalizeRemoteSkillURI(rawURI string) (string, error) {
	trimmed := strings.TrimSpace(rawURI)
	if trimmed == "" {
		return "", fmt.Errorf("remote skill URI must not be empty")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse remote skill URI %q: %w", trimmed, err)
	}
	switch parsed.Scheme {
	case "github", "https":
		return trimmed, nil
	default:
		return "", fmt.Errorf("unsupported remote skill URI scheme %q", parsed.Scheme)
	}
}

func validateCapabilityAssetRef(normalizedPath string) error {
	switch {
	case normalizedPath == "AGENTS.md", normalizedPath == "HUMANS.md":
		return fmt.Errorf("path %q must not target runtime guidance artifacts", normalizedPath)
	case normalizedPath == configRelativePath:
		return fmt.Errorf("path %q must not target Orbit control-plane files", normalizedPath)
	case strings.HasPrefix(normalizedPath, orbitDirName+"/"):
		return fmt.Errorf("path %q must not target Orbit control-plane files", normalizedPath)
	case strings.HasPrefix(normalizedPath, ".harness/"):
		return fmt.Errorf("path %q must not target harness control-plane files", normalizedPath)
	case strings.HasPrefix(normalizedPath, ".git/orbit/state/"):
		return fmt.Errorf("path %q must not target repo-local runtime state", normalizedPath)
	default:
		return nil
	}
}

// ValidateRepositoryConfig validates the full repository config set.
func ValidateRepositoryConfig(config GlobalConfig, definitions []Definition) error {
	if err := ValidateGlobalConfig(config); err != nil {
		return fmt.Errorf("validate global config: %w", err)
	}

	seen := make(map[string]string, len(definitions))

	for index, definition := range definitions {
		source := definition.SourcePath
		if source == "" {
			source = fmt.Sprintf("orbit[%d]", index)
		}

		if definition.ID != "" {
			if previousSource, exists := seen[definition.ID]; exists {
				return fmt.Errorf("duplicate orbit id %q in %s and %s", definition.ID, previousSource, source)
			}

			seen[definition.ID] = source
		}

		if definition.MemberSchema && len(definition.Include) == 0 {
			if err := validateZeroMemberSchemaDefinition(definition); err != nil {
				label := definition.ID
				if label == "" {
					label = fmt.Sprintf("orbit[%d]", index)
				}
				return fmt.Errorf("%s: %w", label, err)
			}
			continue
		}

		if err := ValidateDefinition(definition); err != nil {
			label := definition.ID
			if label == "" {
				label = fmt.Sprintf("orbit[%d]", index)
			}
			return fmt.Errorf("%s: %w", label, err)
		}
	}

	return nil
}

func validateZeroMemberSchemaDefinition(definition Definition) error {
	if err := ids.ValidateOrbitID(definition.ID); err != nil {
		return fmt.Errorf("validate orbit id: %w", err)
	}
	if definition.SourcePath != "" {
		expectedName := definition.ID + ".yaml"
		if filepath.Base(definition.SourcePath) != expectedName {
			return fmt.Errorf("definition filename must match orbit id: expected %q", expectedName)
		}
	}

	return nil
}

// ValidateHostedRepositoryConfig validates hosted OrbitSpecs without requiring
// a compatibility include scope. This allows freshly authored hosted specs to
// start with zero members before member authoring fills in explicit scope.
func ValidateHostedRepositoryConfig(config GlobalConfig, specs []OrbitSpec) error {
	if err := ValidateGlobalConfig(config); err != nil {
		return fmt.Errorf("validate global config: %w", err)
	}

	seen := make(map[string]string, len(specs))
	for index, spec := range specs {
		source := spec.SourcePath
		if source == "" {
			source = fmt.Sprintf("orbit[%d]", index)
		}

		if spec.ID != "" {
			if previousSource, exists := seen[spec.ID]; exists {
				return fmt.Errorf("duplicate orbit id %q in %s and %s", spec.ID, previousSource, source)
			}
			seen[spec.ID] = source
		}

		if err := ValidateHostedOrbitSpec(spec); err != nil {
			label := spec.ID
			if label == "" {
				label = fmt.Sprintf("orbit[%d]", index)
			}
			return fmt.Errorf("%s: %w", label, err)
		}
	}

	return nil
}

func validateScopeOverlayPattern(normalizedPattern string) error {
	matchesConfig, err := matchNormalizedAny([]string{normalizedPattern}, configRelativePath)
	if err != nil {
		return fmt.Errorf("match config path: %w", err)
	}
	if matchesConfig {
		return fmt.Errorf("pattern %q must not match Orbit control-plane paths", normalizedPattern)
	}

	sampleDefinitionPath := path.Join(orbitsRelativeDir, "sample.yaml")

	matchesDefinition, err := matchNormalizedAny([]string{normalizedPattern}, sampleDefinitionPath)
	if err != nil {
		return fmt.Errorf("match orbit definition path: %w", err)
	}
	if matchesDefinition {
		return fmt.Errorf("pattern %q must not match Orbit control-plane paths", normalizedPattern)
	}

	if strings.HasPrefix(normalizedPattern, orbitDirName+"/") {
		return fmt.Errorf("pattern %q must not match Orbit control-plane paths", normalizedPattern)
	}

	if strings.HasPrefix(normalizedPattern, ".harness/") {
		return fmt.Errorf("pattern %q must not match Orbit control-plane paths or runtime-only paths", normalizedPattern)
	}

	if strings.HasPrefix(normalizedPattern, ".git/orbit/state/") {
		return fmt.Errorf("pattern %q must not match Orbit control-plane paths or runtime-only paths", normalizedPattern)
	}

	return nil
}
