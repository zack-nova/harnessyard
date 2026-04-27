package orbit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestOrbitMemberRoleParsing(t *testing.T) {
	t.Parallel()

	expectedRoles := []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberSubject,
		orbitpkg.OrbitMemberRule,
		orbitpkg.OrbitMemberProcess,
	}

	require.Equal(t, expectedRoles, orbitpkg.AllOrbitMemberRoles())

	for _, role := range expectedRoles {
		require.True(t, role.IsValid())

		parsed, err := orbitpkg.ParseOrbitMemberRole(string(role))
		require.NoError(t, err)
		require.Equal(t, role, parsed)
	}

	require.False(t, orbitpkg.OrbitMemberRole("outside").IsValid())

	_, err := orbitpkg.ParseOrbitMemberRole("outside")
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid orbit member role")
}

func TestProjectionPlanPathsForRole(t *testing.T) {
	t.Parallel()

	plan := orbitpkg.ProjectionPlan{
		MetaPaths:    []string{".orbit/orbits/docs.yaml"},
		SubjectPaths: []string{"docs/guide.md"},
		RulePaths:    []string{".markdownlint.yaml"},
		ProcessPaths: []string{"docs/process/review.md"},
	}

	require.Equal(t, []string{".orbit/orbits/docs.yaml"}, plan.PathsForRole(orbitpkg.OrbitMemberMeta))
	require.Equal(t, []string{"docs/guide.md"}, plan.PathsForRole(orbitpkg.OrbitMemberSubject))
	require.Equal(t, []string{".markdownlint.yaml"}, plan.PathsForRole(orbitpkg.OrbitMemberRule))
	require.Equal(t, []string{"docs/process/review.md"}, plan.PathsForRole(orbitpkg.OrbitMemberProcess))
	require.Empty(t, plan.PathsForRole(orbitpkg.OrbitMemberRole("outside")))
}

