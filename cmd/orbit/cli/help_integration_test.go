package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPhaseTwoCommandHelpIncludesCurrentExamples(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "capability",
			args: []string{"capability", "--help"},
			want: []string{
				"list",
				"migrate-v0-66",
				"set",
			},
		},
		{
			name: "capability set",
			args: []string{"capability", "set", "--help"},
			want: []string{
				"commands-paths",
				"skills-local-paths",
				"skills-remote-uris",
			},
		},
		{
			name: "capability migrate-v0-66",
			args: []string{"capability", "migrate-v0-66", "--help"},
			want: []string{
				"orbit capability migrate-v0-66 --orbit execute",
				"--orbit",
				"--json",
			},
		},
		{
			name: "capability set commands-paths",
			args: []string{"capability", "set", "commands-paths", "--help"},
			want: []string{
				"orbit capability set commands-paths --orbit execute --include 'commands/execute/**/*.md'",
				"--orbit",
				"--include",
				"--exclude",
			},
		},
		{
			name: "guidance materialize",
			args: []string{"guidance", "materialize", "--help"},
			want: []string{
				"Examples:",
				"orbit guidance materialize --orbit docs --target all",
				"orbit guidance materialize --orbit docs --target all --seed-empty",
				"orbit guidance materialize --orbit docs --target bootstrap",
				"Supported revision kinds: runtime, source, orbit_template.",
				"--target",
				"--seed-empty",
			},
		},
		{
			name: "guidance backfill",
			args: []string{"guidance", "backfill", "--help"},
			want: []string{
				"Examples:",
				"orbit guidance backfill --orbit docs --target all",
				"orbit guidance backfill --orbit docs --target humans --check",
				"orbit guidance backfill --orbit docs --target bootstrap --json",
				"Supported revision kinds: runtime, source, orbit_template.",
			},
		},
		{
			name: "brief materialize",
			args: []string{"brief", "materialize", "--help"},
			want: []string{
				"Examples:",
				"orbit brief materialize",
				"orbit brief materialize --orbit docs",
				"orbit brief materialize --orbit docs --check",
				"orbit brief materialize --orbit docs --force",
				"Supported revision kinds: runtime, source, orbit_template.",
			},
		},
		{
			name: "brief backfill",
			args: []string{"brief", "backfill", "--help"},
			want: []string{
				"Examples:",
				"orbit brief backfill",
				"orbit brief backfill --orbit docs",
				"orbit brief backfill --orbit docs --check",
				"orbit brief backfill --orbit docs --json",
				"Supported revision kinds: runtime, source, orbit_template.",
			},
		},
		{
			name: "template publish",
			args: []string{"template", "publish", "--help"},
			want: []string{
				"Examples:",
				"orbit template publish",
				"orbit template publish --backfill-brief",
				"orbit template publish --allow-out-of-range-skills",
				"orbit template publish --aggregate-detected-skills",
				"orbit template publish --default",
				"orbit template publish --push --remote origin",
				"orbit template publish --json",
				"--progress",
			},
		},
		{
			name: "template init",
			args: []string{"template", "init", "--help"},
			want: []string{
				"orbit template init",
				"orbit_template branch",
				"Optional when the current Git repo already contains exactly one hosted orbit definition",
				"--orbit",
				"--with-spec",
				"--json",
			},
		},
		{
			name: "template init-source",
			args: []string{"template", "init-source", "--help"},
			want: []string{
				"Examples:",
				"orbit template init-source",
				"orbit template init-source --json",
				"Initialize the current branch as a single-orbit source branch",
			},
		},
		{
			name: "source create",
			args: []string{"source", "create", "--help"},
			want: []string{
				"Examples:",
				"orbit source create ./research-source --orbit research",
				"orbit source create ./research-source --orbit research --json",
				"Create a new source authoring repository",
				"--orbit",
				"--with-spec",
			},
		},
		{
			name: "source init",
			args: []string{"source", "init", "--help"},
			want: []string{
				"orbit source init",
				"single-orbit source branch",
				"Optional when the current Git repo already contains exactly one hosted orbit definition",
				"--orbit",
				"--with-spec",
				"--json",
			},
		},
		{
			name: "template create",
			args: []string{"template", "create", "--help"},
			want: []string{
				"Examples:",
				"orbit template create ./research-template --orbit research",
				"orbit template create ./research-template --orbit research --json",
				"Create a new orbit template authoring repository",
				"--orbit",
				"--with-spec",
			},
		},
		{
			name: "template save",
			args: []string{"template", "save", "--help"},
			want: []string{
				"Examples:",
				"orbit template save docs --dry-run",
				"orbit template save docs --to orbit-template/docs --dry-run",
				"orbit template save docs --backfill-brief --to orbit-template/docs",
				"orbit template save docs --to orbit-template/docs --allow-out-of-range-skills",
				"orbit template save docs --to orbit-template/docs --aggregate-detected-skills",
				"orbit template save docs --to orbit-template/docs --include-completed-bootstrap",
				"orbit template save docs --to orbit-template/docs --edit-template",
				"orbit template save docs --to orbit-template/docs --default --json",
				"If --to is omitted, the command reuses the installed template source_ref",
				"--progress",
			},
		},
		{
			name: "bindings init",
			args: []string{"bindings", "init", "--help"},
			want: []string{
				"Examples:",
				"orbit bindings init orbit-template/docs",
				"orbit bindings init orbit-template/docs --out .harness/vars.yaml",
				"orbit bindings init https://example.com/acme/templates.git --json",
				"without modifying the current repository",
				"--progress",
			},
		},
		{
			name: "branch status",
			args: []string{"branch", "status", "--help"},
			want: []string{
				"Examples:",
				"orbit branch status",
				"orbit branch status --json",
				"current checkout worktree",
				"orbit branch inspect <rev>",
			},
		},
		{
			name: "branch inspect",
			args: []string{"branch", "inspect", "--help"},
			want: []string{
				"Examples:",
				"orbit branch inspect orbit-template/docs",
				"orbit branch inspect main --json",
			},
		},
		{
			name: "branch list",
			args: []string{"branch", "list", "--help"},
			want: []string{
				"Examples:",
				"orbit branch list",
				"orbit branch list --json",
				"This command only lists local branches in Phase 2.",
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := executeCLI(t, t.TempDir(), testCase.args...)
			require.NoError(t, err)
			require.Empty(t, stderr)

			for _, expected := range testCase.want {
				require.Contains(t, stdout, expected)
			}
			switch testCase.name {
			case "capability":
				require.NotContains(t, stdout, "add")
				require.NotContains(t, stdout, "remove")
			case "capability set":
				require.NotContains(t, stdout, "Compatibility path")
			}
		})
	}
}

