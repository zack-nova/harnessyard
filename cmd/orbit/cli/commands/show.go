package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type showOutput struct {
	RepoRoot string `json:"repo_root"`
	Orbit    any    `json:"orbit"`
}

type memberSchemaShowView struct {
	ID                   string                      `json:"id" yaml:"id"`
	Name                 string                      `json:"name,omitempty" yaml:"name,omitempty"`
	Description          string                      `json:"description,omitempty" yaml:"description,omitempty"`
	Schema               string                      `json:"schema" yaml:"schema"`
	SourcePath           string                      `json:"source_path" yaml:"source_path"`
	Meta                 *orbitpkg.OrbitMeta         `json:"meta,omitempty" yaml:"meta,omitempty"`
	Capabilities         *orbitpkg.OrbitCapabilities `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	ResolvedCapabilities *resolvedCapabilityOutput   `json:"resolved_capabilities,omitempty" yaml:"resolved_capabilities,omitempty"`
	Members              []memberSchemaShowMember    `json:"members,omitempty" yaml:"members,omitempty"`
	BehaviorDefaults     roleScopeShowView           `json:"behavior_defaults" yaml:"behavior_defaults"`
}

type memberSchemaShowJSONView struct {
	memberSchemaShowView
	RoleScopes roleScopeShowView `json:"role_scopes"`
}

type memberSchemaShowMember struct {
	Name            string                   `json:"name" yaml:"name"`
	Description     string                   `json:"description,omitempty" yaml:"description,omitempty"`
	Role            orbitpkg.OrbitMemberRole `json:"role" yaml:"role"`
	Lane            string                   `json:"lane,omitempty" yaml:"lane,omitempty"`
	Paths           showMemberPaths          `json:"paths" yaml:"paths"`
	Scopes          *showMemberScopes        `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	EffectiveScopes scopeFlags               `json:"effective_scopes" yaml:"effective_scopes"`
}

type showMemberPaths struct {
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

type showMemberScopes struct {
	Write         *bool `json:"write,omitempty" yaml:"write,omitempty"`
	Export        *bool `json:"export,omitempty" yaml:"export,omitempty"`
	Orchestration *bool `json:"orchestration,omitempty" yaml:"orchestration,omitempty"`
}

type roleScopeShowView struct {
	Meta    scopeFlags `json:"meta" yaml:"meta"`
	Subject scopeFlags `json:"subject" yaml:"subject"`
	Rule    scopeFlags `json:"rule" yaml:"rule"`
	Process scopeFlags `json:"process" yaml:"process"`
}

type scopeFlags struct {
	Projection    bool `json:"projection" yaml:"projection"`
	Write         bool `json:"write" yaml:"write"`
	Export        bool `json:"export" yaml:"export"`
	Orchestration bool `json:"orchestration" yaml:"orchestration"`
}

// NewShowCommand creates the orbit show command.
func NewShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [orbit-id]",
		Short: "Show an orbit definition",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}
			requestedOrbitID := ""
			if len(args) > 0 {
				requestedOrbitID = args[0]
			}
			orbitID, err := resolveAuthoredTruthOrbitID(cmd, repo, requestedOrbitID)
			if err != nil {
				return err
			}

			config, err := loadValidatedVisibleOrbitConfig(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}

			definition, err := definitionByID(config, orbitID)
			if err != nil {
				return err
			}

			spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repo.Root, definition.ID)
			if err != nil {
				return fmt.Errorf("load orbit spec: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			var orbitOutput any
			if spec.HasMemberSchema() {
				if jsonOutput {
					var resolved *resolvedCapabilityOutput
					if hasAuthoredCapabilities(spec) {
						resolvedOutput, err := resolveCapabilitiesForDisplay(cmd.Context(), repo.Root, config, spec)
						if err != nil {
							return err
						}
						resolved = &resolvedOutput
					}
					orbitOutput = buildMemberSchemaShowJSONView(spec, resolved)
				} else {
					orbitOutput = buildMemberSchemaShowView(spec, nil)
				}
			} else {
				orbitOutput = definition
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), showOutput{
					RepoRoot: repo.Root,
					Orbit:    orbitOutput,
				})
			}

			data, err := yaml.Marshal(orbitOutput)
			if err != nil {
				return fmt.Errorf("marshal orbit definition: %w", err)
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), string(data)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}

func buildMemberSchemaShowView(spec orbitpkg.OrbitSpec, resolved *resolvedCapabilityOutput) memberSchemaShowView {
	roleScopes := effectiveRoleScopes(spec)
	members := make([]memberSchemaShowMember, 0, len(spec.Members))

	for _, member := range spec.Members {
		effectiveScopes := roleScopes.forRole(member.Role)
		if member.Scopes != nil {
			if member.Scopes.Write != nil {
				effectiveScopes.Write = *member.Scopes.Write
			}
			if member.Scopes.Export != nil {
				effectiveScopes.Export = *member.Scopes.Export
			}
			if member.Scopes.Orchestration != nil {
				effectiveScopes.Orchestration = *member.Scopes.Orchestration
			}
		}

		members = append(members, memberSchemaShowMember{
			Name:        member.Name,
			Description: member.Description,
			Role:        member.Role,
			Lane:        member.Lane,
			Paths: showMemberPaths{
				Include: append([]string(nil), member.Paths.Include...),
				Exclude: append([]string(nil), member.Paths.Exclude...),
			},
			Scopes:          cloneShowMemberScopes(member.Scopes),
			EffectiveScopes: effectiveScopes,
		})
	}

	return memberSchemaShowView{
		ID:                   spec.ID,
		Name:                 spec.Name,
		Description:          spec.Description,
		Schema:               "members",
		SourcePath:           spec.SourcePath,
		Meta:                 spec.Meta,
		Capabilities:         spec.Capabilities,
		ResolvedCapabilities: resolved,
		Members:              members,
		BehaviorDefaults:     roleScopes,
	}
}

