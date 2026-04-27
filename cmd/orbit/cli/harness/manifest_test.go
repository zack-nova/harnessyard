package harness

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestWriteAndLoadRuntimeManifestFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	createdAt := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.April, 5, 10, 30, 0, 0, time.UTC)
	input := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindRuntime,
		Runtime: &ManifestRuntimeMetadata{
			Package:   testHarnessPackage("project_a"),
			ID:        "project_a",
			Name:      "Project A",
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Members: []ManifestMember{
			{
				Package: testOrbitPackage("docs"),
				OrbitID: "docs",
				Source:  ManifestMemberSourceInstallOrbit,
				AddedAt: time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC),
			},
			{
				Package: testOrbitPackage("cli"),
				OrbitID: "cli",
				Source:  ManifestMemberSourceManual,
				AddedAt: time.Date(2026, time.April, 5, 10, 5, 0, 0, time.UTC),
			},
			{
				Package:        testOrbitPackage("workspace"),
				OrbitID:        "workspace",
				Source:         ManifestMemberSourceInstallBundle,
				IncludedIn:     testIncludedIn("project_a"),
				OwnerHarnessID: "project_a",
				AddedAt:        time.Date(2026, time.April, 5, 10, 15, 0, 0, time.UTC),
			},
		},
	}

	filename, err := WriteManifestFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, ManifestPath(repoRoot), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"    package:\n"+
		"        type: harness\n"+
		"        name: project_a\n"+
		"    name: Project A\n"+
		"    created_at: 2026-04-05T10:00:00Z\n"+
		"    updated_at: 2026-04-05T10:30:00Z\n"+
		"packages:\n"+
		"    - package:\n"+
		"        type: orbit\n"+
		"        name: cli\n"+
		"      source: manual\n"+
		"      added_at: 2026-04-05T10:05:00Z\n"+
		"    - package:\n"+
		"        type: orbit\n"+
		"        name: docs\n"+
		"      source: install_orbit\n"+
		"      added_at: 2026-04-05T10:10:00Z\n"+
		"    - package:\n"+
		"        type: orbit\n"+
		"        name: workspace\n"+
		"      source: install_bundle\n"+
		"      included_in:\n"+
		"        type: harness\n"+
		"        name: project_a\n"+
		"      added_at: 2026-04-05T10:15:00Z\n", string(data))

	expected := input
	expected.Members = []ManifestMember{
		{
			Package: testOrbitPackage("cli"),
			OrbitID: "cli",
			Source:  ManifestMemberSourceManual,
			AddedAt: time.Date(2026, time.April, 5, 10, 5, 0, 0, time.UTC),
		},
		{
			Package: testOrbitPackage("docs"),
			OrbitID: "docs",
			Source:  ManifestMemberSourceInstallOrbit,
			AddedAt: time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC),
		},
		{
			Package:        testOrbitPackage("workspace"),
			OrbitID:        "workspace",
			Source:         ManifestMemberSourceInstallBundle,
			IncludedIn:     testIncludedIn("project_a"),
			OwnerHarnessID: "project_a",
			AddedAt:        time.Date(2026, time.April, 5, 10, 15, 0, 0, time.UTC),
		},
	}

	loaded, err := LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, expected, loaded)
}