func TestParseOrbitSpecDataRoundTripsMemberModelFields(t *testing.T) {
	t.Parallel()

	sourcePath := "/repo/.orbit/orbits/docs.yaml"
	data := []byte("" +
		"id: docs\n" +
		"name: Documentation\n" +
		"description: User-facing docs orbit.\n" +
		"meta:\n" +
		"  file: .orbit/orbits/docs.yaml\n" +
		"  agents_template: |\n" +
		"    You are the $project_name docs orbit.\n" +
		"    Keep release notes current.\n" +
		"  humans_template: |\n" +
		"    Run the docs workflow for $project_name.\n" +
		"    Record handoff notes before you stop.\n" +
		"  bootstrap_template: |\n" +
		"    Initialize the docs orbit before the first task.\n" +
		"    Create any required starter assets.\n" +
		"  include_in_projection: true\n" +
		"  include_in_write: true\n" +
		"  include_in_export: true\n" +
		"  include_description_in_orchestration: true\n" +
		"capabilities:\n" +
		"  commands:\n" +
		"    paths:\n" +
		"      include:\n" +
		"        - commands/docs/**/*.md\n" +
		"      exclude:\n" +
		"        - commands/docs/_drafts/**\n" +
		"  skills:\n" +
		"    local:\n" +
		"      paths:\n" +
		"        include:\n" +
		"          - skills/docs/*\n" +
		"        exclude:\n" +
		"          - skills/docs/_archive/*\n" +
		"    remote:\n" +
		"      uris:\n" +
		"        - github://acme/docs-style\n" +
		"        - https://example.com/skills/review\n" +
		"members:\n" +
		"  - key: docs-content\n" +
		"    name: Docs Content\n" +
		"    role: subject\n" +
		"    paths:\n" +
		"      include:\n" +
		"        - docs/**\n" +
		"        - README.md\n" +
		"      exclude:\n" +
		"        - docs/generated/**\n" +
		"  - key: docs-process\n" +
		"    description: Writer and agent workflow.\n" +
		"    role: process\n" +
		"    lane: bootstrap\n" +
		"    paths:\n" +
		"      include:\n" +
		"        - docs/process/**\n" +
		"    scopes:\n" +
		"      write: false\n" +
		"      orchestration: true\n" +
		"behavior:\n" +
		"  scope:\n" +
		"    projection_roles: [meta, subject, rule, process]\n" +
		"    write_roles: [meta, rule]\n" +
		"    export_roles: [meta, rule]\n" +
		"    orchestration_roles: [meta, rule, process]\n" +
		"  orchestration:\n" +
		"    include_orbit_description: true\n" +
		"    materialize_agents_from_meta: true\n")

	spec, err := orbitpkg.ParseOrbitSpecData(data, sourcePath)
	require.NoError(t, err)

	require.Equal(t, "docs", spec.ID)
	require.Equal(t, "Documentation", spec.Name)
	require.Equal(t, sourcePath, spec.SourcePath)
	require.NotNil(t, spec.Meta)
	require.Equal(t, ".orbit/orbits/docs.yaml", spec.Meta.File)
	require.Equal(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", spec.Meta.AgentsTemplate)
	require.Equal(t, "Run the docs workflow for $project_name.\nRecord handoff notes before you stop.\n", spec.Meta.HumansTemplate)
	require.Equal(t, "Initialize the docs orbit before the first task.\nCreate any required starter assets.\n", spec.Meta.BootstrapTemplate)
	require.NotNil(t, spec.Capabilities)
	require.NotNil(t, spec.Capabilities.Commands)
	require.Equal(t, orbitpkg.OrbitMemberPaths{
		Include: []string{"commands/docs/**/*.md"},
		Exclude: []string{"commands/docs/_drafts/**"},
	}, spec.Capabilities.Commands.Paths)
	require.NotNil(t, spec.Capabilities.Skills)
	require.NotNil(t, spec.Capabilities.Skills.Local)
	require.Equal(t, orbitpkg.OrbitMemberPaths{
		Include: []string{"skills/docs/*"},
		Exclude: []string{"skills/docs/_archive/*"},
	}, spec.Capabilities.Skills.Local.Paths)
	require.NotNil(t, spec.Capabilities.Skills.Remote)
	require.Equal(t, []string{
		"github://acme/docs-style",
		"https://example.com/skills/review",
	}, spec.Capabilities.Skills.Remote.URIs)
	require.Len(t, spec.Members, 2)
	require.Equal(t, "docs-content", spec.Members[0].Name)
	require.Empty(t, spec.Members[0].Key)
	require.Equal(t, orbitpkg.OrbitMemberSubject, spec.Members[0].Role)
	require.Equal(t, "docs-process", spec.Members[1].Name)
	require.Empty(t, spec.Members[1].Key)
	require.Equal(t, orbitpkg.OrbitMemberProcess, spec.Members[1].Role)
	require.Equal(t, "bootstrap", spec.Members[1].Lane)
	require.NotNil(t, spec.Members[1].Scopes)
	require.NotNil(t, spec.Behavior)
	require.True(t, spec.HasMemberSchema())

	marshaled, err := yaml.Marshal(spec)
	require.NoError(t, err)
	require.NotContains(t, string(marshaled), "key:")
	require.Contains(t, string(marshaled), "behavior:")
	require.NotContains(t, string(marshaled), "rules:")

	roundTripped, err := orbitpkg.ParseOrbitSpecData(marshaled, sourcePath)
	require.NoError(t, err)
	require.Equal(t, spec, roundTripped)
}

func TestParseOrbitSpecDataNormalizesLegacyRulesToBehavior(t *testing.T) {
	t.Parallel()

	spec, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"members:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"rules:\n"+
		"  scope:\n"+
		"    write_roles: [meta, rule, subject]\n"), "/repo/.orbit/orbits/docs.yaml")
	require.NoError(t, err)
	require.NotNil(t, spec.Behavior)

	marshaled, err := yaml.Marshal(spec)
	require.NoError(t, err)
	require.Contains(t, string(marshaled), "behavior:")
	require.NotContains(t, string(marshaled), "rules:")
}

