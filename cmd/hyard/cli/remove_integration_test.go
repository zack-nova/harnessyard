package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardRemoveOrbitRemovesUnambiguousRuntimeOrbit(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "remove", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		TargetType   string `json:"target_type"`
		OrbitPackage string `json:"orbit_package"`
		OrbitID      string `json:"orbit_id"`
		RemoveMode   string `json:"remove_mode"`
		MemberCount  int    `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "orbit", payload.TargetType)
	require.Equal(t, "docs", payload.OrbitPackage)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "runtime_cleanup", payload.RemoveMode)
	require.Equal(t, 0, payload.MemberCount)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)
}

func TestHyardUninstallOrbitJSONRemovesInstallBackedRuntimeOrbit(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardInstallBackedRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "uninstall", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Action                string `json:"action"`
		TargetType            string `json:"target_type"`
		OrbitPackage          string `json:"orbit_package"`
		OrbitID               string `json:"orbit_id"`
		MemberSource          string `json:"member_source"`
		RemoveMode            string `json:"remove_mode"`
		MemberCount           int    `json:"member_count"`
		DetachedInstallRecord bool   `json:"detached_install_record"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "uninstall", payload.Action)
	require.Equal(t, "orbit", payload.TargetType)
	require.Equal(t, "docs", payload.OrbitPackage)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "install_orbit", payload.MemberSource)
	require.Equal(t, "runtime_cleanup", payload.RemoveMode)
	require.Equal(t, 0, payload.MemberCount)
	require.True(t, payload.DetachedInstallRecord)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallRecordStatusDetached, orbittemplate.EffectiveInstallRecordStatus(record))
}

func TestHyardUninstallOrbitTextDisclosesManualRuntimeOrbitSource(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "uninstall", "orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "uninstalled orbit package docs from "+repo.Root)
	require.Contains(t, stdout, "member_source: manual")
	require.NotContains(t, stdout, "removed orbit package docs")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)
}