func TestWriteAndLoadRuntimeManifestFileRoundTripPreservesAffiliationMetadata(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindRuntime,
		Runtime: &ManifestRuntimeMetadata{
			Package:   testHarnessPackage("project_a"),
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.April, 22, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 22, 10, 30, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{
				Package:        testOrbitPackage("docs"),
				OrbitID:        "docs",
				Source:         ManifestMemberSourceInstallBundle,
				IncludedIn:     testIncludedIn("writing_stack"),
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 10, 10, 0, 0, time.UTC),
				LastStandaloneOrigin: &orbittemplate.Source{
					SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
					SourceRepo:     "",
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
			},
		},
	}

	filename, err := WriteManifestFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, ManifestPath(repoRoot), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"    package:\n"+
		"        type: harness\n"+
		"        name: project_a\n"+
		"    created_at: 2026-04-22T10:00:00Z\n"+
		"    updated_at: 2026-04-22T10:30:00Z\n"+
		"packages:\n"+
		"    - package:\n"+
		"        type: orbit\n"+
		"        name: docs\n"+
		"      source: install_bundle\n"+
		"      included_in:\n"+
		"        type: harness\n"+
		"        name: writing_stack\n"+
		"      added_at: 2026-04-22T10:10:00Z\n"+
		"      last_standalone_origin:\n"+
		"        source_kind: local_branch\n"+
		"        source_repo: \"\"\n"+
		"        source_ref: orbit-template/docs\n"+
		"        template_commit: abc123\n", string(data))

	loaded, err := LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestWriteAndLoadOrbitTemplateManifestFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindOrbitTemplate,
		Template: &ManifestTemplateMetadata{
			Package:           testOrbitPackage("docs"),
			OrbitID:           "docs",
			DefaultTemplate:   true,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 5, 11, 0, 0, 0, time.UTC),
		},
		Variables: map[string]ManifestVariableSpec{
			"project_name": {
				Description: "Product title",
				Required:    true,
			},
		},
	}

	filename, err := WriteManifestFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, ManifestPath(repoRoot), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"    package:\n"+
		"        type: orbit\n"+
		"        name: docs\n"+
		"    default_template: true\n"+
		"    created_from_branch: main\n"+
		"    created_from_commit: abc123\n"+
		"    created_at: 2026-04-05T11:00:00Z\n"+
		"variables:\n"+
		"    project_name:\n"+
		"        description: Product title\n"+
		"        required: true\n", string(data))

	loaded, err := LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestWriteAndLoadHarnessTemplateManifestFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := ManifestFile{
		SchemaVersion:      1,
		Kind:               ManifestKindHarnessTemplate,
		IncludesRootAgents: true,
		Template: &ManifestTemplateMetadata{
			Package:           testHarnessPackage("workspace"),
			HarnessID:         "workspace",
			DefaultTemplate:   true,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{Package: testOrbitPackage("docs"), OrbitID: "docs"},
			{Package: testOrbitPackage("cmd"), OrbitID: "cmd"},
		},
	}

	filename, err := WriteManifestFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, ManifestPath(repoRoot), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"    package:\n"+
		"        type: harness\n"+
		"        name: workspace\n"+
		"    default_template: true\n"+
		"    created_from_branch: main\n"+
		"    created_from_commit: abc123\n"+
		"    created_at: 2026-04-05T12:00:00Z\n"+
		"packages:\n"+
		"    - package:\n"+
		"        type: orbit\n"+
		"        name: cmd\n"+
		"    - package:\n"+
		"        type: orbit\n"+
		"        name: docs\n"+
		"includes_root_agents: true\n", string(data))

	loaded, err := LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, ManifestFile{
		SchemaVersion:      1,
		Kind:               ManifestKindHarnessTemplate,
		IncludesRootAgents: true,
		Template: &ManifestTemplateMetadata{
			Package:           testHarnessPackage("workspace"),
			HarnessID:         "workspace",
			DefaultTemplate:   true,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{Package: testOrbitPackage("cmd"), OrbitID: "cmd"},
			{Package: testOrbitPackage("docs"), OrbitID: "docs"},
		},
	}, loaded)
}

func TestWriteAndLoadSourceManifestFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindSource,
		Source: &ManifestSourceMetadata{
			Package:      testOrbitPackage("docs"),
			OrbitID:      "docs",
			SourceBranch: "main",
		},
	}

	filename, err := WriteManifestFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, ManifestPath(repoRoot), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"    package:\n"+
		"        type: orbit\n"+
		"        name: docs\n"+
		"    source_branch: main\n", string(data))

	loaded, err := LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestLoadHarnessTemplateManifestFileDefaultsIncludesRootAgentsToFalse(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := ManifestPath(repoRoot)

	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  package:\n"+
		"    type: harness\n"+
		"    name: project_a\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-05T12:00:00Z\n"+
		"packages:\n"+
		"  - package:\n"+
		"      type: orbit\n"+
		"      name: docs\n"), 0o600))

	loaded, err := LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindHarnessTemplate,
		Template: &ManifestTemplateMetadata{
			Package:           testHarnessPackage("project_a"),
			HarnessID:         "project_a",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{Package: testOrbitPackage("docs"), OrbitID: "docs"},
		},
		IncludesRootAgents: false,
	}, loaded)
}

