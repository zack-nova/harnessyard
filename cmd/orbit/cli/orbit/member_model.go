package orbit

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// OrbitSpec is the member-model superset used by the post-v0.3 runtime design.
// It intentionally coexists with the current Definition shape until the
// compatibility parser is switched over in a later phase.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitSpec struct {
	Package      *ids.PackageIdentity `json:"-" yaml:"package,omitempty"`
	ID           string               `yaml:"id,omitempty"`
	Name         string               `yaml:"name,omitempty"`
	Description  string               `yaml:"description,omitempty"`
	Include      []string             `yaml:"include,omitempty"`
	Exclude      []string             `yaml:"exclude,omitempty"`
	Meta         *OrbitMeta           `yaml:"meta,omitempty"`
	Capabilities *OrbitCapabilities   `yaml:"capabilities,omitempty"`
	AgentAddons  *OrbitAgentAddons    `yaml:"agent_addons,omitempty"`
	Members      []OrbitMember        `yaml:"members,omitempty"`
	Content      []OrbitMember        `json:"-" yaml:"content,omitempty"`
	Behavior     *OrbitBehavior       `yaml:"behavior,omitempty"`
	Rules        *OrbitBehavior       `json:"-" yaml:"rules,omitempty"`
	SourcePath   string               `yaml:"-"`
}

// MarshalYAML writes the package-native OrbitSpec shape. Internal field names
// remain stable for Go callers, while authored truth is package/content based.
func (spec OrbitSpec) MarshalYAML() (interface{}, error) {
	type orbitSpecYAML struct {
		Package      *ids.PackageIdentity `yaml:"package"`
		Name         string               `yaml:"name,omitempty"`
		Description  string               `yaml:"description,omitempty"`
		Include      []string             `yaml:"include,omitempty"`
		Exclude      []string             `yaml:"exclude,omitempty"`
		Meta         *OrbitMeta           `yaml:"meta,omitempty"`
		Capabilities *OrbitCapabilities   `yaml:"capabilities,omitempty"`
		AgentAddons  *OrbitAgentAddons    `yaml:"agent_addons,omitempty"`
		Content      []OrbitMember        `yaml:"content,omitempty"`
		Behavior     *OrbitBehavior       `yaml:"behavior,omitempty"`
	}

	normalized, err := normalizeOrbitSpecPackageIdentity(spec)
	if err != nil {
		return nil, err
	}
	normalized, err = normalizeOrbitSpecBehaviorAlias(normalized)
	if err != nil {
		return nil, err
	}

	return orbitSpecYAML{
		Package:      normalized.Package,
		Name:         normalized.Name,
		Description:  normalized.Description,
		Include:      append([]string(nil), normalized.Include...),
		Exclude:      append([]string(nil), normalized.Exclude...),
		Meta:         normalized.Meta,
		Capabilities: normalized.Capabilities,
		AgentAddons:  normalized.AgentAddons,
		Content:      append([]OrbitMember(nil), normalized.Members...),
		Behavior:     normalized.Behavior,
	}, nil
}

// OrbitMeta captures the singleton metadata member that is stored by the
// definition file itself.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitMeta struct {
	File                              string `yaml:"file"`
	AgentsTemplate                    string `yaml:"agents_template,omitempty"`
	HumansTemplate                    string `yaml:"humans_template,omitempty"`
	BootstrapTemplate                 string `yaml:"bootstrap_template,omitempty"`
	IncludeInProjection               bool   `yaml:"include_in_projection"`
	IncludeInWrite                    bool   `yaml:"include_in_write"`
	IncludeInExport                   bool   `yaml:"include_in_export"`
	IncludeDescriptionInOrchestration bool   `yaml:"include_description_in_orchestration"`
}

// OrbitCapabilities captures framework-agnostic capability declarations authored by one orbit.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitCapabilities struct {
	Commands *OrbitCommandCapabilityPaths `json:"commands,omitempty" yaml:"commands,omitempty"`
	Skills   *OrbitSkillCapabilities      `json:"skills,omitempty" yaml:"skills,omitempty"`
}

// OrbitCommandCapabilityPaths captures the authored command path scope.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitCommandCapabilityPaths struct {
	Paths OrbitMemberPaths `json:"paths" yaml:"paths"`
}

// OrbitSkillCapabilities captures local and remote skill capability truth.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitSkillCapabilities struct {
	Local  *OrbitLocalSkillCapabilityPaths `json:"local,omitempty" yaml:"local,omitempty"`
	Remote *OrbitRemoteSkillCapabilities   `json:"remote,omitempty" yaml:"remote,omitempty"`
}