func TestHyardRemoveOrbitRecompilesLedgerOwnedPackageHookOutputs(t *testing.T) {
	repo := seedCommittedHyardRuntimeWithTwoAgentAddonHooks(t)
	selectHyardAgentAddonFramework(t, repo)
	makeHyardAgentAddonHandlerExecutable(t, repo)
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "ops", "block-dangerous-shell", "run.sh"), 0o755))

	lockHyardProcessEnv(t)
	t.Setenv("HOME", t.TempDir())
	stubCodexExecutableOnPath(t)

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "Apply hook activation? accepted by --yes")

	var applyPayload struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &applyPayload))
	require.Equal(t, "ok", applyPayload.Status)
	require.FileExists(t, filepath.Join(repo.Root, ".codex", "hooks.json"))

	stdout, stderr, err = executeHyardCLIUnlocked(t, repo.Root, "remove", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		AgentCleanup struct {
			Status            string   `json:"status"`
			RemovedOutputs    []string `json:"removed_outputs"`
			RecompiledOutputs []string `json:"recompiled_outputs"`
			BlockedOutputs    []string `json:"blocked_outputs"`
		} `json:"agent_cleanup"`
		Readiness struct {
			Agent struct {
				Status string `json:"status"`
			} `json:"agent"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "applied", payload.AgentCleanup.Status)
	require.Empty(t, payload.AgentCleanup.BlockedOutputs)
	require.Contains(t, payload.AgentCleanup.RecompiledOutputs, ".codex/config.toml")
	require.Contains(t, payload.AgentCleanup.RecompiledOutputs, ".codex/hooks.json")
	require.NotContains(t, payload.AgentCleanup.RemovedOutputs, ".codex/hooks.json")
	require.Equal(t, "ready", payload.Readiness.Agent.Status)

	codexHooksData, err := os.ReadFile(filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.NoError(t, err)
	codexHooks := string(codexHooksData)
	require.NotContains(t, codexHooks, "--hook docs:block-dangerous-shell")
	require.Contains(t, codexHooks, "--hook ops:block-dangerous-shell")

	activation, err := harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "codex")
	require.NoError(t, err)
	require.Len(t, activation.PackageHooks, 1)
	require.Equal(t, "ops", activation.PackageHooks[0].Package)
	require.Equal(t, "ops:block-dangerous-shell", activation.PackageHooks[0].DisplayID)

	stdout, stderr, err = executeHyardCLIUnlocked(t, repo.Root, "agent", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var checkPayload struct {
		OK           bool `json:"ok"`
		FindingCount int  `json:"finding_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &checkPayload))
	require.True(t, checkPayload.OK)
	require.Zero(t, checkPayload.FindingCount)
}

func TestHyardRemoveOrbitRemovesLedgerOwnedPackageHookOutputsWhenLastAddonIsRemoved(t *testing.T) {
	repo := seedHyardRuntimeWithAgentAddonHook(t)
	repo.AddAndCommit(t, "seed docs package hook")
	selectHyardAgentAddonFramework(t, repo)
	makeHyardAgentAddonHandlerExecutable(t, repo)

	lockHyardProcessEnv(t)
	t.Setenv("HOME", t.TempDir())

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "Apply hook activation? accepted by --yes")

	var applyPayload struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &applyPayload))
	require.Equal(t, "ok", applyPayload.Status)
	require.FileExists(t, filepath.Join(repo.Root, ".codex", "hooks.json"))

	stdout, stderr, err = executeHyardCLIUnlocked(t, repo.Root, "remove", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		AgentCleanup struct {
			Status            string   `json:"status"`
			RemovedOutputs    []string `json:"removed_outputs"`
			RecompiledOutputs []string `json:"recompiled_outputs"`
			BlockedOutputs    []string `json:"blocked_outputs"`
		} `json:"agent_cleanup"`
		Readiness struct {
			Status string `json:"status"`
			Agent  struct {
				Status           string `json:"status"`
				ActivationStatus string `json:"activation_status"`
				Reasons          []struct {
					Code string `json:"code"`
				} `json:"reasons"`
			} `json:"agent"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "applied", payload.AgentCleanup.Status)
	require.Empty(t, payload.AgentCleanup.BlockedOutputs)
	require.Contains(t, payload.AgentCleanup.RemovedOutputs, ".codex/hooks.json")
	require.Contains(t, payload.AgentCleanup.RemovedOutputs, ".codex/config.toml")
	require.Empty(t, payload.AgentCleanup.RecompiledOutputs)
	require.Equal(t, "usable", payload.Readiness.Status)
	require.Equal(t, "usable", payload.Readiness.Agent.Status)
	require.Equal(t, "missing", payload.Readiness.Agent.ActivationStatus)
	require.Contains(t, readinessReasonCodesFromApplyPayload(payload.Readiness.Agent.Reasons), "agent_activation_missing")

	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "config.toml"))

	_, err = harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "codex")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardRemoveOrbitFailsClosedWhenPackageHookOutputIsUserOwned(t *testing.T) {
	repo := seedHyardRuntimeWithAgentAddonHook(t)
	repo.AddAndCommit(t, "seed docs package hook")
	selectHyardAgentAddonFramework(t, repo)
	makeHyardAgentAddonHandlerExecutable(t, repo)

	lockHyardProcessEnv(t)
	t.Setenv("HOME", t.TempDir())

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "Apply hook activation? accepted by --yes")

	var applyPayload struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &applyPayload))
	require.Equal(t, "ok", applyPayload.Status)

	userOwnedHooks := []byte("{\"user\":\"owned\"}\n")
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".codex", "hooks.json"), userOwnedHooks, 0o600))

	stdout, stderr, err = executeHyardCLIUnlocked(t, repo.Root, "remove", "orbit", "docs", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "agent cleanup is blocked by unowned outputs")
	require.ErrorContains(t, err, ".codex/hooks.json")

	codexHooksData, err := os.ReadFile(filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.NoError(t, err)
	require.Equal(t, userOwnedHooks, codexHooksData)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
}

func TestHyardRemoveOrbitRemovesLedgerOwnedPackageSkillOutput(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeWithTwoSkillPackages(t)
	selectHyardAgentAddonFramework(t, repo)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "agent", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var applyPayload struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &applyPayload))
	require.Equal(t, "ok", applyPayload.Status)
	require.FileExists(t, filepath.Join(repo.Root, ".codex", "skills", "docs-style"))
	require.FileExists(t, filepath.Join(repo.Root, ".codex", "skills", "ops-style"))

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "remove", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		AgentCleanup struct {
			Status         string   `json:"status"`
			RemovedOutputs []string `json:"removed_outputs"`
			BlockedOutputs []string `json:"blocked_outputs"`
		} `json:"agent_cleanup"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "applied", payload.AgentCleanup.Status)
	require.Empty(t, payload.AgentCleanup.BlockedOutputs)
	require.Contains(t, payload.AgentCleanup.RemovedOutputs, ".codex/skills/docs-style")

	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", "docs-style"))
	require.FileExists(t, filepath.Join(repo.Root, ".codex", "skills", "ops-style"))

	activation, err := harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "codex")
	require.NoError(t, err)
	var outputPaths []string
	for _, output := range activation.ProjectOutputs {
		outputPaths = append(outputPaths, output.Path)
	}
	require.NotContains(t, outputPaths, ".codex/skills/docs-style")
	require.Contains(t, outputPaths, ".codex/skills/ops-style")
}

