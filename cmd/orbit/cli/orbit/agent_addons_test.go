package orbit_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func TestResolveAgentAddonHooksReportsPackageScopedHooks(t *testing.T) {
	t.Parallel()

	spec := agentAddonHookSpec(t, "docs", "hooks/docs/block-dangerous-shell/run.sh")

	resolved, err := orbitpkg.ResolveAgentAddonHooks(spec, []string{
		"docs/guide.md",
		"hooks/docs/block-dangerous-shell/run.sh",
	}, []string{
		"docs/guide.md",
		"hooks/docs/block-dangerous-shell/run.sh",
	})
	require.NoError(t, err)
	require.Equal(t, []orbitpkg.ResolvedAgentAddonHook{{
		Package:             "docs",
		ID:                  "block-dangerous-shell",
		DisplayID:           "docs:block-dangerous-shell",
		Required:            true,
		Description:         "Block destructive shell commands before execution.",
		EventKind:           "tool.before",
		Tools:               []string{"shell"},
		CommandPatterns:     []string{"rm -rf *"},
		HandlerType:         "command",
		HandlerPath:         "hooks/docs/block-dangerous-shell/run.sh",
		TimeoutSeconds:      30,
		StatusMessage:       "Checking shell command",
		Targets:             map[string]bool{"claude": true, "codex": true, "openclaw": false},
		UnsupportedBehavior: "skip",
	}}, resolved)
}

func TestResolveAgentAddonHooksRejectsHandlersOutsideExportSurface(t *testing.T) {
	t.Parallel()

	spec := agentAddonHookSpec(t, "docs", "hooks/docs/block-dangerous-shell/run.sh")

	_, err := orbitpkg.ResolveAgentAddonHooks(spec, []string{
		"docs/guide.md",
		"hooks/docs/block-dangerous-shell/run.sh",
	}, []string{
		"docs/guide.md",
	})
	require.ErrorContains(t, err, `agent_addons.hooks.entries[0].handler.path "hooks/docs/block-dangerous-shell/run.sh" must resolve inside the export surface`)
}

func TestParseHostedOrbitSpecRejectsUnsafeAgentAddonHookHandlers(t *testing.T) {
	t.Parallel()

	for name, handlerPath := range map[string]string{
		"absolute path": "/tmp/run.sh",
		"parent escape": "../run.sh",
		"remote URL":    "https://example.com/run.sh",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := orbitpkg.ParseHostedOrbitSpecData([]byte(agentAddonHookSpecYAML("docs", handlerPath)), "/repo/.harness/orbits/docs.yaml")
			require.Error(t, err)
			require.ErrorContains(t, err, "agent_addons.hooks.entries[0].handler.path")
		})
	}
}

func agentAddonHookSpec(t *testing.T, orbitID string, handlerPath string) orbitpkg.OrbitSpec {
	t.Helper()

	spec, err := orbitpkg.ParseHostedOrbitSpecData([]byte(agentAddonHookSpecYAML(orbitID, handlerPath)), "/repo/.harness/orbits/"+orbitID+".yaml")
	require.NoError(t, err)

	return spec
}

func agentAddonHookSpecYAML(orbitID string, handlerPath string) string {
	return "" +
		"package:\n" +
		"  type: orbit\n" +
		"  name: " + orbitID + "\n" +
		"meta:\n" +
		"  file: .harness/orbits/" + orbitID + ".yaml\n" +
		"  include_in_projection: true\n" +
		"  include_in_write: true\n" +
		"  include_in_export: true\n" +
		"  include_description_in_orchestration: true\n" +
		"agent_addons:\n" +
		"  hooks:\n" +
		"    unsupported_behavior: skip\n" +
		"    entries:\n" +
		"      - id: block-dangerous-shell\n" +
		"        required: true\n" +
		"        description: Block destructive shell commands before execution.\n" +
		"        event:\n" +
		"          kind: tool.before\n" +
		"        match:\n" +
		"          tools: [shell]\n" +
		"          command_patterns:\n" +
		"            - \"rm -rf *\"\n" +
		"        handler:\n" +
		"          type: command\n" +
		"          path: " + handlerPath + "\n" +
		"          timeout_seconds: 30\n" +
		"          status_message: Checking shell command\n" +
		"        targets:\n" +
		"          codex: true\n" +
		"          claude: true\n" +
		"          openclaw: false\n" +
		"members:\n" +
		"  - name: docs-content\n" +
		"    role: subject\n" +
		"    paths:\n" +
		"      include:\n" +
		"        - docs/**\n"
}