// OrbitLocalSkillCapabilityPaths captures the authored local-skill path scope.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitLocalSkillCapabilityPaths struct {
	Paths OrbitMemberPaths `json:"paths" yaml:"paths"`
}

// OrbitRemoteSkillCapabilities captures authored remote skill dependency truth.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitRemoteSkillCapabilities struct {
	URIs         []string                     `json:"uris,omitempty" yaml:"uris,omitempty"`
	Dependencies []OrbitRemoteSkillDependency `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
}

// OrbitRemoteSkillDependency captures one authored Orbit-to-remote-skill link.
//
//nolint:revive // OrbitSpec model names keep the orbit-scoped domain prefix across packages.
type OrbitRemoteSkillDependency struct {
	URI      string `json:"uri" yaml:"uri"`
	Required bool   `json:"required" yaml:"required"`
}

// OrbitAgentAddons captures package-scoped agent add-on intent authored by one orbit.
//
//nolint:revive // OrbitSpec model names keep the orbit-scoped domain prefix across packages.
type OrbitAgentAddons struct {
	Hooks *OrbitAgentHookAddons `json:"hooks,omitempty" yaml:"hooks,omitempty"`
}

// OrbitAgentHookAddons captures package-scoped unified hook intent.
//
//nolint:revive // OrbitSpec model names keep the orbit-scoped domain prefix across packages.
type OrbitAgentHookAddons struct {
	UnsupportedBehavior string                `json:"unsupported_behavior,omitempty" yaml:"unsupported_behavior,omitempty"`
	Entries             []OrbitAgentHookEntry `json:"entries,omitempty" yaml:"entries,omitempty"`
}

// OrbitAgentHookEntry captures one package-scoped unified hook declaration.
//
//nolint:revive // OrbitSpec model names keep the orbit-scoped domain prefix across packages.
type OrbitAgentHookEntry struct {
	ID          string                `json:"id" yaml:"id"`
	Required    bool                  `json:"required,omitempty" yaml:"required,omitempty"`
	Description string                `json:"description,omitempty" yaml:"description,omitempty"`
	Event       AgentAddonHookEvent   `json:"event" yaml:"event"`
	Match       AgentAddonHookMatch   `json:"match,omitempty" yaml:"match,omitempty"`
	Handler     AgentAddonHookHandler `json:"handler" yaml:"handler"`
	Targets     map[string]bool       `json:"targets,omitempty" yaml:"targets,omitempty"`
}

// AgentAddonHookEvent captures the unified hook event kind.
type AgentAddonHookEvent struct {
	Kind string `json:"kind" yaml:"kind"`
}

// AgentAddonHookMatch captures framework-neutral match rules.
type AgentAddonHookMatch struct {
	Tools           []string `json:"tools,omitempty" yaml:"tools,omitempty"`
	CommandPatterns []string `json:"command_patterns,omitempty" yaml:"command_patterns,omitempty"`
}

// AgentAddonHookHandler captures the package-local hook implementation entrypoint.
type AgentAddonHookHandler struct {
	Type           string `json:"type" yaml:"type"`
	Path           string `json:"path" yaml:"path"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	StatusMessage  string `json:"status_message,omitempty" yaml:"status_message,omitempty"`
}

// ResolvedCommandCapability captures one derived command capability.
type ResolvedCommandCapability struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ResolvedLocalSkillCapability captures one derived local skill capability.
type ResolvedLocalSkillCapability struct {
	Name        string `json:"name"`
	RootPath    string `json:"root_path"`
	SkillMDPath string `json:"skill_md_path"`
}

// ResolvedRemoteSkillCapability captures one derived remote skill capability.
type ResolvedRemoteSkillCapability struct {
	URI      string `json:"uri"`
	Required bool   `json:"required,omitempty"`
}

// ResolvedAgentAddonHook captures one derived package-scoped hook add-on.
type ResolvedAgentAddonHook struct {
	Package             string          `json:"package"`
	ID                  string          `json:"id"`
	DisplayID           string          `json:"display_id"`
	Required            bool            `json:"required,omitempty"`
	Description         string          `json:"description,omitempty"`
	EventKind           string          `json:"event_kind"`
	Tools               []string        `json:"tools,omitempty"`
	CommandPatterns     []string        `json:"command_patterns,omitempty"`
	HandlerType         string          `json:"handler_type"`
	HandlerPath         string          `json:"handler_path"`
	TimeoutSeconds      int             `json:"timeout_seconds,omitempty"`
	StatusMessage       string          `json:"status_message,omitempty"`
	Targets             map[string]bool `json:"targets,omitempty"`
	UnsupportedBehavior string          `json:"unsupported_behavior,omitempty"`
}