func TestHyardRemoveRejectsVersionedPackageCoordinate(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "remove", "docs@0.1.0", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `remove package "docs@0.1.0" must use the installed package name`)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
}

func TestHyardUninstallOrbitRejectsVersionedPackageCoordinate(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "uninstall", "orbit", "docs@0.1.0", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `uninstall package "docs@0.1.0" must use the installed package name`)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
}

func TestHyardRemoveOrbitDisambiguationRemovesOrbitWhenHarnessHasSameName(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)
	repo := testRepoAtRoot(t, runtimeRoot)
	addHyardHostedOrbitDefinition(t, repo, harnessID)
	require.NoError(t, executeHarnessCLIForHyardTest(t, runtimeRoot, "add", harnessID))

	stdout, stderr, err := executeHyardCLI(t, runtimeRoot, "remove", "orbit", harnessID, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		TargetType   string `json:"target_type"`
		OrbitPackage string `json:"orbit_package"`
		MemberCount  int    `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "orbit", payload.TargetType)
	require.Equal(t, harnessID, payload.OrbitPackage)
	require.Equal(t, 1, payload.MemberCount)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRoot)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
}

func TestHyardRemoveBareNameFailsClosedWhenOrbitAndHarnessAreAmbiguous(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)
	repo := testRepoAtRoot(t, runtimeRoot)
	addHyardHostedOrbitDefinition(t, repo, harnessID)
	require.NoError(t, executeHarnessCLIForHyardTest(t, runtimeRoot, "add", harnessID))

	stdout, stderr, err := executeHyardCLI(t, runtimeRoot, "remove", harnessID)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `remove target "`+harnessID+`" is ambiguous`)
	require.ErrorContains(t, err, `hyard remove orbit `+harnessID)
	require.ErrorContains(t, err, `hyard remove harness `+harnessID)
}

func TestHyardRemoveHarnessDryRunJSONListsOwnedOrbits(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLI(t, runtimeRoot, "remove", "harness", harnessID, "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		TargetType           string   `json:"target_type"`
		HarnessPackage       string   `json:"harness_package"`
		HarnessID            string   `json:"harness_id"`
		OrbitPackages        []string `json:"orbit_packages"`
		OrbitIDs             []string `json:"orbit_ids"`
		DryRun               bool     `json:"dry_run"`
		DeleteBundleRecord   bool     `json:"delete_bundle_record"`
		RemovedPaths         []string `json:"removed_paths"`
		RemovedPathCount     int      `json:"removed_path_count"`
		RemainingMemberCount int      `json:"remaining_member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "harness", payload.TargetType)
	require.Equal(t, harnessID, payload.HarnessPackage)
	require.Equal(t, harnessID, payload.HarnessID)
	require.Equal(t, []string{"docs"}, payload.OrbitPackages)
	require.Equal(t, []string{"docs"}, payload.OrbitIDs)
	require.True(t, payload.DryRun)
	require.True(t, payload.DeleteBundleRecord)
	require.Contains(t, payload.RemovedPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.RemovedPaths, "docs/guide.md")
	require.Equal(t, len(payload.RemovedPaths), payload.RemovedPathCount)
	require.Equal(t, 0, payload.RemainingMemberCount)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRoot)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
}

