package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardRegistryEntryOrbitWritesStdoutCandidate(t *testing.T) {
	lockHyardProcessEnv(t)

	fixture := seedHyardRegistryEntryOrbitFixture(t)

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"orbit",
		"acme/docs@0.1.0",
		"--source",
		fixture.SourceRemote,
		"--ref",
		"orbit-template/docs",
		"--package",
		"docs",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var candidate struct {
		SchemaVersion   int    `yaml:"schema_version"`
		TargetPath      string `yaml:"target_path"`
		Submittable     bool   `yaml:"submittable"`
		PackageType     string `yaml:"package_type"`
		PackageIdentity string `yaml:"package_identity"`
		PackageHandle   string `yaml:"package_handle"`
		Version         string `yaml:"version"`
		PackageStatus   string `yaml:"package_status"`
		Source          struct {
			Remote string `yaml:"remote"`
			Ref    string `yaml:"ref"`
			Commit string `yaml:"commit"`
		} `yaml:"source"`
		Validation struct {
			SourceRemoteReachable struct {
				OK bool `yaml:"ok"`
			} `yaml:"source_remote_reachable"`
			SourceRefResolved struct {
				OK bool `yaml:"ok"`
			} `yaml:"source_ref_resolved"`
			SourceCommitReachable struct {
				OK bool `yaml:"ok"`
			} `yaml:"source_commit_reachable"`
			PackageIdentityMatch struct {
				OK bool `yaml:"ok"`
			} `yaml:"package_identity_match"`
			InstallPreview struct {
				OK bool `yaml:"ok"`
			} `yaml:"install_preview"`
		} `yaml:"validation"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(stdout), &candidate))
	require.Equal(t, 1, candidate.SchemaVersion)
	require.Equal(t, "packages/acme/index.yaml", candidate.TargetPath)
	require.True(t, candidate.Submittable)
	require.Equal(t, "orbit", candidate.PackageType)
	require.Equal(t, "docs", candidate.PackageIdentity)
	require.Equal(t, "acme/docs", candidate.PackageHandle)
	require.Equal(t, "0.1.0", candidate.Version)
	require.Equal(t, "active", candidate.PackageStatus)
	require.Equal(t, gitpkg.ComparablePath(fixture.SourceRemote), gitpkg.ComparablePath(candidate.Source.Remote))
	require.Equal(t, "orbit-template/docs", candidate.Source.Ref)
	require.Equal(t, fixture.SourceCommit, candidate.Source.Commit)
	require.True(t, candidate.Validation.SourceRemoteReachable.OK)
	require.True(t, candidate.Validation.SourceRefResolved.OK)
	require.True(t, candidate.Validation.SourceCommitReachable.OK)
	require.True(t, candidate.Validation.PackageIdentityMatch.OK)
	require.True(t, candidate.Validation.InstallPreview.OK)
}

func TestHyardRegistryEntryHarnessWritesStdoutCandidate(t *testing.T) {
	fixture := seedHyardRegistryEntryHarnessFixture(t)

	stdout, stderr, err := executeHyardCLI(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"harness",
		"acme/workspace@0.1.0",
		"--source",
		fixture.SourceRemote,
		"--ref",
		"harness-template/workspace",
		"--package",
		fixture.PackageIdentity,
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var candidate struct {
		SchemaVersion   int    `yaml:"schema_version"`
		TargetPath      string `yaml:"target_path"`
		Submittable     bool   `yaml:"submittable"`
		PackageType     string `yaml:"package_type"`
		PackageIdentity string `yaml:"package_identity"`
		PackageHandle   string `yaml:"package_handle"`
		Version         string `yaml:"version"`
		PackageStatus   string `yaml:"package_status"`
		Source          struct {
			Remote string `yaml:"remote"`
			Ref    string `yaml:"ref"`
			Commit string `yaml:"commit"`
		} `yaml:"source"`
		Validation struct {
			SourceRemoteReachable struct {
				OK bool `yaml:"ok"`
			} `yaml:"source_remote_reachable"`
			SourceRefResolved struct {
				OK bool `yaml:"ok"`
			} `yaml:"source_ref_resolved"`
			SourceCommitReachable struct {
				OK bool `yaml:"ok"`
			} `yaml:"source_commit_reachable"`
			PackageIdentityMatch struct {
				OK bool `yaml:"ok"`
			} `yaml:"package_identity_match"`
			InstallPreview struct {
				OK bool `yaml:"ok"`
			} `yaml:"install_preview"`
		} `yaml:"validation"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(stdout), &candidate))
	require.Equal(t, 1, candidate.SchemaVersion)
	require.Equal(t, "packages/acme/index.yaml", candidate.TargetPath)
	require.True(t, candidate.Submittable)
	require.Equal(t, "harness", candidate.PackageType)
	require.Equal(t, fixture.PackageIdentity, candidate.PackageIdentity)
	require.Equal(t, "acme/workspace", candidate.PackageHandle)
	require.Equal(t, "0.1.0", candidate.Version)
	require.Equal(t, "active", candidate.PackageStatus)
	require.Equal(t, gitpkg.ComparablePath(fixture.SourceRemote), gitpkg.ComparablePath(candidate.Source.Remote))
	require.Equal(t, "harness-template/workspace", candidate.Source.Ref)
	require.Equal(t, fixture.SourceCommit, candidate.Source.Commit)
	require.True(t, candidate.Validation.SourceRemoteReachable.OK)
	require.True(t, candidate.Validation.SourceRefResolved.OK)
	require.True(t, candidate.Validation.SourceCommitReachable.OK)
	require.True(t, candidate.Validation.PackageIdentityMatch.OK)
	require.True(t, candidate.Validation.InstallPreview.OK)
}

