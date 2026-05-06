package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type hyardContextKey string

const hyardWorkingDirContextKey hyardContextKey = "hyard_working_dir"
const hyardStartLauncherContextKey hyardContextKey = "hyard_start_launcher"

type cloneSourceJSON struct {
	Kind               string `json:"kind"`
	Repo               string `json:"repo"`
	RequestedRef       string `json:"requested_ref,omitempty"`
	ResolvedRef        string `json:"resolved_ref"`
	Commit             string `json:"commit"`
	PackageName        string `json:"package_name,omitempty"`
	PackageCoordinate  string `json:"package_coordinate,omitempty"`
	PackageLocatorKind string `json:"package_locator_kind,omitempty"`
	PackageLocator     string `json:"package_locator,omitempty"`
}

type cloneNextActionJSON struct {
	Kind             string `json:"kind"`
	Command          string `json:"command"`
	WorkingDirectory string `json:"working_directory"`
	Intent           string `json:"intent"`
}

type cloneResultJSON struct {
	HarnessRoot  string                     `json:"harness_root"`
	ManifestPath string                     `json:"manifest_path"`
	HarnessID    string                     `json:"harness_id"`
	Source       cloneSourceJSON            `json:"source"`
	MemberIDs    []string                   `json:"member_ids"`
	MemberCount  int                        `json:"member_count"`
	BundleCount  int                        `json:"bundle_count"`
	Readiness    harnesspkg.ReadinessReport `json:"readiness"`
	NextActions  []cloneNextActionJSON      `json:"next_actions"`
}

// WithWorkingDir injects the working directory used by hyard command tests.
func WithWorkingDir(ctx context.Context, workingDir string) context.Context {
	return context.WithValue(ctx, hyardWorkingDirContextKey, workingDir)
}

// WithStartLauncher injects the Harness Start launcher used by command tests.
func WithStartLauncher(ctx context.Context, launcher harnesspkg.StartLauncher) context.Context {
	return context.WithValue(ctx, hyardStartLauncherContextKey, launcher)
}

func newCloneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone <harness-template-source> [repo-name]",
		Short: "Bootstrap a new runtime repo from a harness template source",
		Long: "Bootstrap a new runtime repo from a harness template source.\n" +
			"This command is a controlled compose of runtime create plus harness template install,\n" +
			"reusing the existing runtime bootstrap, install preview, provenance, and readiness helpers.",
		Example: "" +
			"  hyard clone ../starter-template --ref harness-template/workspace\n" +
			"  hyard clone https://example.com/acme/starter.git demo --ref harness-template/workspace\n" +
			"  hyard clone git@github.com:acme/starter.git --path ../workspaces --ref harness-template/workspace\n",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceArg := strings.TrimSpace(args[0])
			if sourceArg == "" {
				return fmt.Errorf("harness template source must not be empty")
			}

			requestedRef, err := cmd.Flags().GetString("ref")
			if err != nil {
				return fmt.Errorf("read --ref flag: %w", err)
			}
			sourceMetadata := packageMetadata{}
			if shouldParseHyardPackageCoordinateArg(sourceArg) {
				coordinate, err := parseHyardPackageCoordinate(sourceArg)
				if err != nil {
					return err
				}
				if coordinate.Kind != ids.PackageCoordinateGitLocator {
					return fmt.Errorf("hyard clone package coordinate %s is not supported yet; use %s@git:<ref> or the existing explicit source form", coordinate.String(), coordinate.Name)
				}
				if cmd.Flags().Changed("ref") {
					return fmt.Errorf("package coordinate %s cannot be combined with --ref; put the git locator after @git", coordinate.String())
				}
				workingDir, err := hyardWorkingDirFromCommand(cmd)
				if err != nil {
					return err
				}
				locator := normalizePackageGitLocatorRef(coordinate.Locator)
				sourceArg = workingDir
				requestedRef = locator
				sourceMetadata = packageMetadataFromCoordinateWithLocator(coordinate, locator)
			}
			parentDirArg, err := cmd.Flags().GetString("path")
			if err != nil {
				return fmt.Errorf("read --path flag: %w", err)
			}
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read --json flag: %w", err)
			}

			candidate, source, err := resolveCloneTemplateSource(cmd.Context(), sourceArg, requestedRef)
			if err != nil {
				return err
			}

			repoName := ""
			if len(args) == 2 {
				repoName, err = validateExplicitCloneRepoName(args[1])
				if err != nil {
					return err
				}
			}
			if repoName == "" {
				repoName, err = inferCloneRepoName(candidate.RepoURL)
				if err != nil {
					return err
				}
			}

			parentDir, err := cloneParentDirFromCommand(cmd, parentDirArg)
			if err != nil {
				return err
			}
			targetRoot := filepath.Join(parentDir, repoName)

			bootstrap, err := createCloneRuntimeRepo(cmd.Context(), targetRoot, time.Now().UTC())
			if err != nil {
				return err
			}

			installSource := orbittemplate.Source{
				SourceKind:     orbittemplate.InstallSourceKindExternalGit,
				SourceRepo:     candidate.RepoURL,
				SourceRef:      candidate.Branch,
				TemplateCommit: source.Commit,
			}
			preview, err := harnesspkg.BuildTemplateInstallPreview(cmd.Context(), harnesspkg.TemplateInstallPreviewInput{
				RepoRoot:                bootstrap.Repo.Root,
				Source:                  source,
				InstallSource:           installSource,
				RequireResolvedBindings: true,
				Now:                     time.Now().UTC(),
			})
			if err != nil {
				return fmt.Errorf("build harness template install preview: %w", err)
			}

			result, err := harnesspkg.ApplyTemplateInstallPreview(cmd.Context(), bootstrap.Repo.Root, preview, false)
			if err != nil {
				return fmt.Errorf("install harness template: %w", err)
			}
			if _, err := harnesspkg.ApplyRunViewPresentationDefault(cmd.Context(), bootstrap.Repo.Root); err != nil {
				return fmt.Errorf("apply Run View presentation: %w", err)
			}

			readiness, err := harnesspkg.EvaluateRuntimeReadiness(cmd.Context(), bootstrap.Repo.Root)
			if err != nil {
				return fmt.Errorf("evaluate runtime readiness: %w", err)
			}

			output := cloneResultJSON{
				HarnessRoot:  bootstrap.Repo.Root,
				ManifestPath: bootstrap.Bootstrap.ManifestPath,
				HarnessID:    result.Preview.BundleRecord.HarnessID,
				Source: cloneSourceJSON{
					Kind:               orbittemplate.InstallSourceKindExternalGit,
					Repo:               candidate.RepoURL,
					RequestedRef:       strings.TrimSpace(requestedRef),
					ResolvedRef:        candidate.Branch,
					Commit:             source.Commit,
					PackageName:        sourceMetadata.name,
					PackageCoordinate:  sourceMetadata.coordinate,
					PackageLocatorKind: sourceMetadata.locatorKind,
					PackageLocator:     sourceMetadata.locator,
				},
				MemberIDs:   source.MemberIDs(),
				MemberCount: len(source.MemberIDs()),
				BundleCount: 1,
				Readiness:   readiness,
				NextActions: cloneHarnessStartNextActions(bootstrap.Repo.Root),
			}

			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"cloned harness template %s from %s into %s\n",
				candidate.Branch,
				candidate.RepoURL,
				bootstrap.Repo.Root,
			)
			if err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, action := range output.NextActions {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "next_action: cd %s\n", action.WorkingDirectory); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "next_action: %s\n", action.Command); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().String("ref", "", "Install one explicit harness template branch from the source repository")
	cmd.Flags().String("path", "", "Create the new runtime repo under this parent directory")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")

	return cmd
}

func cloneHarnessStartNextActions(harnessRoot string) []cloneNextActionJSON {
	return []cloneNextActionJSON{{
		Kind:             "harness_start",
		Command:          "hyard start",
		WorkingDirectory: harnessRoot,
		Intent:           "run Harness Start inside the cloned Harness Runtime",
	}}
}

type cloneBootstrapResult struct {
	Repo      gitpkg.Repo
	Bootstrap harnesspkg.BootstrapResult
}

