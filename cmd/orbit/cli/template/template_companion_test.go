package orbittemplate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestLoadTemplateCompanionAtRevisionPrefersHostedDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Hosted docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Hosted docs guidance\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Legacy docs orbit\n"+
		"include:\n"+
		"  - legacy/**\n")
	repo.AddAndCommit(t, "seed mixed companion hosts")

	path, spec, err := loadTemplateCompanionAtRevision(context.Background(), repo.Root, "HEAD", "docs")
	require.NoError(t, err)
	require.Equal(t, ".harness/orbits/docs.yaml", path)
	require.Equal(t, "Hosted docs orbit", spec.Description)
	require.Equal(t, "Hosted docs guidance\n", spec.Meta.AgentsTemplate)
}

func TestLoadTemplateCompanionAtRevisionFallsBackToLegacyDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Legacy docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.AddAndCommit(t, "seed legacy-only companion")

	path, spec, err := loadTemplateCompanionAtRevision(context.Background(), repo.Root, "HEAD", "docs")
	require.NoError(t, err)
	require.Equal(t, ".orbit/orbits/docs.yaml", path)
	require.Equal(t, "Legacy docs orbit", spec.Description)
}