func TestHyardRegistryEntryHarnessHelpExplainsCandidateGeneration(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "registry", "entry", "harness", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Generate a Harness Package Registry Entry Candidate")
	require.Contains(t, stdout, "validates the source remote")
	require.Contains(t, stdout, "existing Harness install preview path")
	require.Contains(t, stdout, "--out")
	require.Contains(t, stdout, "--registry")
}

func TestHyardRegistryEntryOrbitHelpExplainsCandidateGeneration(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "registry", "entry", "orbit", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Generate an Orbit Package Registry Entry Candidate")
	require.Contains(t, stdout, "validates the source remote")
	require.Contains(t, stdout, "--out")
	require.Contains(t, stdout, "--registry")
}

func TestHyardRegistryEntryOrbitWritesOutAndRegistryTargets(t *testing.T) {
	lockHyardProcessEnv(t)

	fixture := seedHyardRegistryEntryOrbitFixture(t)

	outPath := filepath.Join(fixture.SourceRepo.Root, "candidate.yaml")
	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"orbit",
		"acme/docs@0.1.0",
		"--source",
		fixture.SourceRemote,
		"--ref",
		"orbit-template/docs",
		"--package",
		"docs",
		"--out",
		outPath,
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "wrote registry entry candidate")
	outData, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.Contains(t, string(outData), "target_path: packages/acme/index.yaml\n")
	require.Contains(t, string(outData), "commit: "+fixture.SourceCommit+"\n")

	registryCheckout := filepath.Join(fixture.SourceRepo.Root, "registry")
	stdout, stderr, err = executeHyardCLIUnlocked(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"orbit",
		"acme/docs@0.1.0",
		"--source",
		fixture.SourceRemote,
		"--ref",
		"orbit-template/docs",
		"--package",
		"docs",
		"--registry",
		registryCheckout,
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "wrote registry entry candidate")
	registryData, err := os.ReadFile(filepath.Join(registryCheckout, "packages", "acme", "index.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(registryData), "package_handle: acme/docs\n")
	require.Contains(t, string(registryData), "source_commit_reachable:\n")
}

func TestHyardRegistryEntryHarnessWritesOutAndRegistryTargets(t *testing.T) {
	fixture := seedHyardRegistryEntryHarnessFixture(t)

	outPath := filepath.Join(fixture.SourceRepo.Root, "candidate.yaml")
	stdout, stderr, err := executeHyardCLI(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"harness",
		"acme/workspace@0.1.0",
		"--source",
		fixture.SourceRemote,
		"--ref",
		"harness-template/workspace",
		"--package",
		fixture.PackageIdentity,
		"--out",
		outPath,
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "wrote registry entry candidate")
	outData, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.Contains(t, string(outData), "package_type: harness\n")
	require.Contains(t, string(outData), "commit: "+fixture.SourceCommit+"\n")

	registryCheckout := filepath.Join(fixture.SourceRepo.Root, "registry")
	stdout, stderr, err = executeHyardCLI(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"harness",
		"acme/workspace@0.1.0",
		"--source",
		fixture.SourceRemote,
		"--ref",
		"harness-template/workspace",
		"--package",
		fixture.PackageIdentity,
		"--registry",
		registryCheckout,
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "wrote registry entry candidate")
	registryData, err := os.ReadFile(filepath.Join(registryCheckout, "packages", "acme", "index.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(registryData), "package_handle: acme/workspace\n")
	require.Contains(t, string(registryData), "source_commit_reachable:\n")
}

func TestHyardRegistryEntryOrbitLocalPreviewCannotWriteCandidate(t *testing.T) {
	lockHyardProcessEnv(t)

	fixture := seedHyardRegistryEntryOrbitFixture(t)

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"orbit",
		"acme/docs@0.1.0",
		"--ref",
		"orbit-template/docs",
		"--package",
		"docs",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "submittable: false\n")
	require.Contains(t, stdout, "local-only preview has no source Git remote\n")

	stdout, stderr, err = executeHyardCLIUnlocked(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"orbit",
		"acme/docs@0.1.0",
		"--ref",
		"orbit-template/docs",
		"--package",
		"docs",
		"--out",
		"candidate.yaml",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "local-only registry entry previews cannot be written")
}

func TestHyardRegistryEntryHarnessLocalPreviewCannotWriteCandidate(t *testing.T) {
	fixture := seedHyardRegistryEntryHarnessFixture(t)

	stdout, stderr, err := executeHyardCLI(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"harness",
		"acme/workspace@0.1.0",
		"--ref",
		"harness-template/workspace",
		"--package",
		fixture.PackageIdentity,
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "submittable: false\n")
	require.Contains(t, stdout, "local-only preview has no source Git remote\n")

	stdout, stderr, err = executeHyardCLI(
		t,
		fixture.SourceRepo.Root,
		"registry",
		"entry",
		"harness",
		"acme/workspace@0.1.0",
		"--ref",
		"harness-template/workspace",
		"--package",
		fixture.PackageIdentity,
		"--out",
		"candidate.yaml",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "local-only registry entry previews cannot be written")
}

func TestHyardRegistryEntryOrbitValidationFailures(t *testing.T) {
	lockHyardProcessEnv(t)

	fixture := seedHyardRegistryEntryOrbitFixture(t)

	wrongCommit := "0000000000000000000000000000000000000000"
	cases := []struct {
		name          string
		sourceRemote  string
		sourceRef     string
		commit        string
		packageName   string
		errorContains string
	}{
		{
			name:          "remote",
			sourceRemote:  filepath.Join(t.TempDir(), "missing.git"),
			sourceRef:     "orbit-template/docs",
			packageName:   "docs",
			errorContains: "validate source remote reachability",
		},
		{
			name:          "ref",
			sourceRemote:  fixture.SourceRemote,
			sourceRef:     "orbit-template/missing",
			packageName:   "docs",
			errorContains: "validate source ref resolution",
		},
		{
			name:          "commit",
			sourceRemote:  fixture.SourceRemote,
			sourceRef:     "orbit-template/docs",
			commit:        wrongCommit,
			packageName:   "docs",
			errorContains: "not expected commit",
		},
		{
			name:          "package identity",
			sourceRemote:  fixture.SourceRemote,
			sourceRef:     "orbit-template/docs",
			packageName:   "api",
			errorContains: "validate package identity match",
		},
		{
			name:          "installability",
			sourceRemote:  fixture.BrokenSourceRemote,
			sourceRef:     "orbit-template/docs",
			packageName:   "docs",
			errorContains: "validate installability",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"registry",
				"entry",
				"orbit",
				"acme/docs@0.1.0",
				"--source",
				tc.sourceRemote,
				"--ref",
				tc.sourceRef,
				"--package",
				tc.packageName,
			}
			if tc.commit != "" {
				args = append(args, "--commit", tc.commit)
			}
			stdout, stderr, err := executeHyardCLIUnlocked(t, fixture.SourceRepo.Root, args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, tc.errorContains)
		})
	}
}