// OrbitMemberRole is the fixed role taxonomy for orbit members.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitMemberRole string

const (
	OrbitMemberMeta    OrbitMemberRole = "meta"
	OrbitMemberSubject OrbitMemberRole = "subject"
	OrbitMemberRule    OrbitMemberRole = "rule"
	OrbitMemberProcess OrbitMemberRole = "process"
)

var orbitMemberRoles = []OrbitMemberRole{
	OrbitMemberMeta,
	OrbitMemberSubject,
	OrbitMemberRule,
	OrbitMemberProcess,
}

const (
	// OrbitMemberLaneBootstrap marks one member as bootstrap-only lifecycle content.
	OrbitMemberLaneBootstrap = "bootstrap"
)

// AllOrbitMemberRoles returns the fixed role list in stable order.
func AllOrbitMemberRoles() []OrbitMemberRole {
	return append([]OrbitMemberRole(nil), orbitMemberRoles...)
}

// IsValid reports whether the role is one of the fixed schema-backed values.
func (role OrbitMemberRole) IsValid() bool {
	for _, candidate := range orbitMemberRoles {
		if role == candidate {
			return true
		}
	}

	return false
}

// ParseOrbitMemberRole parses one fixed member role.
func ParseOrbitMemberRole(raw string) (OrbitMemberRole, error) {
	role := OrbitMemberRole(raw)
	if !role.IsValid() {
		return "", fmt.Errorf("invalid orbit member role %q", raw)
	}

	return role, nil
}

// OrbitMember stores one authored file member in the member model.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitMember struct {
	Key         string                 `yaml:"key,omitempty"`
	Name        string                 `yaml:"name,omitempty"`
	Description string                 `yaml:"description,omitempty"`
	Role        OrbitMemberRole        `yaml:"role"`
	Paths       OrbitMemberPaths       `yaml:"paths"`
	Lane        string                 `yaml:"lane,omitempty"`
	Scopes      *OrbitMemberScopePatch `yaml:"scopes,omitempty"`
}

// OrbitMemberPaths captures the authored include/exclude path set for one member.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitMemberPaths struct {
	Include []string `yaml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty"`
}

// OrbitMemberScopePatch carries explicit scope overrides for one member.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitMemberScopePatch struct {
	Write         *bool `yaml:"write,omitempty"`
	Export        *bool `yaml:"export,omitempty"`
	Orchestration *bool `yaml:"orchestration,omitempty"`
}

// OrbitBehavior captures authored behavior defaults for the member model.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitBehavior struct {
	Scope         OrbitBehaviorScope         `yaml:"scope,omitempty"`
	Orchestration OrbitBehaviorOrchestration `yaml:"orchestration,omitempty"`
}

// OrbitBehaviorScope stores role-to-scope behavior selections.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitBehaviorScope struct {
	ProjectionRoles    []OrbitMemberRole `yaml:"projection_roles,omitempty"`
	WriteRoles         []OrbitMemberRole `yaml:"write_roles,omitempty"`
	ExportRoles        []OrbitMemberRole `yaml:"export_roles,omitempty"`
	OrchestrationRoles []OrbitMemberRole `yaml:"orchestration_roles,omitempty"`
}

// OrbitBehaviorOrchestration stores orchestration-specific authored behavior.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
type OrbitBehaviorOrchestration struct {
	IncludeOrbitDescription   bool `yaml:"include_orbit_description"`
	MaterializeAgentsFromMeta bool `yaml:"materialize_agents_from_meta"`
}

// ProjectionPlan is the role-aware internal plan targeted by later runtime phases.
type ProjectionPlan struct {
	OrbitID            string
	ControlPaths       []string
	MetaPaths          []string
	SubjectPaths       []string
	RulePaths          []string
	ProcessPaths       []string
	CapabilityPaths    []string
	ProjectionPaths    []string
	OrbitWritePaths    []string
	ExportPaths        []string
	OrchestrationPaths []string
	PlanHash           string
}

// PathsForRole returns the role-owned path group from the plan.
func (plan ProjectionPlan) PathsForRole(role OrbitMemberRole) []string {
	switch role {
	case OrbitMemberMeta:
		return append([]string(nil), plan.MetaPaths...)
	case OrbitMemberSubject:
		return append([]string(nil), plan.SubjectPaths...)
	case OrbitMemberRule:
		return append([]string(nil), plan.RulePaths...)
	case OrbitMemberProcess:
		return append([]string(nil), plan.ProcessPaths...)
	default:
		return nil
	}
}

