package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHarnessRootHelpIncludesV03Examples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness create demo-repo")
	require.Contains(t, stdout, "harness bindings plan orbit-template/docs orbit-template/cmd")
	require.Contains(t, stdout, "harness init")
	require.Contains(t, stdout, "harness install batch orbit-template/docs orbit-template/cmd --bindings .harness/vars.yaml")
	require.Contains(t, stdout, "harness guidance compose --target all")
	require.Contains(t, stdout, "harness bootstrap complete --orbit docs")
	require.Contains(t, stdout, "harness bootstrap reopen --orbit docs")
	require.Contains(t, stdout, "harness ready")
	require.Contains(t, stdout, "harness check")
	require.Contains(t, stdout, "harness template save --to harness-template/workspace")
	require.Contains(t, stdout, "harness template publish --to harness-template/workspace --push --remote origin")
}

func TestHarnessBindingsPlanHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "bindings", "plan", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness bindings plan orbit-template/docs orbit-template/cmd")
	require.Contains(t, stdout, "harness bindings plan orbit-template/docs orbit-template/cmd --out .harness/vars.yaml")
	require.Contains(t, stdout, "harness bindings plan orbit-template/docs orbit-template/cmd --json")
	require.Contains(t, stdout, "shared bindings skeleton")
	require.Contains(t, stdout, "--progress")
}

func TestHarnessBindingsMissingHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "bindings", "missing", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness bindings missing --orbit docs")
	require.Contains(t, stdout, "harness bindings missing --all --json")
	require.Contains(t, stdout, "install-backed orbit")
}

func TestHarnessBindingsScanRuntimeHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "bindings", "scan-runtime", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness bindings scan-runtime --orbit docs")
	require.Contains(t, stdout, "harness bindings scan-runtime --all --json")
	require.Contains(t, stdout, "harness bindings scan-runtime --orbit docs --write-install")
	require.Contains(t, stdout, "runtime markdown")
}

func TestHarnessBindingsApplyHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "bindings", "apply", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness bindings apply --orbit docs --dry-run")
	require.Contains(t, stdout, "harness bindings apply --orbit docs")
	require.Contains(t, stdout, "harness bindings apply --orbit docs --force")
	require.Contains(t, stdout, "install-backed orbit")
	require.Contains(t, stdout, "--progress")
}

func TestHarnessCheckHelpIncludesProgressFlag(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "check", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Diagnose harness runtime schema, membership, and install consistency")
	require.Contains(t, stdout, "--progress")
}

func TestHarnessReadyHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "ready", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness ready")
	require.Contains(t, stdout, "harness ready --json")
	require.Contains(t, stdout, "broken / usable / ready")
}

func TestHarnessAgentsComposeHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "agents", "compose", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness agents compose")
	require.Contains(t, stdout, "harness agents compose --force")
	require.Contains(t, stdout, "root AGENTS.md")
}

func TestHarnessBootstrapCompleteHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "bootstrap", "complete", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness bootstrap complete --orbit docs")
	require.Contains(t, stdout, "harness bootstrap complete --orbit docs --json")
	require.Contains(t, stdout, "harness bootstrap complete --all")
	require.Contains(t, stdout, "BOOTSTRAP.md")
}

func TestHarnessBootstrapReopenHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "bootstrap", "reopen", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness bootstrap reopen --orbit docs")
	require.Contains(t, stdout, "harness bootstrap reopen --orbit docs --restore-surface")
	require.Contains(t, stdout, "harness bootstrap reopen --all --json")
	require.Contains(t, stdout, "pending")
}

func TestHarnessGuidanceComposeHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "guidance", "compose", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness guidance compose --target all")
	require.Contains(t, stdout, "harness guidance compose --target humans --json")
	require.Contains(t, stdout, "harness guidance compose --target bootstrap --force")
	require.Contains(t, stdout, "--target")
	require.Contains(t, stdout, "AGENTS.md")
	require.Contains(t, stdout, "HUMANS.md")
	require.Contains(t, stdout, "BOOTSTRAP.md")
}

func TestHarnessHumansComposeHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "humans", "compose", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness humans compose")
	require.Contains(t, stdout, "harness humans compose --force")
	require.Contains(t, stdout, "root HUMANS.md")
}

func TestHarnessFrameworkListHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "framework", "list", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness framework list")
	require.Contains(t, stdout, "harness framework list --json")
	require.Contains(t, stdout, "supported framework adapters")
}

func TestHarnessFrameworkUseHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "framework", "use", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness framework use claude")
	require.Contains(t, stdout, "harness framework use codex --json")
	require.Contains(t, stdout, "selection.json")
}

func TestHarnessFrameworkRecommendHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "framework", "recommend", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "harness framework recommend set codex")
	require.Contains(t, stdout, "harness framework recommend show")
	require.Contains(t, stdout, "recommended framework")
}

func TestHarnessAgentDeriveHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "agent", "derive", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness agent derive")
	require.Contains(t, stdout, "harness agent derive --json")
	require.Contains(t, stdout, ".harness/agents/*")
}

func TestHarnessFrameworkInspectHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "framework", "inspect", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness framework inspect")
	require.Contains(t, stdout, "harness framework inspect --json")
	require.Contains(t, stdout, "recommended framework")
}

func TestHarnessFrameworkPlanHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "framework", "plan", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness framework plan")
	require.Contains(t, stdout, "harness framework plan --json")
	require.Contains(t, stdout, "project materialization plan")
	require.Contains(t, stdout, "global registration plan")
}

func TestHarnessFrameworkApplyHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "framework", "apply", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness framework apply")
	require.Contains(t, stdout, "harness framework apply --json")
	require.Contains(t, stdout, "activation ledger")
}

func TestHarnessFrameworkCheckHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "framework", "check", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness framework check")
	require.Contains(t, stdout, "harness framework check --json")
	require.Contains(t, stdout, "operational prerequisites")
}

func TestHarnessFrameworkRemoveHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "framework", "remove", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness framework remove")
	require.Contains(t, stdout, "harness framework remove --json")
	require.Contains(t, stdout, "owned side effects")
}

func TestHarnessInstallHelpIncludesFormalExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "install", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness install orbit-template/docs --bindings .harness/vars.yaml")
	require.Contains(t, stdout, "harness install https://example.com/acme/templates.git --ref orbit-template/docs --bindings .harness/vars.yaml")
	require.Contains(t, stdout, "harness install orbit-template/docs --overwrite-existing --bindings .harness/vars.yaml --json")
	require.Contains(t, stdout, "--allow-unresolved-bindings")
	require.Contains(t, stdout, "--strict-bindings")
	require.Contains(t, stdout, "--progress")
}

func TestHarnessInstallBatchHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "install", "batch", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness install batch orbit-template/docs orbit-template/cmd --bindings .harness/vars.yaml --dry-run")
	require.Contains(t, stdout, "harness install batch orbit-template/docs orbit-template/cmd --bindings .harness/vars.yaml --json")
	require.Contains(t, stdout, "--allow-unresolved-bindings")
	require.Contains(t, stdout, "--strict-bindings")
	require.Contains(t, stdout, "shared preview")
}

func TestHarnessTemplateSaveHelpIncludesDefaultExample(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "template", "save", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness template save --to harness-template/workspace --dry-run")
	require.Contains(t, stdout, "--edit-template")
	require.Contains(t, stdout, "harness template save --to harness-template/workspace --include-bootstrap")
	require.Contains(t, stdout, "harness template save --to harness-template/workspace --default --json")
	require.Contains(t, stdout, "--dry-run")
	require.Contains(t, stdout, "--overwrite")
	require.Contains(t, stdout, "--default")
	require.Contains(t, stdout, "--progress")
}

func TestHarnessTemplatePublishHelpIncludesExamples(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "template", "publish", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Examples:")
	require.Contains(t, stdout, "harness template publish --to harness-template/workspace")
	require.Contains(t, stdout, "harness template publish --to harness-template/workspace --default")
	require.Contains(t, stdout, "harness template publish --to harness-template/workspace --push --remote origin")
	require.Contains(t, stdout, "harness template publish --to harness-template/workspace --json")
	require.Contains(t, stdout, "--push")
	require.Contains(t, stdout, "--remote")
}