func TestParseOrbitSpecDataRejectsRulesAndBehaviorTogether(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"members:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"rules:\n"+
		"  scope:\n"+
		"    write_roles: [meta, rule]\n"+
		"behavior:\n"+
		"  scope:\n"+
		"    write_roles: [meta, rule, subject]\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "rules and behavior cannot both be present")
}

func TestParseOrbitSpecDataRejectsUnsupportedRemoteSkillURI(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - commands/docs/**/*.md\n"+
		"  skills:\n"+
		"    remote:\n"+
		"      uris:\n"+
		"        - file:///tmp/docs-skill\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, `capabilities.skills.remote.uris[0]: unsupported remote skill URI scheme "file"`)
}

func TestParseHostedOrbitSpecDataSupportsRemoteSkillDependencies(t *testing.T) {
	t.Parallel()

	spec, err := orbitpkg.ParseHostedOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"capabilities:\n"+
		"  skills:\n"+
		"    remote:\n"+
		"      dependencies:\n"+
		"        - uri: github://acme/review-skill\n"+
		"        - uri: https://example.com/skills/release-gate\n"+
		"          required: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), "/repo/.harness/orbits/docs.yaml")
	require.NoError(t, err)
	require.NotNil(t, spec.Capabilities)
	require.NotNil(t, spec.Capabilities.Skills)
	require.NotNil(t, spec.Capabilities.Skills.Remote)
	require.Equal(t, []orbitpkg.OrbitRemoteSkillDependency{
		{URI: "github://acme/review-skill"},
		{URI: "https://example.com/skills/release-gate", Required: true},
	}, spec.Capabilities.Skills.Remote.Dependencies)
}

func TestParseHostedOrbitSpecDataRejectsMixedRemoteSkillShapes(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseHostedOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"capabilities:\n"+
		"  skills:\n"+
		"    remote:\n"+
		"      uris:\n"+
		"        - github://acme/legacy-skill\n"+
		"      dependencies:\n"+
		"        - uri: github://acme/review-skill\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), "/repo/.harness/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "capabilities.skills.remote must not define both uris and dependencies")
}

func TestParseHostedOrbitSpecDataRejectsCapabilityOwnedMemberOverlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		memberPath  string
		containsErr string
	}{
		{
			name:        "command path overlap",
			memberPath:  "commands/execute/**",
			containsErr: `members[0].paths.include[0] overlaps capability-owned commands path "commands/execute/**/*.md"`,
		},
		{
			name:        "local skill root overlap",
			memberPath:  "skills/execute/frontend-test-lab/**",
			containsErr: `members[0].paths.include[0] overlaps capability-owned local skills path "skills/execute/*"`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := orbitpkg.ParseHostedOrbitSpecData([]byte(""+
				"id: execute\n"+
				"meta:\n"+
				"  file: .harness/orbits/execute.yaml\n"+
				"  include_in_projection: true\n"+
				"  include_in_write: true\n"+
				"  include_in_export: true\n"+
				"  include_description_in_orchestration: true\n"+
				"capabilities:\n"+
				"  commands:\n"+
				"    paths:\n"+
				"      include:\n"+
				"        - commands/execute/**/*.md\n"+
				"  skills:\n"+
				"    local:\n"+
				"      paths:\n"+
				"        include:\n"+
				"          - skills/execute/*\n"+
				"members:\n"+
				"  - name: execute-assets\n"+
				"    role: rule\n"+
				"    paths:\n"+
				"      include:\n"+
				"        - "+test.memberPath+"\n"), "/repo/.harness/orbits/execute.yaml")
			require.Error(t, err)
			require.ErrorContains(t, err, test.containsErr)
		})
	}
}