func TestHyardUninstallHarnessDryRunTextUsesUninstallLanguage(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLI(t, runtimeRoot, "uninstall", "harness", harnessID, "--dry-run")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Uninstall harness package "+harnessID+"?")
	require.Contains(t, stdout, "dry_run: true")
	require.Contains(t, stdout, "Orbits to uninstall:")
	require.Contains(t, stdout, "  - docs")
	require.NotContains(t, stdout, "Remove harness package")
	require.NotContains(t, stdout, "Orbits to remove:")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRoot)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
}

func TestHyardRemoveHarnessRequiresConfirmationAndDefaultNoCancels(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLIWithInput(t, runtimeRoot, "\n", "remove", "harness", harnessID)
	require.Error(t, err)
	require.Contains(t, stdout, "Remove harness package "+harnessID+"?")
	require.Contains(t, stdout, "Orbits to remove:")
	require.Contains(t, stdout, "  - docs")
	require.Contains(t, stderr, "Continue? [y/N]")
	require.ErrorContains(t, err, `remove canceled for harness package "`+harnessID+`"`)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRoot)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
}

func TestHyardUninstallHarnessRequiresConfirmationAndDefaultNoCancels(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLIWithInput(t, runtimeRoot, "\n", "uninstall", "harness", harnessID)
	require.Error(t, err)
	require.Contains(t, stdout, "Uninstall harness package "+harnessID+"?")
	require.Contains(t, stdout, "Orbits to uninstall:")
	require.Contains(t, stdout, "  - docs")
	require.NotContains(t, stdout, "Remove harness package")
	require.NotContains(t, stdout, "Orbits to remove:")
	require.Contains(t, stderr, "Continue? [y/N]")
	require.ErrorContains(t, err, `uninstall canceled for harness package "`+harnessID+`"`)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRoot)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
}

