package orbittemplate

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestSelectRemoteTemplateCandidatePrefersExplicitBranchRef(t *testing.T) {
	t.Parallel()

	selected, err := selectRemoteTemplateCandidate("file:///remote.git", "beta", []RemoteTemplateCandidate{
		testRemoteCandidate("alpha", false, "docs"),
		testRemoteCandidate("beta", true, "api"),
	})
	require.NoError(t, err)
	require.Equal(t, "beta", selected.Branch)
	require.Equal(t, "api", selected.Manifest.Template.OrbitID)
}

func TestSelectRemoteTemplateCandidateAcceptsFullHeadRef(t *testing.T) {
	t.Parallel()

	selected, err := selectRemoteTemplateCandidate("file:///remote.git", "refs/heads/alpha", []RemoteTemplateCandidate{
		testRemoteCandidate("alpha", false, "docs"),
		testRemoteCandidate("beta", true, "api"),
	})
	require.NoError(t, err)
	require.Equal(t, "alpha", selected.Branch)
}

func TestSelectRemoteTemplateCandidateReturnsOnlyValidCandidate(t *testing.T) {
	t.Parallel()

	selected, err := selectRemoteTemplateCandidate("file:///remote.git", "", []RemoteTemplateCandidate{
		testRemoteCandidate("only-template", false, "docs"),
	})
	require.NoError(t, err)
	require.Equal(t, "only-template", selected.Branch)
}

func TestSelectRemoteTemplateCandidateReturnsUniqueDefault(t *testing.T) {
	t.Parallel()

	selected, err := selectRemoteTemplateCandidate("file:///remote.git", "", []RemoteTemplateCandidate{
		testRemoteCandidate("alpha", false, "docs"),
		testRemoteCandidate("beta", true, "api"),
		testRemoteCandidate("gamma", false, "web"),
	})
	require.NoError(t, err)
	require.Equal(t, "beta", selected.Branch)
}

func TestSelectRemoteTemplateCandidateFailsWhenMultipleCandidatesHaveNoUniqueDefault(t *testing.T) {
	t.Parallel()

	_, err := selectRemoteTemplateCandidate("file:///remote.git", "", []RemoteTemplateCandidate{
		testRemoteCandidate("alpha", false, "docs"),
		testRemoteCandidate("beta", false, "api"),
	})
	require.Error(t, err)

	var ambiguityErr *RemoteTemplateAmbiguityError
	require.ErrorAs(t, err, &ambiguityErr)
	require.Equal(t, []string{"alpha", "beta"}, ambiguityErr.BranchNames())
}

func TestSelectRemoteTemplateCandidateFailsWhenMultipleDefaultsExist(t *testing.T) {
	t.Parallel()

	_, err := selectRemoteTemplateCandidate("file:///remote.git", "", []RemoteTemplateCandidate{
		testRemoteCandidate("alpha", true, "docs"),
		testRemoteCandidate("beta", true, "api"),
	})
	require.Error(t, err)

	var ambiguityErr *RemoteTemplateAmbiguityError
	require.ErrorAs(t, err, &ambiguityErr)
	require.Equal(t, []string{"alpha", "beta"}, ambiguityErr.BranchNames())
}

func TestSelectRemoteTemplateCandidateFailsWhenExplicitRefIsNotTemplateCandidate(t *testing.T) {
	t.Parallel()

	_, err := selectRemoteTemplateCandidate("file:///remote.git", "missing", []RemoteTemplateCandidate{
		testRemoteCandidate("alpha", true, "docs"),
	})
	require.Error(t, err)

	var notFoundErr *RemoteTemplateNotFoundError
	require.ErrorAs(t, err, &notFoundErr)
	require.Equal(t, "missing", notFoundErr.RequestedRef)
}