func TestHyardRegistryEntryHarnessValidationFailures(t *testing.T) {
	fixture := seedHyardRegistryEntryHarnessFixture(t)

	wrongCommit := "0000000000000000000000000000000000000000"
	cases := []struct {
		name          string
		sourceRemote  string
		sourceRef     string
		commit        string
		packageName   string
		errorContains string
	}{
		{
			name:          "remote",
			sourceRemote:  filepath.Join(t.TempDir(), "missing.git"),
			sourceRef:     "harness-template/workspace",
			packageName:   fixture.PackageIdentity,
			errorContains: "validate source remote reachability",
		},
		{
			name:          "ref",
			sourceRemote:  fixture.SourceRemote,
			sourceRef:     "harness-template/missing",
			packageName:   fixture.PackageIdentity,
			errorContains: "validate source ref resolution",
		},
		{
			name:          "commit",
			sourceRemote:  fixture.SourceRemote,
			sourceRef:     "harness-template/workspace",
			commit:        wrongCommit,
			packageName:   fixture.PackageIdentity,
			errorContains: "not expected commit",
		},
		{
			name:          "package identity",
			sourceRemote:  fixture.SourceRemote,
			sourceRef:     "harness-template/workspace",
			packageName:   "api",
			errorContains: "validate package identity match",
		},
		{
			name:          "installability",
			sourceRemote:  fixture.BrokenSourceRemote,
			sourceRef:     "harness-template/workspace",
			packageName:   fixture.BrokenPackageIdentity,
			errorContains: "validate installability",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"registry",
				"entry",
				"harness",
				"acme/workspace@0.1.0",
				"--source",
				tc.sourceRemote,
				"--ref",
				tc.sourceRef,
				"--package",
				tc.packageName,
			}
			if tc.commit != "" {
				args = append(args, "--commit", tc.commit)
			}
			stdout, stderr, err := executeHyardCLI(t, fixture.SourceRepo.Root, args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, tc.errorContains)
		})
	}
}