func TestParseHostedOrbitSpecDataAcceptsNonCapabilityMemberPaths(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseHostedOrbitSpecData([]byte(""+
		"id: execute\n"+
		"meta:\n"+
		"  file: .harness/orbits/execute.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - commands/execute/**/*.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - skills/execute/*\n"+
		"members:\n"+
		"  - name: execute-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - execute/rules/**\n"), "/repo/.harness/orbits/execute.yaml")
	require.NoError(t, err)
}

func TestParseOrbitSpecDataRejectsUnsupportedMemberLane(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    lane: agents\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, `members[0].lane must be "bootstrap" when present`)
}

func TestParseHostedOrbitSpecDataAcceptsHarnessHostedMetaFile(t *testing.T) {
	t.Parallel()

	sourcePath := "/repo/.harness/orbits/docs.yaml"
	spec, err := orbitpkg.ParseHostedOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"members:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), sourcePath)
	require.NoError(t, err)
	require.Equal(t, sourcePath, spec.SourcePath)
	require.NotNil(t, spec.Meta)
	require.Equal(t, ".harness/orbits/docs.yaml", spec.Meta.File)
}

func TestParseOrbitSpecDataPrefersValidMemberNameOverLegacyKey(t *testing.T) {
	t.Parallel()

	spec, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-content-legacy\n"+
		"    name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), "/repo/.orbit/orbits/docs.yaml")
	require.NoError(t, err)
	require.Len(t, spec.Members, 1)
	require.Equal(t, "docs-content", spec.Members[0].Name)
	require.Empty(t, spec.Members[0].Key)
}

func TestParseHostedOrbitSpecDataRejectsLegacyMetaFile(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseHostedOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), "/repo/.harness/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "meta.file")
	require.ErrorContains(t, err, ".harness/orbits/docs.yaml")
}

func TestOrbitSpecLegacyDefinitionCompatibility(t *testing.T) {
	t.Parallel()

	definition := orbitpkg.Definition{
		ID:          "docs",
		Description: "User-facing docs orbit.",
		Include:     []string{"docs/**", "README.md"},
		Exclude:     []string{"docs/generated/**"},
		SourcePath:  "/repo/.orbit/orbits/docs.yaml",
	}

	spec := orbitpkg.OrbitSpecFromDefinition(definition)
	require.False(t, spec.HasMemberSchema())
	require.Equal(t, definition.ID, spec.ID)
	require.Equal(t, definition.Description, spec.Description)
	require.Equal(t, definition.Include, spec.Include)
	require.Equal(t, definition.Exclude, spec.Exclude)
	require.Equal(t, definition.SourcePath, spec.SourcePath)

	require.Equal(t, definition, spec.LegacyDefinition())
}

func TestParseOrbitSpecDataAcceptsLegacySchema(t *testing.T) {
	t.Parallel()

	sourcePath := "/repo/.orbit/orbits/docs.yaml"
	data := []byte("" +
		"id: docs\n" +
		"description: User-facing docs orbit.\n" +
		"include:\n" +
		"  - docs/**\n" +
		"exclude:\n" +
		"  - docs/generated/**\n")

	spec, err := orbitpkg.ParseOrbitSpecData(data, sourcePath)
	require.NoError(t, err)
	require.False(t, spec.HasMemberSchema())
	require.Equal(t, orbitpkg.Definition{
		ID:          "docs",
		Description: "User-facing docs orbit.",
		Include:     []string{"docs/**"},
		Exclude:     []string{"docs/generated/**"},
		SourcePath:  sourcePath,
	}, spec.LegacyDefinition())
}

func TestParseOrbitSpecDataRejectsInvalidMemberRole(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: outside\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "members[0].role")
	require.ErrorContains(t, err, "invalid orbit member role")
}

func TestParseOrbitSpecDataRejectsInvalidBehaviorScopeRole(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"behavior:\n"+
		"  scope:\n"+
		"    write_roles: [meta, outside]\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "behavior.scope.write_roles[1]")
	require.ErrorContains(t, err, "invalid orbit member role")
}