func buildMemberSchemaShowJSONView(spec orbitpkg.OrbitSpec, resolved *resolvedCapabilityOutput) memberSchemaShowJSONView {
	view := buildMemberSchemaShowView(spec, resolved)
	return memberSchemaShowJSONView{
		memberSchemaShowView: view,
		RoleScopes:           view.BehaviorDefaults,
	}
}

func effectiveRoleScopes(spec orbitpkg.OrbitSpec) roleScopeShowView {
	scopes := roleScopeShowView{
		Meta: scopeFlags{
			Projection:    true,
			Write:         true,
			Export:        true,
			Orchestration: true,
		},
		Subject: scopeFlags{
			Projection: true,
		},
		Rule: scopeFlags{
			Projection:    true,
			Write:         true,
			Export:        true,
			Orchestration: true,
		},
		Process: scopeFlags{
			Projection:    true,
			Orchestration: true,
		},
	}

	behavior := spec.Behavior
	if behavior == nil {
		behavior = spec.Rules
	}
	if behavior == nil {
		return scopes
	}

	if behavior.Scope.ProjectionRoles != nil {
		scopes.setProjectionRoles(behavior.Scope.ProjectionRoles)
	}
	if behavior.Scope.WriteRoles != nil {
		scopes.setWriteRoles(behavior.Scope.WriteRoles)
	}
	if behavior.Scope.ExportRoles != nil {
		scopes.setExportRoles(behavior.Scope.ExportRoles)
	}
	if behavior.Scope.OrchestrationRoles != nil {
		scopes.setOrchestrationRoles(behavior.Scope.OrchestrationRoles)
	}

	return scopes
}

func cloneShowMemberScopes(scopes *orbitpkg.OrbitMemberScopePatch) *showMemberScopes {
	if scopes == nil {
		return nil
	}

	return &showMemberScopes{
		Write:         scopes.Write,
		Export:        scopes.Export,
		Orchestration: scopes.Orchestration,
	}
}

func (view *roleScopeShowView) forRole(role orbitpkg.OrbitMemberRole) scopeFlags {
	switch role {
	case orbitpkg.OrbitMemberMeta:
		return view.Meta
	case orbitpkg.OrbitMemberSubject:
		return view.Subject
	case orbitpkg.OrbitMemberRule:
		return view.Rule
	case orbitpkg.OrbitMemberProcess:
		return view.Process
	default:
		return scopeFlags{}
	}
}

func (view *roleScopeShowView) resetProjection() {
	view.Meta.Projection = false
	view.Subject.Projection = false
	view.Rule.Projection = false
	view.Process.Projection = false
}

func (view *roleScopeShowView) resetWrite() {
	view.Meta.Write = false
	view.Subject.Write = false
	view.Rule.Write = false
	view.Process.Write = false
}

func (view *roleScopeShowView) resetExport() {
	view.Meta.Export = false
	view.Subject.Export = false
	view.Rule.Export = false
	view.Process.Export = false
}

func (view *roleScopeShowView) resetOrchestration() {
	view.Meta.Orchestration = false
	view.Subject.Orchestration = false
	view.Rule.Orchestration = false
	view.Process.Orchestration = false
}

func (view *roleScopeShowView) setProjectionRoles(roles []orbitpkg.OrbitMemberRole) {
	view.resetProjection()
	for _, role := range roles {
		flags := view.flagsForRole(role)
		flags.Projection = true
	}
}

func (view *roleScopeShowView) setWriteRoles(roles []orbitpkg.OrbitMemberRole) {
	view.resetWrite()
	for _, role := range roles {
		flags := view.flagsForRole(role)
		flags.Write = true
	}
}

func (view *roleScopeShowView) setExportRoles(roles []orbitpkg.OrbitMemberRole) {
	view.resetExport()
	for _, role := range roles {
		flags := view.flagsForRole(role)
		flags.Export = true
	}
}

func (view *roleScopeShowView) setOrchestrationRoles(roles []orbitpkg.OrbitMemberRole) {
	view.resetOrchestration()
	for _, role := range roles {
		flags := view.flagsForRole(role)
		flags.Orchestration = true
	}
}

func (view *roleScopeShowView) flagsForRole(role orbitpkg.OrbitMemberRole) *scopeFlags {
	switch role {
	case orbitpkg.OrbitMemberMeta:
		return &view.Meta
	case orbitpkg.OrbitMemberSubject:
		return &view.Subject
	case orbitpkg.OrbitMemberRule:
		return &view.Rule
	case orbitpkg.OrbitMemberProcess:
		return &view.Process
	default:
		return &scopeFlags{}
	}
}
