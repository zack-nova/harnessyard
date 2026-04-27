package orbittemplate

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestResolveRemoteTemplateCandidateSnapshotLoadsManifestDefinitionAndUserFiles(t *testing.T) {
	t.Parallel()

	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	candidate, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, sourceRef)
	require.NoError(t, err)

	source, err := ResolveRemoteTemplateCandidateSnapshot(context.Background(), runtimeRepo.Root, candidate)
	require.NoError(t, err)
	require.Equal(t, sourceRef, source.Ref)
	require.Equal(t, strings.TrimSpace(sourceRepo.Run(t, "rev-parse", sourceRef)), source.Commit)
	require.Equal(t, "docs", source.Manifest.Template.OrbitID)
	require.Equal(t, "docs", source.Definition.ID)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, source.Files)
	require.Empty(t, runtimeRepo.Run(t, "for-each-ref", "--format=%(refname)", "refs/orbits/tmp/remote-source"))
}

func TestResolveRemoteTemplateCandidateSnapshotRejectsUnexpectedOrbitControlFiles(t *testing.T) {
	t.Parallel()

	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	sourceRepo.Run(t, "checkout", sourceRef)
	sourceRepo.WriteFile(t, ".orbit/orbits/extra.yaml", ""+
		"id: extra\n"+
		"description: Extra orbit\n"+
		"include:\n"+
		"  - extra/**\n")
	sourceRepo.AddAndCommit(t, "add unexpected template control file")

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	candidate, err := ResolveRemoteTemplateSource(context.Background(), runtimeRepo.Root, remoteURL, sourceRef)
	require.NoError(t, err)

	_, err = ResolveRemoteTemplateCandidateSnapshot(context.Background(), runtimeRepo.Root, candidate)
	require.Error(t, err)
	require.ErrorContains(t, err, "forbidden path .orbit/orbits/extra.yaml")
	require.Empty(t, runtimeRepo.Run(t, "for-each-ref", "--format=%(refname)", "refs/orbits/tmp/remote-source"))
}