func TestOrbitRootHelpIncludesV03Examples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeCLI(t, t.TempDir(), "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "orbit create docs")
	require.Contains(t, stdout, "orbit enter docs")
	require.Contains(t, stdout, "orbit template save docs --to orbit-template/docs")
	require.Contains(t, stdout, "orbit bindings init orbit-template/docs --out .harness/vars.yaml")
}

func TestOrbitInitHelpMarksCompatibilityWrapper(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeCLI(t, t.TempDir(), "init", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "legacy Orbit compatibility config")
	require.Contains(t, stdout, "deprecated")
	require.Contains(t, stdout, "use `harness init`")
}

func TestTemplateApplyHelpMarksLegacyWrapper(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeCLI(t, t.TempDir(), "template", "apply", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Legacy compatibility wrapper around `harness install`.")
	require.Contains(t, stdout, "Prefer `harness install` for new runtime installs.")
	require.Contains(t, stdout, "harness install orbit-template/docs --bindings .harness/vars.yaml")
	require.Contains(t, stdout, "harness install https://example.com/acme/templates.git --ref orbit-template/docs --bindings .harness/vars.yaml")
	require.Contains(t, stdout, "--ref is only valid for external Git sources.")
	require.NotContains(t, stdout, "orbit template apply orbit-template/docs --dry-run --json")
	require.NotContains(t, stdout, "orbit template apply orbit-template/docs --editor")
	require.NotContains(t, stdout, "orbit template apply https://example.com/acme/templates.git --ref orbit-template/docs")
}

func TestTemplateHelpHidesApplyWrapperFromPublicSubcommandList(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeCLI(t, t.TempDir(), "template", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "\n  create ")
	require.Contains(t, stdout, "\n  init ")
	require.Contains(t, stdout, "\n  init-source ")
	require.Contains(t, stdout, "\n  publish ")
	require.Contains(t, stdout, "\n  save ")
	require.NotContains(t, stdout, "\n  apply ")
}

func TestSourceHelpShowsCreateAndInitCommands(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeCLI(t, t.TempDir(), "source", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "\n  create ")
	require.Contains(t, stdout, "\n  init ")
}