func TestSelectRemoteTemplateCandidateFailsWhenNoValidTemplatesExist(t *testing.T) {
	t.Parallel()

	_, err := selectRemoteTemplateCandidate("file:///remote.git", "", nil)
	require.Error(t, err)

	var notFoundErr *RemoteTemplateNotFoundError
	require.ErrorAs(t, err, &notFoundErr)
	require.Empty(t, notFoundErr.RequestedRef)
}

func TestResolveRemoteTemplateSourceUsesEnumerationAndUniqueDefaultSelection(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: aaa111\n"+
		"  created_at: 2026-03-21T10:00:00Z\n")
	source.AddAndCommit(t, "seed non-default template")
	source.Run(t, "branch", "-M", "alpha")

	source.Run(t, "checkout", "-b", "beta")
	source.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: api\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: bbb222\n"+
		"  created_at: 2026-03-21T11:00:00Z\n")
	source.AddAndCommit(t, "seed default template")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	selected, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, "")
	require.NoError(t, err)
	require.Equal(t, "beta", selected.Branch)
	require.True(t, selected.Manifest.Template.DefaultTemplate)
}

func TestResolveRemoteTemplateSourceExplicitFullHeadRefUsesShortBranchName(t *testing.T) {
	t.Parallel()

	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	selected, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, "refs/heads/"+sourceRef)
	require.NoError(t, err)
	require.Equal(t, sourceRef, selected.Branch)
	require.Equal(t, "refs/heads/"+sourceRef, selected.Ref)
	require.Equal(t, "docs", selected.Manifest.Template.OrbitID)
}

func TestResolveRemoteTemplateSourceExplicitRefFailsClosedOnNonTemplateBranch(t *testing.T) {
	t.Parallel()

	sourceRepo, _ := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)
	nonTemplateBranch := strings.TrimSpace(sourceRepo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))

	_, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, nonTemplateBranch)
	require.Error(t, err)

	var notFoundErr *RemoteTemplateNotFoundError
	require.ErrorAs(t, err, &notFoundErr)
	require.Equal(t, nonTemplateBranch, notFoundErr.RequestedRef)
}

func TestResolveRemoteTemplateSourceExplicitSourceRefAliasesPublishedBranch(t *testing.T) {
	t.Parallel()

	source := seedRemoteSourceRepo(t, &remoteSourceRepoOptions{
		PublishOrbitID:  "docs",
		PublishTemplate: true,
	})

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	selected, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, "main")
	require.NoError(t, err)
	require.Equal(t, "orbit-template/docs", selected.Branch)
	require.Equal(t, "refs/heads/orbit-template/docs", selected.Ref)
	require.Equal(t, "main", selected.RequestedRef)
	require.Equal(t, RemoteTemplateResolutionSourceAlias, selected.ResolutionKind)
	require.Equal(t, "docs", selected.Manifest.Template.OrbitID)
}

func TestResolveRemoteTemplateSourceExplicitSourceRefFailsWithoutPublishOrbitID(t *testing.T) {
	t.Parallel()

	source := seedRemoteSourceRepo(t, &remoteSourceRepoOptions{
		PublishOrbitID:  "",
		PublishTemplate: false,
	})

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	_, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, "main")
	require.Error(t, err)

	var notFoundErr *RemoteTemplateNotFoundError
	require.ErrorAs(t, err, &notFoundErr)
	require.Equal(t, "main", notFoundErr.RequestedRef)
	require.True(t, notFoundErr.SourceBranch)
	require.Equal(t, RemoteTemplateNotFoundReasonSourceAliasMissingPublishOrbitID, notFoundErr.Reason)
	require.ErrorContains(t, err, "source.orbit_id")
}