type hyardRegistryEntryOrbitFixture struct {
	SourceRepo         *testutil.Repo
	SourceRemote       string
	SourceCommit       string
	BrokenSourceRemote string
}

type hyardRegistryEntryHarnessFixture struct {
	SourceRepo            *testutil.Repo
	SourceRemote          string
	SourceCommit          string
	PackageIdentity       string
	BrokenSourceRemote    string
	BrokenPackageIdentity string
}

func seedHyardRegistryEntryOrbitFixture(t *testing.T) hyardRegistryEntryOrbitFixture {
	t.Helper()

	sourceRepo := seedCommittedHyardSourceRepo(t)
	_, stderr, err := executeHyardCLIUnlocked(t, sourceRepo.Root, "publish", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	sourceRemote := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	sourceCommit := sourceRepo.RevParse(t, "orbit-template/docs")

	brokenRepo := seedCommittedHyardSourceRepo(t)
	_, stderr, err = executeHyardCLIUnlocked(t, brokenRepo.Root, "publish", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	brokenRepo.Run(t, "switch", "orbit-template/docs")
	brokenRepo.Run(t, "rm", ".harness/orbits/docs.yaml")
	brokenRepo.AddAndCommit(t, "break orbit template installability")
	brokenRemote := testutil.NewBareRemoteFromRepo(t, brokenRepo)

	return hyardRegistryEntryOrbitFixture{
		SourceRepo:         sourceRepo,
		SourceRemote:       sourceRemote,
		SourceCommit:       sourceCommit,
		BrokenSourceRemote: brokenRemote,
	}
}

func seedHyardRegistryEntryHarnessFixture(t *testing.T) hyardRegistryEntryHarnessFixture {
	t.Helper()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	sourceRemote := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	sourceCommit := sourceRepo.RevParse(t, "harness-template/workspace")
	packageIdentity := harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root)

	brokenRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	brokenRepo.Run(t, "switch", "harness-template/workspace")
	brokenRepo.Run(t, "rm", ".harness/template.yaml")
	brokenRepo.AddAndCommit(t, "break harness template installability")
	brokenRemote := testutil.NewBareRemoteFromRepo(t, brokenRepo)

	return hyardRegistryEntryHarnessFixture{
		SourceRepo:            sourceRepo,
		SourceRemote:          sourceRemote,
		SourceCommit:          sourceCommit,
		PackageIdentity:       packageIdentity,
		BrokenSourceRemote:    brokenRemote,
		BrokenPackageIdentity: harnesspkg.DefaultHarnessIDForPath(brokenRepo.Root),
	}
}