// ParseOrbitSpecData decodes one member-model orbit spec without switching the
// current Definition-based command surface to the new schema yet.
func ParseOrbitSpecData(data []byte, sourcePath string) (OrbitSpec, error) {
	return parseOrbitSpecData(data, sourcePath, true)
}

// ParseHostedOrbitSpecData decodes one hosted member-model orbit spec from .harness/orbits/.
func ParseHostedOrbitSpecData(data []byte, sourcePath string) (OrbitSpec, error) {
	return parseOrbitSpecDataWithPathBuilder(data, sourcePath, true, HostedDefinitionRelativePath)
}

func parseOrbitSpecData(data []byte, sourcePath string, validateSourcePath bool) (OrbitSpec, error) {
	return parseOrbitSpecDataWithPathBuilder(data, sourcePath, validateSourcePath, DefinitionRelativePath)
}

func parseOrbitSpecDataWithPathBuilder(
	data []byte,
	sourcePath string,
	validateSourcePath bool,
	pathBuilder func(string) (string, error),
) (OrbitSpec, error) {
	var spec OrbitSpec
	if err := contractutil.DecodeKnownFields(data, &spec); err != nil {
		return OrbitSpec{}, fmt.Errorf("decode orbit spec: %w", err)
	}

	spec.SourcePath = sourcePath
	spec, err := normalizeOrbitSpecBehaviorAlias(spec)
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("normalize orbit behavior: %w", err)
	}
	spec, err = normalizeOrbitSpecPackageIdentity(spec)
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("normalize orbit package identity: %w", err)
	}
	if err := validateOrbitSpecWithPathBuilder(spec, validateSourcePath, pathBuilder); err != nil {
		return OrbitSpec{}, fmt.Errorf("validate orbit spec: %w", err)
	}
	normalized, err := normalizeOrbitSpecMemberIdentities(spec)
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("normalize orbit spec: %w", err)
	}

	return normalized, nil
}

func normalizeOrbitSpecPackageIdentity(spec OrbitSpec) (OrbitSpec, error) {
	if len(spec.Content) > 0 && len(spec.Members) == 0 {
		spec.Members = append([]OrbitMember(nil), spec.Content...)
	}
	spec.Content = nil

	if spec.Package != nil {
		if err := ids.ValidatePackageIdentity(*spec.Package, ids.PackageTypeOrbit, "package"); err != nil {
			return OrbitSpec{}, fmt.Errorf("validate orbit spec package: %w", err)
		}
		spec.ID = spec.Package.Name
		return spec, nil
	}

	if spec.ID == "" {
		return OrbitSpec{}, fmt.Errorf("package must be present")
	}
	identity, err := ids.NewPackageIdentity(ids.PackageTypeOrbit, spec.ID, "")
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("derive orbit spec package: %w", err)
	}
	spec.Package = &identity

	return spec, nil
}

func normalizeOrbitSpecBehaviorAlias(spec OrbitSpec) (OrbitSpec, error) {
	if spec.Behavior != nil && spec.Rules != nil {
		return OrbitSpec{}, fmt.Errorf("rules and behavior cannot both be present")
	}
	if spec.Behavior == nil && spec.Rules != nil {
		spec.Behavior = spec.Rules
	}
	spec.Rules = nil

	return spec, nil
}

func normalizeOrbitSpecMemberIdentities(spec OrbitSpec) (OrbitSpec, error) {
	if !spec.HasMemberSchema() {
		return spec, nil
	}

	normalizedMembers := make([]OrbitMember, 0, len(spec.Members))
	for index, member := range spec.Members {
		normalized, err := normalizeOrbitMemberIdentity(member)
		if err != nil {
			return OrbitSpec{}, fmt.Errorf("members[%d].name: %w", index, err)
		}
		normalizedMembers = append(normalizedMembers, normalized)
	}

	spec.Members = normalizedMembers

	return spec, nil
}

func normalizeOrbitMemberIdentity(member OrbitMember) (OrbitMember, error) {
	name := strings.TrimSpace(member.Name)
	if err := ids.ValidateOrbitID(name); err == nil {
		member.Name = name
		member.Key = ""

		return member, nil
	}

	key := strings.TrimSpace(member.Key)
	if err := ids.ValidateOrbitID(key); err == nil {
		member.Name = key
		member.Key = ""

		return member, nil
	}

	if name == "" && key == "" {
		return OrbitMember{}, fmt.Errorf("must be present")
	}

	if name != "" {
		return OrbitMember{}, fmt.Errorf("validate orbit member name %q: %w", name, ids.ValidateOrbitID(name))
	}

	return OrbitMember{}, fmt.Errorf("validate orbit member name %q: %w", key, ids.ValidateOrbitID(key))
}

