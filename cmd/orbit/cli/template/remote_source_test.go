package orbittemplate

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestEnumerateRemoteTemplateSourcesFiltersToValidTemplateBranches(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, "README.md", "plain branch\n")
	source.AddAndCommit(t, "seed plain branch")
	source.Run(t, "branch", "-M", "looks-template-but-plain")

	source.Run(t, "checkout", "-b", "looks-runtime-but-template")
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n")
	source.AddAndCommit(t, "add valid template manifest")

	source.Run(t, "checkout", "looks-template-but-plain")
	source.Run(t, "checkout", "-b", "looks-plain-but-template")
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: api\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: def456\n"+
		"  created_at: 2026-03-21T11:00:00Z\n")
	source.AddAndCommit(t, "add default template manifest")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Equal(t, []RemoteTemplateCandidate{
		{
			RepoURL: remoteURL,
			Branch:  "looks-plain-but-template",
			Ref:     "refs/heads/looks-plain-but-template",
			Commit:  source.RevParse(t, "looks-plain-but-template"),
			Manifest: Manifest{
				SchemaVersion: 1,
				Kind:          TemplateKind,
				Template: Metadata{
					OrbitID:           "api",
					DefaultTemplate:   true,
					CreatedFromBranch: "main",
					CreatedFromCommit: "def456",
					CreatedAt:         time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
				},
				Variables: map[string]VariableSpec{},
			},
		},
		{
			RepoURL: remoteURL,
			Branch:  "looks-runtime-but-template",
			Ref:     "refs/heads/looks-runtime-but-template",
			Commit:  source.RevParse(t, "looks-runtime-but-template"),
			Manifest: Manifest{
				SchemaVersion: 1,
				Kind:          TemplateKind,
				Template: Metadata{
					OrbitID:           "docs",
					DefaultTemplate:   false,
					CreatedFromBranch: "main",
					CreatedFromCommit: "abc123",
					CreatedAt:         time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
				},
				Variables: map[string]VariableSpec{},
			},
		},
	}, candidates)
}

func TestEnumerateRemoteTemplateSourcesIgnoresInvalidLegacyTemplateManifestWhenBranchManifestIsValid(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: bad999\n"+
		"  created_at: 2026-03-21T12:00:00Z\n")
	source.WriteFile(t, ".orbit/template.yaml", "schema_version: nope\n")
	source.AddAndCommit(t, "seed branch manifest with invalid legacy manifest")
	source.Run(t, "branch", "-M", "broken-template")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Equal(t, []RemoteTemplateCandidate{
		{
			RepoURL: remoteURL,
			Branch:  "broken-template",
			Ref:     "refs/heads/broken-template",
			Commit:  source.RevParse(t, "broken-template"),
			Manifest: Manifest{
				SchemaVersion: 1,
				Kind:          TemplateKind,
				Template: Metadata{
					OrbitID:           "docs",
					DefaultTemplate:   false,
					CreatedFromBranch: "main",
					CreatedFromCommit: "bad999",
					CreatedAt:         time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC),
				},
				Variables: map[string]VariableSpec{},
			},
		},
	}, candidates)
}

func TestEnumerateRemoteTemplateSourcesIgnoresLegacySharedFilesWhenBranchManifestIsValid(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: bad999\n"+
		"  created_at: 2026-03-21T12:00:00Z\n")
	source.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: bad999\n"+
		"  created_at: 2026-03-21T12:00:00Z\n"+
		"variables: {}\n"+
		"shared_files:\n"+
		"  - path: AGENTS.md\n"+
		"    kind: agents_fragment\n"+
		"    merge_mode: replace-block\n"+
		"    include_unmarked_content: true\n")
	source.AddAndCommit(t, "seed branch manifest with legacy shared files")
	source.Run(t, "branch", "-M", "shared-files-template")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Equal(t, []RemoteTemplateCandidate{
		{
			RepoURL: remoteURL,
			Branch:  "shared-files-template",
			Ref:     "refs/heads/shared-files-template",
			Commit:  source.RevParse(t, "shared-files-template"),
			Manifest: Manifest{
				SchemaVersion: 1,
				Kind:          TemplateKind,
				Template: Metadata{
					OrbitID:           "docs",
					DefaultTemplate:   false,
					CreatedFromBranch: "main",
					CreatedFromCommit: "bad999",
					CreatedAt:         time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC),
				},
				Variables: map[string]VariableSpec{},
			},
		},
	}, candidates)
}

func TestEnumerateRemoteTemplateSourcesCleansTemporaryRefs(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n")
	source.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables: {}\n")
	source.AddAndCommit(t, "seed template branch")
	source.Run(t, "branch", "-M", "remote-template")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	_, err := EnumerateRemoteTemplateSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Empty(t, runtimeRepo.Run(t, "for-each-ref", "--format=%(refname)", "refs/orbits/tmp/remote-source"))
}

func TestEnumerateRemoteTemplateSourcesRejectsBranchesWithoutOrbitTemplateBranchManifest(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables: {}\n")
	source.AddAndCommit(t, "seed legacy-only template branch")
	source.Run(t, "branch", "-M", "legacy-template")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestEnumerateRemoteTemplateSourcesAcceptsBranchManifestOnlyTemplatePayload(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    description: Product title\n"+
		"    required: true\n")
	source.AddAndCommit(t, "seed branch-manifest-only template branch")
	source.Run(t, "branch", "-M", "remote-template")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Equal(t, []RemoteTemplateCandidate{
		{
			RepoURL: remoteURL,
			Branch:  "remote-template",
			Ref:     "refs/heads/remote-template",
			Commit:  source.RevParse(t, "remote-template"),
			Manifest: Manifest{
				SchemaVersion: 1,
				Kind:          TemplateKind,
				Template: Metadata{
					OrbitID:           "docs",
					DefaultTemplate:   false,
					CreatedFromBranch: "main",
					CreatedFromCommit: "abc123",
					CreatedAt:         time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
				},
				Variables: map[string]VariableSpec{
					"project_name": {
						Description: "Product title",
						Required:    true,
					},
				},
			},
		},
	}, candidates)
}

func TestEnumerateRemoteTemplateSourcesPrefersBranchManifestDefaultTemplate(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n")
	source.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables: {}\n")
	source.AddAndCommit(t, "seed template branch with branch-manifest default override")
	source.Run(t, "branch", "-M", "remote-template")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.False(t, candidates[0].Manifest.Template.DefaultTemplate)
}
