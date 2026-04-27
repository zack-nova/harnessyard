package orbittemplate

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBuildLocalBindingsInitPreviewReturnsManifestBackedSkeleton(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)

	preview, err := BuildLocalBindingsInitPreview(context.Background(), LocalBindingsInitInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
	})
	require.NoError(t, err)
	require.Equal(t, Source{
		SourceKind:     InstallSourceKindLocalBranch,
		SourceRepo:     "",
		SourceRef:      sourceRef,
		TemplateCommit: strings.TrimSpace(repo.Run(t, "rev-parse", sourceRef)),
	}, preview.Source)
	require.Equal(t, "docs", preview.Manifest.Template.OrbitID)
	require.Equal(t, bindings.VarsFile{
		SchemaVersion: 1,
		Variables: map[string]bindings.VariableBinding{
			"project_name": {
				Value:       "",
				Description: "Product title",
			},
		},
	}, preview.Skeleton)
}

func TestBuildRemoteBindingsInitPreviewReturnsRemoteSkeleton(t *testing.T) {
	t.Parallel()

	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "plain repo\n")
	repo.AddAndCommit(t, "seed plain repo")

	preview, err := BuildRemoteBindingsInitPreview(context.Background(), RemoteBindingsInitInput{
		RepoRoot:  repo.Root,
		RemoteURL: remoteURL,
	})
	require.NoError(t, err)
	require.Equal(t, Source{
		SourceKind:     InstallSourceKindExternalGit,
		SourceRepo:     remoteURL,
		SourceRef:      sourceRef,
		TemplateCommit: strings.TrimSpace(sourceRepo.Run(t, "rev-parse", sourceRef)),
	}, preview.Source)
	require.Equal(t, bindings.VarsFile{
		SchemaVersion: 1,
		Variables: map[string]bindings.VariableBinding{
			"project_name": {
				Value:       "",
				Description: "Product title",
			},
		},
	}, preview.Skeleton)
}
