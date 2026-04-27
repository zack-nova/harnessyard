package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHarnessInstallOrbitWithAgentAddonsReportsAgentAddonsAndMissingActivation(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", "orbit-template/docs")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"content:\n"+
		"  - key: docs-content\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"        - hooks/**\n"+
		"behavior:\n"+
		"  scope:\n"+
		"    projection_roles:\n"+
		"      - meta\n"+
		"      - subject\n"+
		"      - rule\n"+
		"      - process\n"+
		"    write_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"    export_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"    orchestration_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"      - process\n"+
		"  orchestration:\n"+
		"    include_orbit_description: true\n"+
		"    materialize_agents_from_meta: true\n"+
		"agent_addons:\n"+
		"  hooks:\n"+
		"    entries:\n"+
		"      - id: block-dangerous-shell\n"+
		"        required: true\n"+
		"        event:\n"+
		"          kind: tool.before\n"+
		"        match:\n"+
		"          tools:\n"+
		"            - shell\n"+
		"        handler:\n"+
		"          type: command\n"+
		"          path: hooks/docs/block-dangerous-shell/run.sh\n"+
		"        targets:\n"+
		"          codex: true\n")
	repo.WriteFile(t, "hooks/docs/block-dangerous-shell/run.sh", "#!/usr/bin/env bash\nexit 0\n")
	repo.AddAndCommit(t, "add agent addon")
	repo.Run(t, "checkout", runtimeBranch)

	_, _, err := executeHarnessCLI(t, repo.Root, "agent", "use", "codex", "--json")
	require.NoError(t, err)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		AgentAddons struct {
			Hooks []struct {
				DisplayID     string `json:"display_id"`
				Activation    string `json:"activation"`
				HandlerPath   string `json:"handler_path"`
				HandlerDigest string `json:"handler_digest"`
			} `json:"hooks"`
		} `json:"agent_addons"`
		Readiness struct {
			Status         string `json:"status"`
			RuntimeReasons []struct {
				Code string `json:"code"`
			} `json:"runtime_reasons"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.AgentAddons.Hooks, 1)
	require.Equal(t, "docs:block-dangerous-shell", payload.AgentAddons.Hooks[0].DisplayID)
	require.Equal(t, "not_applied", payload.AgentAddons.Hooks[0].Activation)
	require.Equal(t, "hooks/docs/block-dangerous-shell/run.sh", payload.AgentAddons.Hooks[0].HandlerPath)
	require.NotEmpty(t, payload.AgentAddons.Hooks[0].HandlerDigest)
	require.Equal(t, "usable", payload.Readiness.Status)
	require.Contains(t, harnessReadinessCodes(payload.Readiness.RuntimeReasons), "agent_activation_missing")

	_, err = os.Stat(filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func harnessReadinessCodes(reasons []struct {
	Code string `json:"code"`
}) []string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, reason.Code)
	}
	return codes
}