func TestHyardRemoveHarnessAppliesAfterConfirmation(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLIWithInput(t, runtimeRoot, "y\n", "remove", "harness", harnessID)
	require.NoError(t, err)
	require.Contains(t, stdout, "Remove harness package "+harnessID+"?")
	require.Contains(t, stdout, "removed harness package "+harnessID)
	require.Contains(t, stderr, "Continue? [y/N]")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRoot)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	_, err = harnesspkg.LoadBundleRecord(runtimeRoot, harnessID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file")

	_, err = os.Stat(filepath.Join(runtimeRoot, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardUninstallHarnessAppliesAfterConfirmation(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLIWithInput(t, runtimeRoot, "y\n", "uninstall", "harness", harnessID)
	require.NoError(t, err)
	require.Contains(t, stdout, "Uninstall harness package "+harnessID+"?")
	require.Contains(t, stdout, "uninstalled harness package "+harnessID)
	require.Contains(t, stdout, "uninstalled_orbits: docs")
	require.NotContains(t, stdout, "removed harness package")
	require.Contains(t, stderr, "Continue? [y/N]")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRoot)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	_, err = harnesspkg.LoadBundleRecord(runtimeRoot, harnessID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file")

	_, err = os.Stat(filepath.Join(runtimeRoot, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardRemoveHarnessYesSkipsPrompt(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLI(t, runtimeRoot, "remove", "harness", harnessID, "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		TargetType           string   `json:"target_type"`
		HarnessPackage       string   `json:"harness_package"`
		OrbitPackages        []string `json:"orbit_packages"`
		DryRun               bool     `json:"dry_run"`
		RemovedPaths         []string `json:"removed_paths"`
		RemainingMemberCount int      `json:"remaining_member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "harness", payload.TargetType)
	require.Equal(t, harnessID, payload.HarnessPackage)
	require.Equal(t, []string{"docs"}, payload.OrbitPackages)
	require.False(t, payload.DryRun)
	require.Contains(t, payload.RemovedPaths, ".harness/bundles/"+harnessID+".yaml")
	require.Equal(t, 0, payload.RemainingMemberCount)
}

func TestHyardUninstallHarnessYesJSONPreservesRemoveShapedPayload(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLI(t, runtimeRoot, "uninstall", "harness", harnessID, "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Action               string   `json:"action"`
		TargetType           string   `json:"target_type"`
		HarnessPackage       string   `json:"harness_package"`
		HarnessID            string   `json:"harness_id"`
		RemoveMode           string   `json:"remove_mode"`
		OrbitPackages        []string `json:"orbit_packages"`
		OrbitIDs             []string `json:"orbit_ids"`
		DryRun               bool     `json:"dry_run"`
		RemovedPaths         []string `json:"removed_paths"`
		RemovedPathCount     int      `json:"removed_path_count"`
		DeletedBundleRecord  bool     `json:"deleted_bundle_record"`
		RemainingMemberCount int      `json:"remaining_member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "uninstall", payload.Action)
	require.Equal(t, "harness", payload.TargetType)
	require.Equal(t, harnessID, payload.HarnessPackage)
	require.Equal(t, harnessID, payload.HarnessID)
	require.Equal(t, "harness_package_remove", payload.RemoveMode)
	require.Equal(t, []string{"docs"}, payload.OrbitPackages)
	require.Equal(t, []string{"docs"}, payload.OrbitIDs)
	require.False(t, payload.DryRun)
	require.Contains(t, payload.RemovedPaths, ".harness/bundles/"+harnessID+".yaml")
	require.Contains(t, payload.RemovedPaths, ".harness/orbits/docs.yaml")
	require.Equal(t, len(payload.RemovedPaths), payload.RemovedPathCount)
	require.True(t, payload.DeletedBundleRecord)
	require.Equal(t, 0, payload.RemainingMemberCount)
}

func TestHyardRemoveHarnessJSONApplyRequiresYes(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)

	stdout, stderr, err := executeHyardCLI(t, runtimeRoot, "remove", "harness", harnessID, "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "remove harness --json requires --yes or --dry-run")
}

func TestHyardRemoveHarnessFailsClosedWithDirtyTouchedPath(t *testing.T) {
	t.Parallel()

	runtimeRoot, harnessID := cloneHyardHarnessRuntime(t)
	require.NoError(t, os.WriteFile(filepath.Join(runtimeRoot, "docs", "guide.md"), []byte("dirty guide\n"), 0o644))

	stdout, stderr, err := executeHyardCLI(t, runtimeRoot, "remove", "harness", harnessID, "--yes")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `cannot remove harness package "`+harnessID+`" with uncommitted changes on touched paths`)
	require.ErrorContains(t, err, "docs/guide.md")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRoot)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
}

func cloneHyardHarnessRuntime(t *testing.T) (string, string) {
	t.Helper()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()
	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotEmpty(t, payload.HarnessRoot)
	require.NotEmpty(t, payload.HarnessID)
	commitHyardRuntimeRoot(t, payload.HarnessRoot, "commit cloned harness runtime")

	return payload.HarnessRoot, payload.HarnessID
}

func testRepoAtRoot(t *testing.T, root string) *testutil.Repo {
	t.Helper()

	return &testutil.Repo{Root: root}
}

func commitHyardRuntimeRoot(t *testing.T, root string, message string) {
	t.Helper()

	runGitForHyardRemoveTest(t, root, "config", "user.name", "Orbit Test")
	runGitForHyardRemoveTest(t, root, "config", "user.email", "orbit@example.com")
	runGitForHyardRemoveTest(t, root, "add", "-A")
	runGitForHyardRemoveTest(t, root, "commit", "-m", message)
}

func seedCommittedHyardInstallBackedRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := seedCommittedHyardRuntimeRepo(t)
	now := time.Date(2026, time.May, 5, 12, 0, 0, 0, time.UTC)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	runtimeFile.Members[0].Source = harnesspkg.MemberSourceInstallOrbit
	runtimeFile.Members[0].AddedAt = now
	runtimeFile.Harness.UpdatedAt = now

	_, err = harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFileFromRuntimeFile(runtimeFile))
	require.NoError(t, err)

	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: now,
	})
	require.NoError(t, err)

	repo.AddAndCommit(t, "mark docs as install-backed package")

	return repo
}

func runGitForHyardRemoveTest(t *testing.T, root string, args ...string) {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %v failed:\n%s", args, string(output))
}

func seedCommittedHyardRuntimeWithTwoAgentAddonHooks(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHyardCLI(t, repo.Root, "init", "runtime")
	require.NoError(t, err)

	writeHyardAgentAddonHookPackage(t, repo, "docs")
	writeHyardAgentAddonHookPackage(t, repo, "ops")
	repo.Run(t, "add", "-A")
	_, err = harnesspkg.AddManualMember(context.Background(), repo.Root, "docs", time.Date(2026, time.April, 26, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = harnesspkg.AddManualMember(context.Background(), repo.Root, "ops", time.Date(2026, time.April, 26, 12, 5, 0, 0, time.UTC))
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed package hooks")

	return repo
}

func writeHyardAgentAddonHookPackage(t *testing.T, repo *testutil.Repo, packageName string) {
	t.Helper()

	repo.WriteFile(t, ".harness/orbits/"+packageName+".yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: "+packageName+"\n"+
		"meta:\n"+
		"  file: .harness/orbits/"+packageName+".yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"agent_addons:\n"+
		"  hooks:\n"+
		"    unsupported_behavior: skip\n"+
		"    entries:\n"+
		"      - id: block-dangerous-shell\n"+
		"        required: false\n"+
		"        description: Block dangerous shell commands.\n"+
		"        event:\n"+
		"          kind: tool.before\n"+
		"        match:\n"+
		"          tools: [shell]\n"+
		"        handler:\n"+
		"          type: command\n"+
		"          path: hooks/"+packageName+"/block-dangerous-shell/run.sh\n"+
		"        targets:\n"+
		"          codex: true\n"+
		"members:\n"+
		"  - name: "+packageName+"-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - "+packageName+"/**\n"+
		"        - hooks/"+packageName+"/**\n")
	repo.WriteFile(t, packageName+"/guide.md", packageName+" guide\n")
	repo.WriteFile(t, "hooks/"+packageName+"/block-dangerous-shell/run.sh", "#!/bin/sh\nset -eu\ncat > hooks/"+packageName+"/block-dangerous-shell/captured.json\nprintf '{\"decision\":\"allow\"}\\n'\n")
}

func seedCommittedHyardRuntimeWithTwoSkillPackages(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHyardCLI(t, repo.Root, "init", "runtime")
	require.NoError(t, err)

	writeHyardSkillPackage(t, repo, "docs", "docs-style")
	writeHyardSkillPackage(t, repo, "ops", "ops-style")
	repo.Run(t, "add", "-A")
	_, err = harnesspkg.AddManualMember(context.Background(), repo.Root, "docs", time.Date(2026, time.April, 26, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = harnesspkg.AddManualMember(context.Background(), repo.Root, "ops", time.Date(2026, time.April, 26, 12, 5, 0, 0, time.UTC))
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed skill packages")

	return repo
}

func writeHyardSkillPackage(t *testing.T, repo *testutil.Repo, packageName string, skillName string) {
	t.Helper()

	repo.WriteFile(t, ".harness/orbits/"+packageName+".yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: "+packageName+"\n"+
		"meta:\n"+
		"  file: .harness/orbits/"+packageName+".yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - skills/"+packageName+"/*\n"+
		"members:\n"+
		"  - name: "+packageName+"-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - "+packageName+"/**\n")
	repo.WriteFile(t, packageName+"/guide.md", packageName+" guide\n")
	repo.WriteFile(t, "skills/"+packageName+"/"+skillName+"/SKILL.md", ""+
		"---\n"+
		"name: "+skillName+"\n"+
		"description: "+skillName+" references.\n"+
		"---\n"+
		"# "+skillName+"\n")
}