func orbitMemberIdentityName(member OrbitMember) string {
	if name := strings.TrimSpace(member.Name); name != "" {
		return name
	}

	return strings.TrimSpace(member.Key)
}

// HasMemberSchema reports whether the spec uses any member-model fields.
func (spec OrbitSpec) HasMemberSchema() bool {
	return spec.Meta != nil || spec.Capabilities != nil || spec.AgentAddons != nil || len(spec.Members) > 0 || len(spec.Content) > 0 || spec.Behavior != nil || spec.Rules != nil
}

// OrbitSpecFromDefinition lifts the current legacy definition shape into the
// member-model container without changing semantics.
//
//nolint:revive // Orbit member runtime docs freeze this exported domain name for cross-package consistency.
func OrbitSpecFromDefinition(definition Definition) OrbitSpec {
	return OrbitSpec{
		Package:     &ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: definition.ID},
		ID:          definition.ID,
		Description: definition.Description,
		Include:     append([]string(nil), definition.Include...),
		Exclude:     append([]string(nil), definition.Exclude...),
		SourcePath:  definition.SourcePath,
	}
}

// LegacyDefinition projects the member-model container back to the current
// Definition shape used by the existing command surface.
func (spec OrbitSpec) LegacyDefinition() Definition {
	return Definition{
		ID:          spec.ID,
		Description: spec.Description,
		Include:     append([]string(nil), spec.Include...),
		Exclude:     append([]string(nil), spec.Exclude...),
		SourcePath:  spec.SourcePath,
	}
}

func compatibilityDefinitionFromOrbitSpec(spec OrbitSpec) (Definition, error) {
	return compatibilityDefinitionFromOrbitSpecWithValidation(spec, true)
}

// CompatibilityDefinitionFromOrbitSpec projects one OrbitSpec onto the
// compatibility Definition shape used by runtime-facing command surfaces.
func CompatibilityDefinitionFromOrbitSpec(spec OrbitSpec) (Definition, error) {
	return compatibilityDefinitionFromOrbitSpecWithValidation(spec, true)
}

func compatibilityDefinitionFromOrbitSpecWithValidation(spec OrbitSpec, validateSourcePath bool) (Definition, error) {
	if !spec.HasMemberSchema() {
		definition := spec.LegacyDefinition()
		if definition.Exclude == nil {
			definition.Exclude = []string{}
		}
		if err := validateDefinition(definition, validateSourcePath); err != nil {
			return Definition{}, fmt.Errorf("validate compatibility orbit definition: %w", err)
		}

		return definition, nil
	}

	include := make([]string, 0, len(spec.Members))
	exclude := make([]string, 0, len(spec.Members))
	seenInclude := make(map[string]struct{})
	seenExclude := make(map[string]struct{})

	for _, member := range spec.Members {
		for _, pattern := range member.Paths.Include {
			if _, ok := seenInclude[pattern]; ok {
				continue
			}
			seenInclude[pattern] = struct{}{}
			include = append(include, pattern)
		}

		for _, pattern := range member.Paths.Exclude {
			if _, ok := seenExclude[pattern]; ok {
				continue
			}
			seenExclude[pattern] = struct{}{}
			exclude = append(exclude, pattern)
		}
	}

	definition := Definition{
		ID:           spec.ID,
		Description:  spec.Description,
		Include:      include,
		Exclude:      exclude,
		SourcePath:   spec.SourcePath,
		MemberSchema: true,
	}

	if len(definition.Include) == 0 {
		if err := ids.ValidateOrbitID(definition.ID); err != nil {
			return Definition{}, fmt.Errorf("validate compatibility orbit definition: %w", err)
		}
		if validateSourcePath && definition.SourcePath != "" {
			expectedName := definition.ID + ".yaml"
			if filepath.Base(definition.SourcePath) != expectedName {
				return Definition{}, fmt.Errorf("validate compatibility orbit definition: definition filename must match orbit id: expected %q", expectedName)
			}
		}
		return definition, nil
	}

	if err := validateDefinition(definition, validateSourcePath); err != nil {
		return Definition{}, fmt.Errorf("validate compatibility orbit definition: %w", err)
	}

	return definition, nil
}
