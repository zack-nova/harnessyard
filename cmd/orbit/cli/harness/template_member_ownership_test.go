package harness

import (
	"testing"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestAnalyzeTemplateMemberOwnershipReturnsExclusiveAndSharedPaths(t *testing.T) {
	t.Parallel()

	source := LocalTemplateInstallSource{
		Manifest: TemplateManifest{
			Members: []TemplateMember{
				{OrbitID: "docs"},
				{OrbitID: "shared"},
			},
		},
		MemberSnapshots: map[string]TemplateMemberSnapshot{
			"docs": {
				SchemaVersion: 1,
				Kind:          TemplateMemberSnapshotKind,
				OrbitID:       "docs",
				MemberSource:  MemberSourceManual,
				Snapshot: TemplateMemberSnapshotData{
					ExportedPaths: []string{"docs/guide.md", "shared/checklist.md"},
					FileDigests: map[string]string{
						"docs/guide.md":       contentDigest([]byte("docs guide\n")),
						"shared/checklist.md": contentDigest([]byte("shared checklist\n")),
					},
					Variables: map[string]TemplateVariableSpec{},
				},
			},
			"shared": {
				SchemaVersion: 1,
				Kind:          TemplateMemberSnapshotKind,
				OrbitID:       "shared",
				MemberSource:  MemberSourceManual,
				Snapshot: TemplateMemberSnapshotData{
					ExportedPaths: []string{"shared/checklist.md"},
					FileDigests: map[string]string{
						"shared/checklist.md": contentDigest([]byte("shared checklist\n")),
					},
					Variables: map[string]TemplateVariableSpec{},
				},
			},
		},
		Files: []orbittemplate.CandidateFile{
			{Path: "docs/guide.md", Content: []byte("docs guide\n")},
			{Path: "shared/checklist.md", Content: []byte("shared checklist\n")},
		},
	}

	ownership, err := AnalyzeTemplateMemberOwnership(source, "docs")
	require.NoError(t, err)
	require.Equal(t, "docs", ownership.OrbitID)
	require.Equal(t, []string{"docs/guide.md"}, ownership.ExclusivePaths)
	require.Equal(t, []string{"shared/checklist.md"}, ownership.SharedPaths)
}

func TestAnalyzeTemplateMemberOwnershipRejectsMissingSnapshot(t *testing.T) {
	t.Parallel()

	source := LocalTemplateInstallSource{
		Manifest: TemplateManifest{
			Members: []TemplateMember{
				{OrbitID: "docs"},
				{OrbitID: "shared"},
			},
		},
		MemberSnapshots: map[string]TemplateMemberSnapshot{
			"docs": {
				SchemaVersion: 1,
				Kind:          TemplateMemberSnapshotKind,
				OrbitID:       "docs",
				MemberSource:  MemberSourceManual,
				Snapshot: TemplateMemberSnapshotData{
					ExportedPaths: []string{"docs/guide.md"},
					FileDigests: map[string]string{
						"docs/guide.md": contentDigest([]byte("docs guide\n")),
					},
					Variables: map[string]TemplateVariableSpec{},
				},
			},
		},
		Files: []orbittemplate.CandidateFile{
			{Path: "docs/guide.md", Content: []byte("docs guide\n")},
		},
	}

	_, err := AnalyzeTemplateMemberOwnership(source, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `template member snapshot for "shared" is required`)
}

func TestAnalyzeTemplateMemberOwnershipRejectsMissingPayloadPath(t *testing.T) {
	t.Parallel()

	source := LocalTemplateInstallSource{
		Manifest: TemplateManifest{
			Members: []TemplateMember{
				{OrbitID: "docs"},
			},
		},
		MemberSnapshots: map[string]TemplateMemberSnapshot{
			"docs": {
				SchemaVersion: 1,
				Kind:          TemplateMemberSnapshotKind,
				OrbitID:       "docs",
				MemberSource:  MemberSourceManual,
				Snapshot: TemplateMemberSnapshotData{
					ExportedPaths: []string{"docs/guide.md"},
					FileDigests: map[string]string{
						"docs/guide.md": contentDigest([]byte("docs guide\n")),
					},
					Variables: map[string]TemplateVariableSpec{},
				},
			},
		},
		Files: []orbittemplate.CandidateFile{},
	}

	_, err := AnalyzeTemplateMemberOwnership(source, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `snapshot path "docs/guide.md" is missing from template payload`)
}

func TestAnalyzeTemplateMemberOwnershipRejectsSharedDigestMismatch(t *testing.T) {
	t.Parallel()

	source := LocalTemplateInstallSource{
		Manifest: TemplateManifest{
			Members: []TemplateMember{
				{OrbitID: "docs"},
				{OrbitID: "shared"},
			},
		},
		MemberSnapshots: map[string]TemplateMemberSnapshot{
			"docs": {
				SchemaVersion: 1,
				Kind:          TemplateMemberSnapshotKind,
				OrbitID:       "docs",
				MemberSource:  MemberSourceManual,
				Snapshot: TemplateMemberSnapshotData{
					ExportedPaths: []string{"shared/checklist.md"},
					FileDigests: map[string]string{
						"shared/checklist.md": contentDigest([]byte("shared checklist\n")),
					},
					Variables: map[string]TemplateVariableSpec{},
				},
			},
			"shared": {
				SchemaVersion: 1,
				Kind:          TemplateMemberSnapshotKind,
				OrbitID:       "shared",
				MemberSource:  MemberSourceManual,
				Snapshot: TemplateMemberSnapshotData{
					ExportedPaths: []string{"shared/checklist.md"},
					FileDigests: map[string]string{
						"shared/checklist.md": contentDigest([]byte("different checklist\n")),
					},
					Variables: map[string]TemplateVariableSpec{},
				},
			},
		},
		Files: []orbittemplate.CandidateFile{
			{Path: "shared/checklist.md", Content: []byte("shared checklist\n")},
		},
	}

	_, err := AnalyzeTemplateMemberOwnership(source, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `shared snapshot path "shared/checklist.md" has inconsistent digests`)
}

func TestAnalyzeTemplateMemberOwnershipRejectsUnownedPayloadPath(t *testing.T) {
	t.Parallel()

	source := LocalTemplateInstallSource{
		Manifest: TemplateManifest{
			Members: []TemplateMember{
				{OrbitID: "docs"},
			},
		},
		MemberSnapshots: map[string]TemplateMemberSnapshot{
			"docs": {
				SchemaVersion: 1,
				Kind:          TemplateMemberSnapshotKind,
				OrbitID:       "docs",
				MemberSource:  MemberSourceManual,
				Snapshot: TemplateMemberSnapshotData{
					ExportedPaths: []string{"docs/guide.md"},
					FileDigests: map[string]string{
						"docs/guide.md": contentDigest([]byte("docs guide\n")),
					},
					Variables: map[string]TemplateVariableSpec{},
				},
			},
		},
		Files: []orbittemplate.CandidateFile{
			{Path: "docs/extra.md", Content: []byte("extra guide\n")},
			{Path: "docs/guide.md", Content: []byte("docs guide\n")},
		},
	}

	_, err := AnalyzeTemplateMemberOwnership(source, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `template payload paths are not owned by any member snapshot: docs/extra.md`)
}

func TestAnalyzeTemplateMemberOwnershipRejectsSnapshotVariableManifestDrift(t *testing.T) {
	t.Parallel()

	source := LocalTemplateInstallSource{
		Manifest: TemplateManifest{
			Members: []TemplateMember{
				{OrbitID: "docs"},
			},
			Variables: map[string]TemplateVariableSpec{
				"project_name": {Required: true},
			},
		},
		MemberSnapshots: map[string]TemplateMemberSnapshot{
			"docs": {
				SchemaVersion: 1,
				Kind:          TemplateMemberSnapshotKind,
				OrbitID:       "docs",
				MemberSource:  MemberSourceManual,
				Snapshot: TemplateMemberSnapshotData{
					ExportedPaths: []string{"docs/guide.md"},
					FileDigests: map[string]string{
						"docs/guide.md": contentDigest([]byte("$project_name guide\n")),
					},
					Variables: map[string]TemplateVariableSpec{
						"wrong_name": {Required: true},
					},
				},
			},
		},
		Files: []orbittemplate.CandidateFile{
			{Path: "docs/guide.md", Content: []byte("$project_name guide\n")},
		},
	}

	_, err := AnalyzeTemplateMemberOwnership(source, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `template member snapshot for "docs" has variable summary drift`)
}