func createCloneRuntimeRepo(ctx context.Context, targetPath string, now time.Time) (cloneBootstrapResult, error) {
	if err := os.MkdirAll(targetPath, 0o750); err != nil {
		return cloneBootstrapResult{}, fmt.Errorf("create target directory %s: %w", targetPath, err)
	}

	if _, err := gitpkg.EnsureRepoRoot(ctx, targetPath); err != nil {
		return cloneBootstrapResult{}, fmt.Errorf("ensure git repo root: %w", err)
	}

	repo, err := gitpkg.DiscoverRepo(ctx, targetPath)
	if err != nil {
		return cloneBootstrapResult{}, fmt.Errorf("discover git repository: %w", err)
	}
	if gitpkg.ComparablePath(repo.Root) != gitpkg.ComparablePath(targetPath) {
		return cloneBootstrapResult{}, fmt.Errorf("expected harness root %s to be a git repo root, got %s", targetPath, repo.Root)
	}

	manifestPath := harnesspkg.ManifestPath(repo.Root)
	if _, err := os.Stat(manifestPath); err == nil {
		manifest, loadErr := harnesspkg.LoadManifestFile(repo.Root)
		if loadErr == nil {
			if manifest.Kind != harnesspkg.ManifestKindRuntime {
				return cloneBootstrapResult{}, fmt.Errorf("harness already initialized with non-runtime manifest at %s", manifestPath)
			}
			return cloneBootstrapResult{}, fmt.Errorf("harness runtime already initialized at %s", manifestPath)
		}
		return cloneBootstrapResult{}, fmt.Errorf("load existing harness manifest: %w", loadErr)
	} else if !errors.Is(err, os.ErrNotExist) {
		return cloneBootstrapResult{}, fmt.Errorf("stat %s: %w", manifestPath, err)
	}

	bootstrap, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, now)
	if err != nil {
		return cloneBootstrapResult{}, fmt.Errorf("bootstrap harness control plane: %w", err)
	}

	return cloneBootstrapResult{
		Repo:      repo,
		Bootstrap: bootstrap,
	}, nil
}

func resolveCloneTemplateSource(
	ctx context.Context,
	sourceArg string,
	requestedRef string,
) (harnesspkg.RemoteTemplateInstallCandidate, harnesspkg.LocalTemplateInstallSource, error) {
	scratchRoot, err := os.MkdirTemp("", "hyard-clone-source-*")
	if err != nil {
		return harnesspkg.RemoteTemplateInstallCandidate{}, harnesspkg.LocalTemplateInstallSource{}, fmt.Errorf("create clone source scratch repo: %w", err)
	}
	defer os.RemoveAll(scratchRoot)

	if _, err := gitpkg.EnsureRepoRoot(ctx, scratchRoot); err != nil {
		return harnesspkg.RemoteTemplateInstallCandidate{}, harnesspkg.LocalTemplateInstallSource{}, fmt.Errorf("initialize clone source scratch repo: %w", err)
	}

	candidate, source, err := harnesspkg.ResolveRemoteTemplateInstallSource(ctx, scratchRoot, sourceArg, requestedRef)
	if err != nil {
		return harnesspkg.RemoteTemplateInstallCandidate{}, harnesspkg.LocalTemplateInstallSource{}, fmt.Errorf("resolve harness template source: %w", err)
	}

	return candidate, source, nil
}

func cloneParentDirFromCommand(cmd *cobra.Command, rawPath string) (string, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rawPath) == "" {
		return filepath.Clean(workingDir), nil
	}
	if filepath.IsAbs(rawPath) {
		return filepath.Clean(rawPath), nil
	}

	return filepath.Clean(filepath.Join(workingDir, rawPath)), nil
}

func hyardWorkingDirFromCommand(cmd *cobra.Command) (string, error) {
	if cmd.Context() != nil {
		if workingDir, ok := cmd.Context().Value(hyardWorkingDirContextKey).(string); ok && strings.TrimSpace(workingDir) != "" {
			return workingDir, nil
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	return workingDir, nil
}

func inferCloneRepoName(source string) (string, error) {
	trimmedSource := strings.TrimSpace(source)
	if trimmedSource == "" {
		return "", fmt.Errorf("clone could not infer a repo name from an empty source; provide [repo-name] explicitly")
	}

	base := ""
	if parsed, err := url.Parse(trimmedSource); err == nil && parsed.Scheme != "" {
		base = path.Base(strings.TrimRight(parsed.Path, "/"))
		if base == "." || base == "/" || base == "" {
			base = path.Base(strings.TrimRight(parsed.Opaque, "/"))
		}
	} else {
		trimmedSource = strings.TrimRight(trimmedSource, "/\\")
		if idx := strings.LastIndexAny(trimmedSource, "/:"); idx >= 0 {
			base = trimmedSource[idx+1:]
		} else {
			base = trimmedSource
		}
	}

	base = strings.TrimSuffix(strings.TrimSpace(base), ".git")
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("clone could not infer a stable repo name from source %q; provide [repo-name] explicitly", source)
	}

	return base, nil
}

func validateExplicitCloneRepoName(raw string) (string, error) {
	repoName := strings.TrimSpace(raw)
	if repoName == "" {
		return "", fmt.Errorf("clone [repo-name] must not be empty")
	}
	if repoName == "." || repoName == ".." || strings.ContainsAny(repoName, `/\`) {
		return "", fmt.Errorf("clone [repo-name] must be one leaf directory name")
	}

	return repoName, nil
}

func emitHyardJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}

	return nil
}