func TestValidateRuntimeManifestFileRejectsMixedFieldsAndInvalidMembers(t *testing.T) {
	t.Parallel()

	input := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindRuntime,
		Runtime: &ManifestRuntimeMetadata{
			Package:   testHarnessPackage("project_a"),
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 5, 10, 30, 0, 0, time.UTC),
		},
		Template: &ManifestTemplateMetadata{
			Package:           testOrbitPackage("docs"),
			OrbitID:           "docs",
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 5, 11, 0, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{
				Package: testOrbitPackage("docs"),
				OrbitID: "docs",
				Source:  ManifestMemberSourceManual,
				AddedAt: time.Date(2026, time.April, 5, 10, 5, 0, 0, time.UTC),
			},
			{
				Package: testOrbitPackage("docs"),
				OrbitID: "docs",
				Source:  "bundle",
				AddedAt: time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC),
			},
		},
	}

	err := ValidateRuntimeManifestFile(input)
	require.Error(t, err)
	require.ErrorContains(t, err, "template must not be present")
}

func TestValidateRuntimeManifestFileRejectsInvalidAffiliationCombinations(t *testing.T) {
	t.Parallel()

	base := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindRuntime,
		Runtime: &ManifestRuntimeMetadata{
			Package:   testHarnessPackage("project_a"),
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.April, 22, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 22, 10, 30, 0, 0, time.UTC),
		},
	}

	tests := []struct {
		name     string
		member   ManifestMember
		contains string
	}{
		{
			name: "bundle member requires owner harness id",
			member: ManifestMember{
				OrbitID: "docs",
				Source:  ManifestMemberSourceInstallBundle,
				AddedAt: time.Date(2026, time.April, 22, 10, 10, 0, 0, time.UTC),
			},
			contains: `packages[0].included_in must be present when source is "install_bundle"`,
		},
		{
			name: "last standalone origin validates source kind",
			member: ManifestMember{
				Package:        testOrbitPackage("docs"),
				OrbitID:        "docs",
				Source:         ManifestMemberSourceInstallBundle,
				IncludedIn:     testIncludedIn("writing_stack"),
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 10, 10, 0, 0, time.UTC),
				LastStandaloneOrigin: &orbittemplate.Source{
					SourceKind:     "unknown",
					SourceRepo:     "",
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
			},
			contains: `packages[0].last_standalone_origin.source_kind must be one of "local_branch" or "external_git"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := base
			input.Members = []ManifestMember{tt.member}

			err := ValidateRuntimeManifestFile(input)
			require.Error(t, err)
			require.ErrorContains(t, err, tt.contains)
		})
	}
}

func TestValidateRuntimeManifestFileAllowsAssignedStandaloneSources(t *testing.T) {
	t.Parallel()

	base := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindRuntime,
		Runtime: &ManifestRuntimeMetadata{
			Package:   testHarnessPackage("project_a"),
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.April, 22, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 22, 10, 30, 0, 0, time.UTC),
		},
	}

	tests := []struct {
		name   string
		member ManifestMember
	}{
		{
			name: "manual member may declare owner harness id",
			member: ManifestMember{
				Package:        testOrbitPackage("docs"),
				OrbitID:        "docs",
				Source:         ManifestMemberSourceManual,
				IncludedIn:     testIncludedIn("writing_stack"),
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 10, 10, 0, 0, time.UTC),
			},
		},
		{
			name: "install orbit member may declare owner harness id",
			member: ManifestMember{
				Package:        testOrbitPackage("api"),
				OrbitID:        "api",
				Source:         ManifestMemberSourceInstallOrbit,
				IncludedIn:     testIncludedIn("writing_stack"),
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 10, 20, 0, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := base
			input.Members = []ManifestMember{tt.member}

			err := ValidateRuntimeManifestFile(input)
			require.NoError(t, err)
		})
	}
}

func TestValidateOrbitTemplateManifestFileRejectsHarnessOnlyFields(t *testing.T) {
	t.Parallel()

	input := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindOrbitTemplate,
		Template: &ManifestTemplateMetadata{
			Package:           testOrbitPackage("docs"),
			OrbitID:           "docs",
			HarnessID:         "project_a",
			DefaultTemplate:   true,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 5, 11, 0, 0, 0, time.UTC),
		},
	}

	err := ValidateOrbitTemplateManifestFile(input)
	require.Error(t, err)
	require.ErrorContains(t, err, "template.harness_id must not be present")
}

func TestValidateHarnessTemplateManifestFileRejectsInvalidMembers(t *testing.T) {
	t.Parallel()

	input := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindHarnessTemplate,
		Template: &ManifestTemplateMetadata{
			Package:           testHarnessPackage("project_a"),
			HarnessID:         "project_a",
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{Package: testOrbitPackage("docs"), OrbitID: "docs"},
			{
				Package: testOrbitPackage("docs"),
				OrbitID: "docs",
				Source:  ManifestMemberSourceManual,
			},
		},
	}

	err := ValidateHarnessTemplateManifestFile(input)
	require.Error(t, err)
	require.ErrorContains(t, err, "packages[1].package.name must be unique")
}

func TestValidateHarnessTemplateManifestFileAllowsEmptyMembers(t *testing.T) {
	t.Parallel()

	input := ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindHarnessTemplate,
		Template: &ManifestTemplateMetadata{
			Package:           testHarnessPackage("project_a"),
			HarnessID:         "project_a",
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC),
		},
		Members: []ManifestMember{},
	}

	err := ValidateHarnessTemplateManifestFile(input)
	require.NoError(t, err)
}

func TestParseManifestFileDataRejectsInvalidKindsAndMixedBranchFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		data        string
		messagePart string
	}{
		{
			name: "unsupported kind",
			data: "" +
				"schema_version: 1\n" +
				"kind: unsupported\n",
			messagePart: "kind must be one of",
		},
		{
			name: "source requires source metadata",
			data: "" +
				"schema_version: 1\n" +
				"kind: source\n",
			messagePart: "source must be present",
		},
		{
			name: "source must not carry runtime fields",
			data: "" +
				"schema_version: 1\n" +
				"kind: source\n" +
				"source:\n" +
				"  package:\n" +
				"    type: orbit\n" +
				"    name: docs\n" +
				"  source_branch: main\n" +
				"runtime:\n" +
				"  package:\n" +
				"    type: harness\n" +
				"    name: project_a\n" +
				"  created_at: 2026-04-05T10:00:00Z\n" +
				"  updated_at: 2026-04-05T10:30:00Z\n",
			messagePart: "runtime must not be present",
		},
		{
			name: "runtime mixed with template fields",
			data: "" +
				"schema_version: 1\n" +
				"kind: runtime\n" +
				"runtime:\n" +
				"  package:\n" +
				"    type: harness\n" +
				"    name: project_a\n" +
				"  created_at: 2026-04-05T10:00:00Z\n" +
				"  updated_at: 2026-04-05T10:30:00Z\n" +
				"template:\n" +
				"  package:\n" +
				"    type: orbit\n" +
				"    name: docs\n" +
				"  created_from_branch: main\n" +
				"  created_from_commit: abc123\n" +
				"  created_at: 2026-04-05T11:00:00Z\n" +
				"packages: []\n",
			messagePart: "template must not be present",
		},
		{
			name: "orbit template must not declare members",
			data: "" +
				"schema_version: 1\n" +
				"kind: orbit_template\n" +
				"template:\n" +
				"  package:\n" +
				"    type: orbit\n" +
				"    name: docs\n" +
				"  default_template: false\n" +
				"  created_from_branch: main\n" +
				"  created_from_commit: abc123\n" +
				"  created_at: 2026-04-05T11:00:00Z\n" +
				"packages: []\n",
			messagePart: "packages must not be present",
		},
		{
			name: "harness template member must not carry runtime fields",
			data: "" +
				"schema_version: 1\n" +
				"kind: harness_template\n" +
				"template:\n" +
				"  package:\n" +
				"    type: harness\n" +
				"    name: project_a\n" +
				"  created_from_branch: main\n" +
				"  created_from_commit: abc123\n" +
				"  created_at: 2026-04-05T12:00:00Z\n" +
				"packages:\n" +
				"  - package:\n" +
				"      type: orbit\n" +
				"      name: docs\n" +
				"    source: manual\n",
			messagePart: "packages[0].source must not be present",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseManifestFileData([]byte(tc.data))
			require.Error(t, err)
			require.ErrorContains(t, err, tc.messagePart)
		})
	}
}

func TestParseManifestFileDataAllowsZeroMemberHarnessTemplate(t *testing.T) {
	t.Parallel()

	manifest, err := ParseManifestFileData([]byte("" +
		"schema_version: 1\n" +
		"kind: harness_template\n" +
		"template:\n" +
		"  package:\n" +
		"    type: harness\n" +
		"    name: project_a\n" +
		"  created_from_branch: main\n" +
		"  created_from_commit: abc123\n" +
		"  created_at: 2026-04-05T12:00:00Z\n" +
		"packages: []\n"))
	require.NoError(t, err)
	require.Empty(t, manifest.Members)
}
