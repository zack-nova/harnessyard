package orbittemplate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBuildRemoteTemplateApplyPreviewExplicitRefUsesSingleFetchFastPath(t *testing.T) {
	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Remote Orbit\n"), 0o600))

	commands := installGitCommandLogger(t)

	preview, err := BuildRemoteTemplateApplyPreview(context.Background(), RemoteTemplateApplyPreviewInput{
		RepoRoot:         runtimeRepo.Root,
		RemoteURL:        remoteURL,
		RequestedRef:     sourceRef,
		BindingsFilePath: bindingsPath,
		Now:              time.Date(2026, time.March, 21, 13, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, sourceRef, preview.Source.Ref)

	require.Equal(t, 1, countLoggedGitSubcommands(commands(), "fetch"))
	require.Zero(t, countLoggedGitSubcommands(commands(), "ls-remote"))
	require.Empty(t, runtimeRepo.Run(t, "for-each-ref", "--format=%(refname)", "refs/orbits/tmp/remote-source"))
}

func TestBuildRemoteBindingsInitPreviewExplicitRefUsesSingleFetchFastPath(t *testing.T) {
	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	commands := installGitCommandLogger(t)

	preview, err := BuildRemoteBindingsInitPreview(context.Background(), RemoteBindingsInitInput{
		RepoRoot:     runtimeRepo.Root,
		RemoteURL:    remoteURL,
		RequestedRef: sourceRef,
	})
	require.NoError(t, err)
	require.Equal(t, sourceRef, preview.Source.SourceRef)

	require.Equal(t, 1, countLoggedGitSubcommands(commands(), "fetch"))
	require.Zero(t, countLoggedGitSubcommands(commands(), "ls-remote"))
	require.Empty(t, runtimeRepo.Run(t, "for-each-ref", "--format=%(refname)", "refs/orbits/tmp/remote-source"))
}

func installGitCommandLogger(t *testing.T) func() []string {
	t.Helper()
	return installGitWrapper(t, "")
}

func installGitCommandLoggerWithFetchFailure(t *testing.T, failSubstring string) func() []string {
	t.Helper()

	return installGitWrapper(t, failSubstring)
}

func installGitWrapper(t *testing.T, failSubstring string) func() []string {
	t.Helper()

	realGit, err := exec.LookPath("git")
	require.NoError(t, err)

	wrapperDir := t.TempDir()
	logPath := filepath.Join(wrapperDir, "git.log")
	wrapperPath := filepath.Join(wrapperDir, "git")
	require.NoError(t, os.WriteFile(wrapperPath, []byte(""+
		"#!/bin/sh\n"+
		"printf '%s\\n' \"$*\" >> \"$GIT_WRAPPER_LOG\"\n"+
		"if [ -n \"$GIT_WRAPPER_FAIL_SUBSTRING\" ]; then\n"+
		"  case \"$*\" in\n"+
		"    *\" fetch \"*\"$GIT_WRAPPER_FAIL_SUBSTRING\"*)\n"+
		"      exit 1\n"+
		"      ;;\n"+
		"  esac\n"+
		"fi\n"+
		"exec \"$GIT_WRAPPER_REAL\" \"$@\"\n"), 0o755))

	t.Setenv("GIT_WRAPPER_LOG", logPath)
	t.Setenv("GIT_WRAPPER_REAL", realGit)
	t.Setenv("GIT_WRAPPER_FAIL_SUBSTRING", failSubstring)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return func() []string {
		data, err := os.ReadFile(logPath)
		if os.IsNotExist(err) {
			return nil
		}
		require.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			return nil
		}

		return lines
	}
}

func countLoggedGitSubcommands(commands []string, subcommand string) int {
	count := 0
	for _, command := range commands {
		if loggedGitSubcommand(command) == subcommand {
			count++
		}
	}

	return count
}

func loggedGitSubcommand(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	for index := 0; index < len(fields); index++ {
		if fields[index] == "-C" && index+1 < len(fields) {
			index++
			continue
		}
		if strings.HasPrefix(fields[index], "-") {
			continue
		}
		return fields[index]
	}

	return ""
}
