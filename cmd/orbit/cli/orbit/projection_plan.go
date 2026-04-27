package orbit

import (
	"fmt"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

type planScopeFlags struct {
	Projection    bool
	Write         bool
	Export        bool
	Orchestration bool
}

// ResolveProjectionPlan builds the role-aware projection plan for one orbit spec.
func ResolveProjectionPlan(config RepositoryConfig, spec OrbitSpec, trackedFiles []string) (ProjectionPlan, error) {
	if err := ValidateRepositoryConfig(config.Global, config.Orbits); err != nil {
		return ProjectionPlan{}, fmt.Errorf("validate repository config: %w", err)
	}
	if err := validateOrbitSpecForHost(spec); err != nil {
		return ProjectionPlan{}, fmt.Errorf("validate orbit spec: %w", err)
	}

	if !spec.HasMemberSchema() {
		return resolveLegacyProjectionPlan(config, spec.LegacyDefinition(), trackedFiles)
	}

	controlPaths, err := controlReadPaths(config)
	if err != nil {
		return ProjectionPlan{}, fmt.Errorf("resolve control read paths: %w", err)
	}

	companionPath, err := specControlPath(spec)
	if err != nil {
		return ProjectionPlan{}, fmt.Errorf("resolve companion path: %w", err)
	}

	sharedScopePatterns, err := normalizePatterns(config.Global.SharedScope)
	if err != nil {
		return ProjectionPlan{}, fmt.Errorf("normalize shared scope patterns: %w", err)
	}
	projectionVisiblePatterns, err := normalizePatterns(config.Global.ProjectionVisible)
	if err != nil {
		return ProjectionPlan{}, fmt.Errorf("normalize projection-visible patterns: %w", err)
	}

	roleDefaults := defaultProjectionPlanRoleScopes(spec)
	metaFlags := planScopeFlags{
		Projection:    spec.Meta.IncludeInProjection,
		Write:         spec.Meta.IncludeInWrite,
		Export:        spec.Meta.IncludeInExport,
		Orchestration: spec.Meta.IncludeDescriptionInOrchestration,
	}

	metaPaths := []string{companionPath}
	subjectPaths := []string{}
	rulePaths := []string{}
	processPaths := []string{}
	var capabilityPaths []string
	projectionPaths := []string{}
	orbitWritePaths := []string{}
	exportPaths := []string{}
	orchestrationPaths := []string{}

	if metaFlags.Projection {
		projectionPaths = append(projectionPaths, companionPath)
	}
	if metaFlags.Write {
		orbitWritePaths = append(orbitWritePaths, companionPath)
	}
	if metaFlags.Export {
		exportPaths = append(exportPaths, companionPath)
	}
	if metaFlags.Orchestration {
		orchestrationPaths = append(orchestrationPaths, companionPath)
	}

	for _, trackedFile := range trackedFiles {
		normalizedPath, err := ids.NormalizeRepoRelativePath(trackedFile)
		if err != nil {
			return ProjectionPlan{}, fmt.Errorf("normalize tracked file %q: %w", trackedFile, err)
		}
		if isControlPlanePath(normalizedPath) {
			continue
		}

		matchedMemberIndex := -1
		for index, member := range spec.Members {
			matches, matchErr := pathMatchesMember(member, normalizedPath)
			if matchErr != nil {
				return ProjectionPlan{}, fmt.Errorf("match member %q for %q: %w", orbitMemberIdentityName(member), normalizedPath, matchErr)
			}
			if !matches {
				continue
			}
			if matchedMemberIndex >= 0 {
				return ProjectionPlan{}, fmt.Errorf(
					"path %q matches multiple orbit members %q and %q",
					normalizedPath,
					orbitMemberIdentityName(spec.Members[matchedMemberIndex]),
					orbitMemberIdentityName(member),
				)
			}
			matchedMemberIndex = index
		}

		if matchedMemberIndex >= 0 {
			member := spec.Members[matchedMemberIndex]
			flags := roleDefaults.forRole(member.Role)
			flags.applyPatch(member.Scopes)

			subjectPaths, rulePaths, processPaths = appendRolePath(subjectPaths, rulePaths, processPaths, member.Role, normalizedPath)
			projectionPaths, orbitWritePaths, exportPaths, orchestrationPaths = appendPlanPaths(
				projectionPaths,
				orbitWritePaths,
				exportPaths,
				orchestrationPaths,
				flags,
				normalizedPath,
			)
			continue
		}

		inSharedScope, err := matchNormalizedAny(sharedScopePatterns, normalizedPath)
		if err != nil {
			return ProjectionPlan{}, fmt.Errorf("match shared scope patterns for %q: %w", normalizedPath, err)
		}
		if inSharedScope {
			flags := roleDefaults.forRole(OrbitMemberSubject)
			subjectPaths = append(subjectPaths, normalizedPath)
			projectionPaths, orbitWritePaths, exportPaths, orchestrationPaths = appendPlanPaths(
				projectionPaths,
				orbitWritePaths,
				exportPaths,
				orchestrationPaths,
				flags,
				normalizedPath,
			)
			continue
		}

		projectionVisible, err := matchNormalizedAny(projectionVisiblePatterns, normalizedPath)
		if err != nil {
			return ProjectionPlan{}, fmt.Errorf("match projection-visible patterns for %q: %w", normalizedPath, err)
		}
		if projectionVisible {
			flags := roleDefaults.forRole(OrbitMemberProcess)
			processPaths = append(processPaths, normalizedPath)
			projectionPaths, orbitWritePaths, exportPaths, orchestrationPaths = appendPlanPaths(
				projectionPaths,
				orbitWritePaths,
				exportPaths,
				orchestrationPaths,
				flags,
				normalizedPath,
			)
		}
	}

	capabilityPaths, err = resolveCapabilityOverlayPaths(spec, trackedFiles)
	if err != nil {
		return ProjectionPlan{}, fmt.Errorf("resolve capability overlay paths: %w", err)
	}
	projectionPaths = append(projectionPaths, capabilityPaths...)
	orbitWritePaths = append(orbitWritePaths, capabilityPaths...)
	exportPaths = append(exportPaths, capabilityPaths...)

	plan, err := finalizeProjectionPlan(ProjectionPlan{
		OrbitID:            spec.ID,
		ControlPaths:       mergeSortedUniquePaths(controlPaths),
		MetaPaths:          mergeSortedUniquePaths(metaPaths),
		SubjectPaths:       mergeSortedUniquePaths(subjectPaths),
		RulePaths:          mergeSortedUniquePaths(rulePaths),
		ProcessPaths:       mergeSortedUniquePaths(processPaths),
		CapabilityPaths:    mergeSortedUniquePaths(capabilityPaths),
		ProjectionPaths:    mergeSortedUniquePaths(projectionPaths),
		OrbitWritePaths:    mergeSortedUniquePaths(orbitWritePaths),
		ExportPaths:        mergeSortedUniquePaths(exportPaths),
		OrchestrationPaths: mergeSortedUniquePaths(orchestrationPaths),
	})
	if err != nil {
		return ProjectionPlan{}, err
	}
	if err := validateOrbitCapabilitiesAgainstExportSurface(spec, trackedFiles, plan.ExportPaths); err != nil {
		return ProjectionPlan{}, err
	}

	return plan, nil
}

func validateOrbitSpecForHost(spec OrbitSpec) error {
	controlPath, err := specControlPath(spec)
	if err != nil {
		return fmt.Errorf("resolve control path: %w", err)
	}
	if strings.HasPrefix(controlPath, hostedOrbitsRelativeDir+"/") {
		return ValidateHostedOrbitSpec(spec)
	}

	return ValidateOrbitSpec(spec)
}

func validateOrbitCapabilitiesAgainstExportSurface(
	spec OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
) error {
	if spec.Capabilities == nil {
		return nil
	}

	if err := validateCommandCapabilityExportSurface(spec.Capabilities.Commands, trackedFiles, exportPaths); err != nil {
		return err
	}
	if spec.Capabilities.Skills != nil {
		if err := validateLocalSkillCapabilityExportSurface(spec.Capabilities.Skills.Local, trackedFiles, exportPaths); err != nil {
			return err
		}
	}

	return nil
}

// ScopeSetFromProjectionPlan maps the role-aware plan back onto the current ScopeSet
// shape so existing command consumers can continue using the legacy path buckets.
func ScopeSetFromProjectionPlan(plan ProjectionPlan) ScopeSet {
	ownedPaths := mergeSortedUniquePaths(plan.SubjectPaths, plan.RulePaths)
	projectionOnlyPaths := append([]string(nil), plan.ProcessPaths...)
	companionPaths := append([]string(nil), plan.MetaPaths...)

	return ScopeSet{
		ControlReadPaths:     append([]string(nil), plan.ControlPaths...),
		OwnedPaths:           ownedPaths,
		ProjectionOnlyPaths:  projectionOnlyPaths,
		UserDataPaths:        ownedPaths,
		CompanionPaths:       companionPaths,
		ScopedOperationPaths: mergeSortedUniquePaths(companionPaths, ownedPaths),
		ProjectionPaths:      append([]string(nil), plan.ProjectionPaths...),
	}
}

// ResolveScopeSetForSpec builds the legacy ScopeSet view from an OrbitSpec via ProjectionPlan.
func ResolveScopeSetForSpec(config RepositoryConfig, spec OrbitSpec, trackedFiles []string) (ScopeSet, error) {
	plan, err := ResolveProjectionPlan(config, spec, trackedFiles)
	if err != nil {
		return ScopeSet{}, err
	}

	return ScopeSetFromProjectionPlan(plan), nil
}

func resolveLegacyProjectionPlan(config RepositoryConfig, definition Definition, trackedFiles []string) (ProjectionPlan, error) {
	ownedPaths, projectionOnlyPaths, err := resolveUserDataPaths(config.Global, definition, trackedFiles)
	if err != nil {
		return ProjectionPlan{}, err
	}

	controlPaths, err := controlReadPaths(config)
	if err != nil {
		return ProjectionPlan{}, fmt.Errorf("resolve control read paths: %w", err)
	}

	companionPath, err := definitionControlPath(definition)
	if err != nil {
		return ProjectionPlan{}, fmt.Errorf("resolve companion path: %w", err)
	}

	metaPaths := []string{companionPath}
	projectionPaths := mergeSortedUniquePaths(metaPaths, ownedPaths, projectionOnlyPaths)
	orbitWritePaths := mergeSortedUniquePaths(metaPaths, ownedPaths)

	return finalizeProjectionPlan(ProjectionPlan{
		OrbitID:            definition.ID,
		ControlPaths:       mergeSortedUniquePaths(controlPaths),
		MetaPaths:          mergeSortedUniquePaths(metaPaths),
		SubjectPaths:       mergeSortedUniquePaths(ownedPaths),
		RulePaths:          []string{},
		ProcessPaths:       mergeSortedUniquePaths(projectionOnlyPaths),
		CapabilityPaths:    []string{},
		ProjectionPaths:    projectionPaths,
		OrbitWritePaths:    orbitWritePaths,
		ExportPaths:        orbitWritePaths,
		OrchestrationPaths: mergeSortedUniquePaths(metaPaths),
	})
}

func pathMatchesMember(member OrbitMember, normalizedPath string) (bool, error) {
	includePatterns, err := normalizePatterns(member.Paths.Include)
	if err != nil {
		return false, fmt.Errorf("normalize include patterns: %w", err)
	}
	excludePatterns, err := normalizePatterns(member.Paths.Exclude)
	if err != nil {
		return false, fmt.Errorf("normalize exclude patterns: %w", err)
	}

	included, err := matchNormalizedAny(includePatterns, normalizedPath)
	if err != nil {
		return false, fmt.Errorf("match include patterns: %w", err)
	}
	if !included {
		return false, nil
	}

	excluded, err := matchNormalizedAny(excludePatterns, normalizedPath)
	if err != nil {
		return false, fmt.Errorf("match exclude patterns: %w", err)
	}

	return !excluded, nil
}

func appendRolePath(subjectPaths []string, rulePaths []string, processPaths []string, role OrbitMemberRole, path string) ([]string, []string, []string) {
	switch role {
	case OrbitMemberSubject:
		subjectPaths = append(subjectPaths, path)
	case OrbitMemberRule:
		rulePaths = append(rulePaths, path)
	case OrbitMemberProcess:
		processPaths = append(processPaths, path)
	}

	return subjectPaths, rulePaths, processPaths
}

func appendPlanPaths(
	projectionPaths []string,
	orbitWritePaths []string,
	exportPaths []string,
	orchestrationPaths []string,
	flags planScopeFlags,
	path string,
) ([]string, []string, []string, []string) {
	if flags.Projection {
		projectionPaths = append(projectionPaths, path)
	}
	if flags.Write {
		orbitWritePaths = append(orbitWritePaths, path)
	}
	if flags.Export {
		exportPaths = append(exportPaths, path)
	}
	if flags.Orchestration {
		orchestrationPaths = append(orchestrationPaths, path)
	}

	return projectionPaths, orbitWritePaths, exportPaths, orchestrationPaths
}

func (flags *planScopeFlags) applyPatch(patch *OrbitMemberScopePatch) {
	if patch == nil {
		return
	}
	if patch.Write != nil {
		flags.Write = *patch.Write
	}
	if patch.Export != nil {
		flags.Export = *patch.Export
	}
	if patch.Orchestration != nil {
		flags.Orchestration = *patch.Orchestration
	}
}

type roleScopeDefaults struct {
	meta    planScopeFlags
	subject planScopeFlags
	rule    planScopeFlags
	process planScopeFlags
}

func defaultProjectionPlanRoleScopes(spec OrbitSpec) roleScopeDefaults {
	defaults := roleScopeDefaults{
		meta: planScopeFlags{
			Projection:    true,
			Write:         true,
			Export:        true,
			Orchestration: true,
		},
		subject: planScopeFlags{
			Projection: true,
		},
		rule: planScopeFlags{
			Projection:    true,
			Write:         true,
			Export:        true,
			Orchestration: true,
		},
		process: planScopeFlags{
			Projection:    true,
			Orchestration: true,
		},
	}

	behavior := spec.Behavior
	if behavior == nil {
		behavior = spec.Rules
	}
	if behavior == nil {
		return defaults
	}
	if behavior.Scope.ProjectionRoles != nil {
		defaults.setProjectionRoles(behavior.Scope.ProjectionRoles)
	}
	if behavior.Scope.WriteRoles != nil {
		defaults.setWriteRoles(behavior.Scope.WriteRoles)
	}
	if behavior.Scope.ExportRoles != nil {
		defaults.setExportRoles(behavior.Scope.ExportRoles)
	}
	if behavior.Scope.OrchestrationRoles != nil {
		defaults.setOrchestrationRoles(behavior.Scope.OrchestrationRoles)
	}

	return defaults
}

func (defaults roleScopeDefaults) forRole(role OrbitMemberRole) planScopeFlags {
	switch role {
	case OrbitMemberMeta:
		return defaults.meta
	case OrbitMemberSubject:
		return defaults.subject
	case OrbitMemberRule:
		return defaults.rule
	case OrbitMemberProcess:
		return defaults.process
	default:
		return planScopeFlags{}
	}
}

func (defaults *roleScopeDefaults) setProjectionRoles(roles []OrbitMemberRole) {
	defaults.meta.Projection = false
	defaults.subject.Projection = false
	defaults.rule.Projection = false
	defaults.process.Projection = false
	for _, role := range roles {
		flags := defaults.flagsForRole(role)
		flags.Projection = true
	}
}

func (defaults *roleScopeDefaults) setWriteRoles(roles []OrbitMemberRole) {
	defaults.meta.Write = false
	defaults.subject.Write = false
	defaults.rule.Write = false
	defaults.process.Write = false
	for _, role := range roles {
		flags := defaults.flagsForRole(role)
		flags.Write = true
	}
}

func (defaults *roleScopeDefaults) setExportRoles(roles []OrbitMemberRole) {
	defaults.meta.Export = false
	defaults.subject.Export = false
	defaults.rule.Export = false
	defaults.process.Export = false
	for _, role := range roles {
		flags := defaults.flagsForRole(role)
		flags.Export = true
	}
}

func (defaults *roleScopeDefaults) setOrchestrationRoles(roles []OrbitMemberRole) {
	defaults.meta.Orchestration = false
	defaults.subject.Orchestration = false
	defaults.rule.Orchestration = false
	defaults.process.Orchestration = false
	for _, role := range roles {
		flags := defaults.flagsForRole(role)
		flags.Orchestration = true
	}
}

func (defaults *roleScopeDefaults) flagsForRole(role OrbitMemberRole) *planScopeFlags {
	switch role {
	case OrbitMemberMeta:
		return &defaults.meta
	case OrbitMemberSubject:
		return &defaults.subject
	case OrbitMemberRule:
		return &defaults.rule
	case OrbitMemberProcess:
		return &defaults.process
	default:
		return &planScopeFlags{}
	}
}