func TestResolveRemoteTemplateSourceExplicitSourceRefFailsWhenPublishedTemplateBranchIsMissing(t *testing.T) {
	t.Parallel()

	source := seedRemoteSourceRepo(t, &remoteSourceRepoOptions{
		PublishOrbitID:  "docs",
		PublishTemplate: false,
	})

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	_, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, "main")
	require.Error(t, err)

	var notFoundErr *RemoteTemplateNotFoundError
	require.ErrorAs(t, err, &notFoundErr)
	require.Equal(t, "main", notFoundErr.RequestedRef)
	require.True(t, notFoundErr.SourceBranch)
	require.Equal(t, "orbit-template/docs", notFoundErr.ResolvedRef)
	require.Equal(t, RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateMissing, notFoundErr.Reason)
	require.ErrorContains(t, err, "orbit template publish")
}

func TestResolveRemoteTemplateSourceNoRefPrefersDefaultSourceBranchAlias(t *testing.T) {
	t.Parallel()

	source := seedRemoteSourceRepo(t, &remoteSourceRepoOptions{
		PublishOrbitID:  "docs",
		PublishTemplate: true,
	})

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	selected, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, "")
	require.NoError(t, err)
	require.Equal(t, "orbit-template/docs", selected.Branch)
	require.Equal(t, "refs/heads/orbit-template/docs", selected.Ref)
	require.Equal(t, "main", selected.RequestedRef)
	require.Equal(t, RemoteTemplateResolutionSourceAlias, selected.ResolutionKind)
	require.Equal(t, "docs", selected.Manifest.Template.OrbitID)
}

func TestResolveRemoteTemplateSourceNoRefExplainsMissingPublishedTemplateForDefaultSourceBranch(t *testing.T) {
	t.Parallel()

	source := seedRemoteSourceRepo(t, &remoteSourceRepoOptions{
		PublishOrbitID:  "docs",
		PublishTemplate: false,
	})

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	_, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, "")
	require.Error(t, err)

	var notFoundErr *RemoteTemplateNotFoundError
	require.ErrorAs(t, err, &notFoundErr)
	require.Equal(t, "main", notFoundErr.RequestedRef)
	require.True(t, notFoundErr.SourceBranch)
	require.Equal(t, "orbit-template/docs", notFoundErr.ResolvedRef)
	require.Equal(t, RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateMissing, notFoundErr.Reason)
	require.ErrorContains(t, err, "orbit template publish")
}

func testRemoteCandidate(branch string, defaultTemplate bool, orbitID string) RemoteTemplateCandidate {
	return RemoteTemplateCandidate{
		RepoURL:        "file:///remote.git",
		Branch:         branch,
		Ref:            "refs/heads/" + branch,
		Commit:         branch + "-commit",
		ResolutionKind: RemoteTemplateResolutionTemplateBranch,
		Manifest: Manifest{
			SchemaVersion: 1,
			Kind:          TemplateKind,
			Template: Metadata{
				OrbitID:           orbitID,
				DefaultTemplate:   defaultTemplate,
				CreatedFromBranch: "main",
				CreatedFromCommit: branch + "-origin",
				CreatedAt:         time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
			},
			Variables: map[string]VariableSpec{},
		},
	}
}

type remoteSourceRepoOptions struct {
	PublishOrbitID  string
	PublishTemplate bool
}

func seedRemoteSourceRepo(t *testing.T, options *remoteSourceRepoOptions) *testutil.Repo {
	t.Helper()

	if options == nil {
		options = &remoteSourceRepoOptions{}
	}

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	orbitIDLine := ""
	if strings.TrimSpace(options.PublishOrbitID) != "" {
		orbitIDLine = fmt.Sprintf("  orbit_id: %s\n", options.PublishOrbitID)
	}
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		orbitIDLine+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "README.md", "author docs\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed source branch")

	if options.PublishTemplate {
		_, err := SaveTemplateBranch(context.Background(), TemplateSaveInput{
			Preview: TemplateSavePreviewInput{
				RepoRoot:      repo.Root,
				OrbitID:       "docs",
				TargetBranch:  "orbit-template/docs",
				DefaultBranch: true,
			},
			Overwrite: true,
		})
		require.NoError(t, err)
		repo.Run(t, "checkout", "main")
	}

	return repo
}