func TestParseOrbitSpecDataRejectsDuplicateMemberNames(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"  - name: docs-content\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .markdownlint.yaml\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "members[1].name must be unique")
}

func TestParseOrbitSpecDataRejectsMismatchedMetaFile(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/guides.yaml\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "meta.file")
	require.ErrorContains(t, err, ".orbit/orbits/docs.yaml")
}

func TestParseOrbitSpecDataRejectsMixedLegacyAndMemberSchema(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "legacy include/exclude")
}

func TestParseOrbitSpecDataRejectsInvalidScopePatchField(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseOrbitSpecData([]byte(""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n"+
		"    scopes:\n"+
		"      projection: false\n"), "/repo/.orbit/orbits/docs.yaml")
	require.Error(t, err)
	require.ErrorContains(t, err, "field projection not found")
}

func TestParseDefinitionDataKeepsLegacySchemaUnchanged(t *testing.T) {
	t.Parallel()

	sourcePath := "/repo/.orbit/orbits/docs.yaml"
	data := []byte("" +
		"id: docs\n" +
		"description: User-facing docs orbit.\n" +
		"include:\n" +
		"  - docs/**\n" +
		"  - README.md\n" +
		"exclude:\n" +
		"  - docs/generated/**\n")

	definition, err := orbitpkg.ParseDefinitionData(data, sourcePath)
	require.NoError(t, err)
	require.Equal(t, orbitpkg.Definition{
		ID:          "docs",
		Description: "User-facing docs orbit.",
		Include:     []string{"docs/**", "README.md"},
		Exclude:     []string{"docs/generated/**"},
		SourcePath:  sourcePath,
	}, definition)
}

func TestParseDefinitionDataDefaultsLegacyExcludeToEmptySlice(t *testing.T) {
	t.Parallel()

	sourcePath := "/repo/.orbit/orbits/docs.yaml"
	definition, err := orbitpkg.ParseDefinitionData([]byte(""+
		"id: docs\n"+
		"description: User-facing docs orbit.\n"+
		"include:\n"+
		"  - docs/**\n"), sourcePath)
	require.NoError(t, err)
	require.Equal(t, orbitpkg.Definition{
		ID:          "docs",
		Description: "User-facing docs orbit.",
		Include:     []string{"docs/**"},
		Exclude:     []string{},
		SourcePath:  sourcePath,
	}, definition)
}

func TestParseDefinitionDataAcceptsMemberSchemaCompatibilityProjection(t *testing.T) {
	t.Parallel()

	sourcePath := "/repo/.orbit/orbits/docs.yaml"
	definition, err := orbitpkg.ParseDefinitionData([]byte(""+
		"id: docs\n"+
		"description: User-facing docs orbit.\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"      exclude:\n"+
		"        - docs/generated/**\n"+
		"  - name: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .markdownlint.yaml\n"), sourcePath)
	require.NoError(t, err)
	require.Equal(t, orbitpkg.Definition{
		ID:           "docs",
		Description:  "User-facing docs orbit.",
		Include:      []string{"docs/**", ".markdownlint.yaml"},
		Exclude:      []string{"docs/generated/**"},
		SourcePath:   sourcePath,
		MemberSchema: true,
	}, definition)
}

func TestLoadRepositoryConfigAcceptsMemberSchemaDefinitions(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: User-facing docs orbit.\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")

	config, err := orbitpkg.LoadRepositoryConfig(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Len(t, config.Orbits, 1)
	require.Equal(t, orbitpkg.Definition{
		ID:           "docs",
		Description:  "User-facing docs orbit.",
		Include:      []string{"docs/**"},
		Exclude:      []string{},
		SourcePath:   repo.Root + "/.orbit/orbits/docs.yaml",
		MemberSchema: true,
	}, config.Orbits[0])
}
