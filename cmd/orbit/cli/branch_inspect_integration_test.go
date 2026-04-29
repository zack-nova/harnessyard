package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBranchInspectMatchesGoldenOutputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		args       []string
		goldenFile string
		seed       func(t *testing.T, repo *testutil.Repo)
	}{
		{
			name:       "source text",
			args:       []string{"branch", "inspect", "HEAD"},
			goldenFile: "source_branch.txt",
			seed:       seedSourceBranchForInspect,
		},
		{
			name:       "source json",
			args:       []string{"branch", "inspect", "HEAD", "--json"},
			goldenFile: "source_branch.json",
			seed:       seedSourceBranchForInspect,
		},
		{
			name:       "orbit template text",
			args:       []string{"branch", "inspect", "template-source"},
			goldenFile: "orbit_template.txt",
			seed:       seedOrbitTemplateBranchForInspect,
		},
		{
			name:       "orbit template json",
			args:       []string{"branch", "inspect", "template-source", "--json"},
			goldenFile: "orbit_template.json",
			seed:       seedOrbitTemplateBranchForInspect,
		},
		{
			name:       "harness template text",
			args:       []string{"branch", "inspect", "harness-template"},
			goldenFile: "harness_template.txt",
			seed:       seedHarnessTemplateBranchForInspect,
		},
		{
			name:       "harness template json",
			args:       []string{"branch", "inspect", "harness-template", "--json"},
			goldenFile: "harness_template.json",
			seed:       seedHarnessTemplateBranchForInspect,
		},
		{
			name:       "runtime text",
			args:       []string{"branch", "inspect", "HEAD"},
			goldenFile: "runtime_with_installs.txt",
			seed:       seedRuntimeBranchForInspect,
		},
		{
			name:       "runtime json",
			args:       []string{"branch", "inspect", "HEAD", "--json"},
			goldenFile: "runtime_with_installs.json",
			seed:       seedRuntimeBranchForInspect,
		},
		{
			name:       "zero-member runtime text",
			args:       []string{"branch", "inspect", "HEAD"},
			goldenFile: "runtime_zero_member.txt",
			seed:       seedZeroMemberRuntimeBranchForInspect,
		},
		{
			name:       "zero-member runtime json",
			args:       []string{"branch", "inspect", "HEAD", "--json"},
			goldenFile: "runtime_zero_member.json",
			seed:       seedZeroMemberRuntimeBranchForInspect,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repo := testutil.NewRepo(t)
			testCase.seed(t, repo)

			stdout, stderr, err := executeCLI(t, repo.Root, testCase.args...)
			require.NoError(t, err)
			require.Empty(t, stderr)
			require.Equal(t, loadBranchInspectGolden(t, testCase.goldenFile), stdout)
		})
	}
}

func TestBranchInspectReportsPlainBranchWithoutFailing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "plain branch\n")
	repo.AddAndCommit(t, "seed plain branch")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "inspect", "HEAD")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "kind: plain\nreason: no valid .harness/manifest.yaml found\n", stdout)
}

func loadBranchInspectGolden(t *testing.T, filename string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "branch_inspect", filename))
	require.NoError(t, err)

	return string(data)
}

func seedOrbitTemplateBranchForInspect(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
		"id: api\n"+
		"include:\n"+
		"  - api/**\n")
	repo.WriteFile(t, "api/spec.md", "legacy stray\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "template content\n")
	repo.AddAndCommit(t, "seed template branch")
	repo.Run(t, "branch", "-M", "template-source")
}

func seedSourceBranchForInspect(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
		"id: api\n"+
		"include:\n"+
		"  - api/**\n")
	repo.WriteFile(t, "api/spec.md", "legacy stray\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "source docs\n")
	repo.AddAndCommit(t, "seed source branch")
	repo.Run(t, "branch", "-M", "source-main")
}

func seedHarnessTemplateBranchForInspect(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: deadbeef\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: docs\n"+
		"  - orbit_id: cmd\n"+
		"root_guidance:\n"+
		"  agents: true\n"+
		"  humans: false\n"+
		"  bootstrap: false\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/cmd.yaml", ""+
		"id: cmd\n"+
		"description: Cmd orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/cmd.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: cmd-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - cmd/**\n")
	repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
		"id: api\n"+
		"include:\n"+
		"  - api/**\n")
	repo.WriteFile(t, "api/spec.md", "legacy stray\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/cmd.yaml", ""+
		"id: cmd\n"+
		"include:\n"+
		"  - cmd/**\n")
	repo.WriteFile(t, "docs/guide.md", "template docs\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n")
	repo.AddAndCommit(t, "seed harness template branch")
	repo.Run(t, "branch", "-M", "harness-template")
}

func seedRuntimeBranchForInspect(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: docs\n"+
		"    source: install_orbit\n"+
		"    added_at: 2026-03-25T10:00:00Z\n"+
		"  - orbit_id: cmd\n"+
		"    source: manual\n"+
		"    added_at: 2026-03-25T10:05:00Z\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/cmd.yaml", ""+
		"id: cmd\n"+
		"include:\n"+
		"  - cmd/**\n")
	repo.WriteFile(t, ".harness/installs/docs.yaml", ""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_repo: \"\"\n"+
		"  source_ref: orbit-template/docs\n"+
		"  template_commit: deadbeef\n"+
		"applied_at: 2026-03-21T12:00:00Z\n")
	repo.WriteFile(t, "docs/guide.md", "runtime docs\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n")
	repo.AddAndCommit(t, "seed runtime branch")
}

func seedZeroMemberRuntimeBranchForInspect(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.AddAndCommit(t, "seed zero-member runtime branch")
}
