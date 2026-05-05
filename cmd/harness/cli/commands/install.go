package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type installSourceJSON struct {
	Kind               string `json:"kind"`
	Repo               string `json:"repo,omitempty"`
	Ref                string `json:"ref"`
	RequestedRef       string `json:"requested_ref,omitempty"`
	ResolvedRef        string `json:"resolved_ref,omitempty"`
	ResolutionKind     string `json:"resolution_kind,omitempty"`
	Commit             string `json:"commit"`
	PackageName        string `json:"package_name,omitempty"`
	PackageCoordinate  string `json:"package_coordinate,omitempty"`
	PackageLocatorKind string `json:"package_locator_kind,omitempty"`
	PackageLocator     string `json:"package_locator,omitempty"`
}

type installBindingJSON struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Namespace string `json:"namespace,omitempty"`
}

type installAgentAddonsJSON struct {
	Hooks []installAgentAddonHookJSON `json:"hooks,omitempty"`
}

type installAgentAddonHookJSON struct {
	OrbitID         string          `json:"orbit_id,omitempty"`
	Package         string          `json:"package"`
	ID              string          `json:"id"`
	DisplayID       string          `json:"display_id"`
	Required        bool            `json:"required,omitempty"`
	Description     string          `json:"description,omitempty"`
	EventKind       string          `json:"event_kind"`
	Tools           []string        `json:"tools,omitempty"`
	CommandPatterns []string        `json:"command_patterns,omitempty"`
	HandlerType     string          `json:"handler_type"`
	HandlerPath     string          `json:"handler_path"`
	HandlerDigest   string          `json:"handler_digest"`
	Targets         map[string]bool `json:"targets,omitempty"`
	Activation      string          `json:"activation"`
}

type installPreviewJSON struct {
	DryRun            bool                          `json:"dry_run"`
	HarnessRoot       string                        `json:"harness_root"`
	TemplateKind      string                        `json:"template_kind,omitempty"`
	OverwriteExisting bool                          `json:"overwrite_existing"`
	Source            installSourceJSON             `json:"source"`
	OrbitID           string                        `json:"orbit_id"`
	HarnessID         string                        `json:"harness_id,omitempty"`
	MemberIDs         []string                      `json:"member_ids,omitempty"`
	Bindings          []installBindingJSON          `json:"bindings"`
	AgentAddons       *installAgentAddonsJSON       `json:"agent_addons,omitempty"`
	Files             []string                      `json:"files"`
	Warnings          []string                      `json:"warnings,omitempty"`
	Conflicts         []orbittemplate.ApplyConflict `json:"conflicts"`
}

type installResultJSON struct {
	DryRun       bool                       `json:"dry_run"`
	HarnessRoot  string                     `json:"harness_root"`
	Source       installSourceJSON          `json:"source"`
	OrbitID      string                     `json:"orbit_id"`
	WrittenPaths []string                   `json:"written_paths"`
	Warnings     []string                   `json:"warnings,omitempty"`
	AgentAddons  *installAgentAddonsJSON    `json:"agent_addons,omitempty"`
	MemberCount  int                        `json:"member_count"`
	Readiness    harnesspkg.ReadinessReport `json:"readiness"`
}

type explicitRemoteTemplateRefKind uint8

const (
	explicitRemoteTemplateRefUnknown explicitRemoteTemplateRefKind = iota
	explicitRemoteTemplateRefOrbit
	explicitRemoteTemplateRefHarness
)

type harnessTemplateInstallResultJSON struct {
	DryRun       bool                       `json:"dry_run"`
	HarnessRoot  string                     `json:"harness_root"`
	TemplateKind string                     `json:"template_kind"`
	Source       installSourceJSON          `json:"source"`
	HarnessID    string                     `json:"harness_id"`
	MemberIDs    []string                   `json:"member_ids"`
	WrittenPaths []string                   `json:"written_paths"`
	Warnings     []string                   `json:"warnings,omitempty"`
	AgentAddons  *installAgentAddonsJSON    `json:"agent_addons,omitempty"`
	MemberCount  int                        `json:"member_count"`
	BundleCount  int                        `json:"bundle_count"`
	Readiness    harnesspkg.ReadinessReport `json:"readiness"`
}

type installTargetState struct {
	Member                 *harnesspkg.RuntimeMember
	HasDefinition          bool
	HasInstallRecord       bool
	ExistingRecord         orbittemplate.InstallRecord
	HasBundleRecord        bool
	ExistingBundleRecord   harnesspkg.BundleRecord
	RequiresOverwrite      bool
	RequiresBundleTransfer bool
}

// Test-only hook for deterministic cleanup-failure injection around overwrite rollback.
var beforeInstallOwnedCleanupHook func(repoRoot string, orbitID string, plan orbittemplate.InstallOwnedCleanupPlan)

const (
	installTemplateKindOrbit   = "orbit_template"
	installTemplateKindHarness = "harness_template"
)

// NewInstallCommand creates the harness install command.
func NewInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <template-branch|git-source>",
		Short: "Install one orbit template from the current repo or an external Git source",
		Long: "Install one orbit template from the current repo or an external Git source into the current harness runtime.\n" +
			"By default this command writes the runtime immediately; use --dry-run to preview without mutating the repository.",
		Example: "" +
			"  harness install orbit-template/docs --bindings .harness/vars.yaml\n" +
			"  harness install https://example.com/acme/templates.git --ref orbit-template/docs --bindings .harness/vars.yaml\n" +
			"  harness install orbit-template/docs --overwrite-existing --bindings .harness/vars.yaml --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			bindingsPath, err := cmd.Flags().GetString("bindings")
			if err != nil {
				return fmt.Errorf("read --bindings flag: %w", err)
			}
			dryRun, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return fmt.Errorf("read --dry-run flag: %w", err)
			}
			overwriteExisting, err := cmd.Flags().GetBool("overwrite-existing")
			if err != nil {
				return fmt.Errorf("read --overwrite-existing flag: %w", err)
			}
			overrideIDs, err := installOverrideIDsFromFlags(cmd)
			if err != nil {
				return err
			}
			allowUnresolvedBindings, err := allowUnresolvedBindingsFromFlags(cmd)
			if err != nil {
				return err
			}
			interactive, err := cmd.Flags().GetBool("interactive")
			if err != nil {
				return fmt.Errorf("read --interactive flag: %w", err)
			}
			editorMode, err := cmd.Flags().GetBool("editor")
			if err != nil {
				return fmt.Errorf("read --editor flag: %w", err)
			}
			requestedRef, err := cmd.Flags().GetString("ref")
			if err != nil {
				return fmt.Errorf("read --ref flag: %w", err)
			}
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			progressMode, err := cmd.Flags().GetString("progress")
			if err != nil {
				return fmt.Errorf("read --progress flag: %w", err)
			}
			progress, err := newInstallProgressEmitter(cmd.ErrOrStderr(), progressMode)
			if err != nil {
				return err
			}

			sourceArg := args[0]
			prompter := buildInstallPrompter(cmd, interactive)
			var editor orbittemplate.Editor
			if editorMode {
				editor, err = orbittemplate.NewEnvironmentEditor()
				if err != nil {
					return fmt.Errorf("configure bindings editor: %w", err)
				}
			}

			if err := stageProgress(progress, "resolving install source"); err != nil {
				return err
			}
			localSource, err := installSourceUsesLocalRevision(cmd.Context(), resolved.Repo.Root, sourceArg)
			if err != nil {
				return err
			}

			now := time.Now().UTC()
			activeInstallOrbitIDs := harnesspkg.ActiveInstallOrbitIDs(resolved.Runtime)
			if localSource {
				if strings.TrimSpace(requestedRef) != "" {
					return fmt.Errorf("--ref is only supported when installing from an external Git source")
				}

				harnessSource, err := harnesspkg.ResolveLocalTemplateInstallSource(cmd.Context(), resolved.Repo.Root, sourceArg)
				if err == nil {
					installSource := orbittemplate.Source{
						SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
						SourceRepo:     "",
						SourceRef:      harnessSource.Ref,
						TemplateCommit: harnessSource.Commit,
					}
					if err := stageProgress(progress, "resolving bindings"); err != nil {
						return err
					}
					if dryRun {
						preview, err := harnesspkg.BuildTemplateInstallPreview(cmd.Context(), harnesspkg.TemplateInstallPreviewInput{
							RepoRoot:                resolved.Repo.Root,
							Source:                  harnessSource,
							InstallSource:           installSource,
							BindingsFilePath:        bindingsPath,
							OverrideOrbitIDs:        overrideIDs,
							OverwriteExisting:       overwriteExisting,
							Interactive:             interactive,
							Prompter:                prompter,
							EditorMode:              editorMode,
							Editor:                  editor,
							RequireResolvedBindings: !allowUnresolvedBindings,
							Now:                     now,
						})
						if err != nil {
							return fmt.Errorf("build harness template install preview: %w", err)
						}
						if err := stageProgress(progress, "checking conflicts"); err != nil {
							return err
						}
						return emitHarnessTemplateInstallPreview(
							cmd,
							resolved.Repo.Root,
							installSourceJSON{
								Kind:   orbittemplate.InstallSourceKindLocalBranch,
								Ref:    harnessSource.Ref,
								Commit: harnessSource.Commit,
							},
							preview,
							jsonOutput,
						)
					}
					preview, err := harnesspkg.BuildTemplateInstallPreview(cmd.Context(), harnesspkg.TemplateInstallPreviewInput{
						RepoRoot:                resolved.Repo.Root,
						Source:                  harnessSource,
						InstallSource:           installSource,
						BindingsFilePath:        bindingsPath,
						OverrideOrbitIDs:        overrideIDs,
						OverwriteExisting:       overwriteExisting,
						Interactive:             interactive,
						Prompter:                prompter,
						EditorMode:              editorMode,
						Editor:                  editor,
						RequireResolvedBindings: !allowUnresolvedBindings,
						Now:                     now,
					})
					if err != nil {
						return fmt.Errorf("build harness template install preview: %w", err)
					}
					if err := stageProgress(progress, "checking conflicts"); err != nil {
						return err
					}
					if err := stageProgress(progress, "writing files"); err != nil {
						return err
					}
					result, err := harnesspkg.ApplyTemplateInstallPreview(cmd.Context(), resolved.Repo.Root, preview, overwriteExisting)
					if err != nil {
						return fmt.Errorf("install harness template: %w", err)
					}
					appendRunViewPresentationToHarnessTemplateInstallResult(cmd.Context(), resolved.Repo.Root, &result)
					if err := stageProgress(progress, "updating runtime metadata"); err != nil {
						return err
					}
					if err := stageProgress(progress, "install complete"); err != nil {
						return err
					}
					return emitHarnessTemplateInstallResult(cmd, resolved.Repo.Root, result, jsonOutput)
				}
				var notHarnessTemplateErr *harnesspkg.LocalTemplateInstallSourceNotFoundError
				if !errors.As(err, &notHarnessTemplateErr) {
					return fmt.Errorf("resolve local harness template source: %w", err)
				}

				previewInput := orbittemplate.TemplateApplyPreviewInput{
					RepoRoot:                resolved.Repo.Root,
					SourceRef:               sourceArg,
					BindingsFilePath:        bindingsPath,
					RuntimeInstallOrbitIDs:  activeInstallOrbitIDs,
					AllowUnresolvedBindings: allowUnresolvedBindings,
					Interactive:             interactive,
					Prompter:                prompter,
					EditorMode:              editorMode,
					Editor:                  editor,
					Now:                     now,
				}
				if err := stageProgress(progress, "resolving bindings"); err != nil {
					return err
				}
				preview, err := orbittemplate.BuildTemplateApplyPreview(cmd.Context(), previewInput)
				if err != nil {
					return fmt.Errorf("build harness install preview: %w", err)
				}
				if err := stageProgress(progress, "checking conflicts"); err != nil {
					return err
				}
				targetState, err := inspectInstallTargetState(resolved.Repo.Root, resolved.Runtime, preview.Source.Manifest.Template.OrbitID)
				if err != nil {
					return err
				}
				if err := validateOrbitInstallTargetState(targetState, preview.Source.Manifest.Template.OrbitID, preview.InstallRecord.Template, overwriteExisting, overrideIDs); err != nil {
					return err
				}
				var cleanupPlan orbittemplate.InstallOwnedCleanupPlan
				if targetState.RequiresOverwrite {
					cleanupPlan, err = orbittemplate.BuildInstallOwnedCleanupPlan(cmd.Context(), resolved.Repo.Root, targetState.ExistingRecord, preview)
					if err != nil {
						return fmt.Errorf("reconstruct existing install ownership: %w", err)
					}
				}
				bundleShrinkPlan, hasBundleShrinkPlan, err := buildBundleTransferShrinkPlan(
					cmd.Context(),
					resolved.Repo.Root,
					preview.Source.Manifest.Template.OrbitID,
					targetState,
					preview,
				)
				if err != nil {
					return fmt.Errorf("prepare bundle-backed transfer cleanup: %w", err)
				}
				effectiveOverwriteExisting := overwriteExisting || overrideRequested(overrideIDs, preview.Source.Manifest.Template.OrbitID)
				if dryRun {
					if err := stageProgress(progress, "install complete"); err != nil {
						return err
					}
					return emitInstallPreview(cmd, resolved.Repo.Root, preview, jsonOutput, overwriteExisting)
				}

				var bundleShrinkPlanPtr *harnesspkg.BundleMemberShrinkPlan
				if hasBundleShrinkPlan {
					bundleShrinkPlanPtr = &bundleShrinkPlan
				}
				transactionPaths := buildInstallTransactionPaths(resolved.Repo.Root, preview, cleanupPlan, bundleShrinkPlanPtr)
				previewInput.OverwriteExisting = effectiveOverwriteExisting
				previewInput.SkipSharedAgentsWrite = true
				if err := stageProgress(progress, "writing files"); err != nil {
					return err
				}
				tx, err := harnesspkg.BeginInstallTransaction(cmd.Context(), resolved.Repo.Root, transactionPaths)
				if err != nil {
					return fmt.Errorf("begin install transaction: %w", err)
				}
				rollbackOnError := func(cause error) error {
					if rollbackErr := tx.Rollback(); rollbackErr != nil {
						return errors.Join(cause, fmt.Errorf("rollback install transaction: %w", rollbackErr))
					}
					return cause
				}
				result, err := orbittemplate.ApplyLocalTemplate(cmd.Context(), orbittemplate.TemplateApplyInput{Preview: previewInput})
				if err != nil {
					return rollbackOnError(fmt.Errorf("install local template: %w", err))
				}
				if targetState.RequiresOverwrite {
					runBeforeInstallOwnedCleanupHook(resolved.Repo.Root, result.Preview.Source.Manifest.Template.OrbitID, cleanupPlan)
					removedPaths, err := orbittemplate.ApplyInstallOwnedCleanup(resolved.Repo.Root, result.Preview.Source.Manifest.Template.OrbitID, cleanupPlan)
					if err != nil {
						return rollbackOnError(fmt.Errorf("remove stale install-owned paths: %w", err))
					}
					result.WrittenPaths = append(result.WrittenPaths, removedPaths...)
				}
				if err := stageProgress(progress, "updating runtime metadata"); err != nil {
					return rollbackOnError(err)
				}
				memberResult, err := upsertInstallMemberForState(cmd.Context(), resolved.Repo.Root, result.Preview.Source.Manifest.Template.OrbitID, now, targetState)
				if err != nil {
					return rollbackOnError(fmt.Errorf("record install-backed member: %w", err))
				}
				if hasBundleShrinkPlan {
					removedPaths, err := harnesspkg.ApplyBundleMemberShrinkPlan(resolved.Repo.Root, bundleShrinkPlan)
					if err != nil {
						return rollbackOnError(fmt.Errorf("apply bundle-backed transfer cleanup: %w", err))
					}
					result.WrittenPaths = append(result.WrittenPaths, removedPaths...)
				}
				tx.Commit()
				guidanceOutcome := composeInstallScopedGuidance(cmd.Context(), resolved.Repo.Root, []string{result.Preview.Source.Manifest.Template.OrbitID}, effectiveOverwriteExisting, composeRuntimeGuidanceForInstall)
				result.Preview.Warnings = append(result.Preview.Warnings, guidanceOutcome.Warnings...)
				appendInstallGuidancePaths(resolved.Repo.Root, result.Preview, &result, guidanceOutcome.WrittenPaths)
				if err := stageProgress(progress, "install complete"); err != nil {
					return err
				}
				return emitInstallResult(cmd, resolved.Repo.Root, result, memberResult.ManifestPath, memberResult.Runtime, jsonOutput)
			}

			previewInput := orbittemplate.RemoteTemplateApplyPreviewInput{
				RepoRoot:                resolved.Repo.Root,
				RemoteURL:               sourceArg,
				RequestedRef:            requestedRef,
				BindingsFilePath:        bindingsPath,
				RuntimeInstallOrbitIDs:  activeInstallOrbitIDs,
				AllowUnresolvedBindings: allowUnresolvedBindings,
				Interactive:             interactive,
				Prompter:                prompter,
				EditorMode:              editorMode,
				Editor:                  editor,
				Now:                     now,
			}
			if err := preflightRemoteInstallLocalInputs(cmd.Context(), resolved.Repo.Root, bindingsPath); err != nil {
				return err
			}
			if err := preflightExplicitRemoteOrbitInstallConflict(
				resolved.Repo.Root,
				resolved.Runtime,
				sourceArg,
				requestedRef,
				overwriteExisting,
				overrideIDs,
			); err != nil {
				return err
			}
			requestedRefKind := classifyExplicitRemoteTemplateRef(requestedRef)
			if strings.TrimSpace(requestedRef) == "" {
				if err := stageProgress(progress, "resolving external template candidates"); err != nil {
					return err
				}
			}
			if err := stageProgress(progress, "fetching selected template"); err != nil {
				return err
			}
			if strings.TrimSpace(requestedRef) != "" && requestedRefKind != explicitRemoteTemplateRefOrbit {
				harnessCandidate, harnessSource, harnessErr := harnesspkg.ResolveRemoteTemplateInstallSource(
					cmd.Context(),
					resolved.Repo.Root,
					sourceArg,
					requestedRef,
				)
				if harnessErr == nil {
					installSource := orbittemplate.Source{
						SourceKind:     orbittemplate.InstallSourceKindExternalGit,
						SourceRepo:     harnessCandidate.RepoURL,
						SourceRef:      harnessCandidate.Branch,
						TemplateCommit: harnessSource.Commit,
					}
					if err := stageProgress(progress, "resolving bindings"); err != nil {
						return err
					}
					if dryRun {
						preview, err := harnesspkg.BuildTemplateInstallPreview(cmd.Context(), harnesspkg.TemplateInstallPreviewInput{
							RepoRoot:                resolved.Repo.Root,
							Source:                  harnessSource,
							InstallSource:           installSource,
							BindingsFilePath:        bindingsPath,
							OverrideOrbitIDs:        overrideIDs,
							OverwriteExisting:       overwriteExisting,
							Interactive:             interactive,
							Prompter:                prompter,
							EditorMode:              editorMode,
							Editor:                  editor,
							RequireResolvedBindings: !allowUnresolvedBindings,
							Now:                     now,
						})
						if err != nil {
							return fmt.Errorf("build harness template install preview: %w", err)
						}
						if err := stageProgress(progress, "checking conflicts"); err != nil {
							return err
						}
						if err := stageProgress(progress, "install complete"); err != nil {
							return err
						}
						return emitHarnessTemplateInstallPreview(
							cmd,
							resolved.Repo.Root,
							installSourceJSON{
								Kind:   orbittemplate.InstallSourceKindExternalGit,
								Repo:   harnessCandidate.RepoURL,
								Ref:    harnessCandidate.Branch,
								Commit: harnessSource.Commit,
							},
							preview,
							jsonOutput,
						)
					}
					preview, err := harnesspkg.BuildTemplateInstallPreview(cmd.Context(), harnesspkg.TemplateInstallPreviewInput{
						RepoRoot:                resolved.Repo.Root,
						Source:                  harnessSource,
						InstallSource:           installSource,
						BindingsFilePath:        bindingsPath,
						OverrideOrbitIDs:        overrideIDs,
						OverwriteExisting:       overwriteExisting,
						Interactive:             interactive,
						Prompter:                prompter,
						EditorMode:              editorMode,
						Editor:                  editor,
						RequireResolvedBindings: !allowUnresolvedBindings,
						Now:                     now,
					})
					if err != nil {
						return fmt.Errorf("build harness template install preview: %w", err)
					}
					if err := stageProgress(progress, "checking conflicts"); err != nil {
						return err
					}
					if err := stageProgress(progress, "writing files"); err != nil {
						return err
					}
					result, err := harnesspkg.ApplyTemplateInstallPreview(cmd.Context(), resolved.Repo.Root, preview, overwriteExisting)
					if err != nil {
						return fmt.Errorf("install harness template: %w", err)
					}
					appendRunViewPresentationToHarnessTemplateInstallResult(cmd.Context(), resolved.Repo.Root, &result)
					if err := stageProgress(progress, "updating runtime metadata"); err != nil {
						return err
					}
					if err := stageProgress(progress, "install complete"); err != nil {
						return err
					}
					return emitHarnessTemplateInstallResult(cmd, resolved.Repo.Root, result, jsonOutput)
				}
				var notHarnessTemplateErr *harnesspkg.RemoteTemplateInstallNotFoundError
				if !errors.As(harnessErr, &notHarnessTemplateErr) {
					return fmt.Errorf("resolve remote harness template source: %w", harnessErr)
				}
			}

			preview, err := orbittemplate.BuildRemoteTemplateApplyPreview(cmd.Context(), previewInput)
			if err != nil {
				var notOrbitTemplateErr *orbittemplate.RemoteTemplateNotFoundError
				if strings.TrimSpace(requestedRef) == "" && errors.As(err, &notOrbitTemplateErr) {
					harnessCandidate, harnessSource, harnessErr := harnesspkg.ResolveRemoteTemplateInstallSource(
						cmd.Context(),
						resolved.Repo.Root,
						sourceArg,
						"",
					)
					if harnessErr == nil {
						installSource := orbittemplate.Source{
							SourceKind:     orbittemplate.InstallSourceKindExternalGit,
							SourceRepo:     harnessCandidate.RepoURL,
							SourceRef:      harnessCandidate.Branch,
							TemplateCommit: harnessSource.Commit,
						}
						if dryRun {
							preview, err := harnesspkg.BuildTemplateInstallPreview(cmd.Context(), harnesspkg.TemplateInstallPreviewInput{
								RepoRoot:                resolved.Repo.Root,
								Source:                  harnessSource,
								InstallSource:           installSource,
								BindingsFilePath:        bindingsPath,
								OverwriteExisting:       overwriteExisting,
								Interactive:             interactive,
								Prompter:                prompter,
								EditorMode:              editorMode,
								Editor:                  editor,
								RequireResolvedBindings: !allowUnresolvedBindings,
								Now:                     now,
							})
							if err != nil {
								return fmt.Errorf("build harness template install preview: %w", err)
							}
							return emitHarnessTemplateInstallPreview(
								cmd,
								resolved.Repo.Root,
								installSourceJSON{
									Kind:   orbittemplate.InstallSourceKindExternalGit,
									Repo:   harnessCandidate.RepoURL,
									Ref:    harnessCandidate.Branch,
									Commit: harnessSource.Commit,
								},
								preview,
								jsonOutput,
							)
						}
						preview, err := harnesspkg.BuildTemplateInstallPreview(cmd.Context(), harnesspkg.TemplateInstallPreviewInput{
							RepoRoot:                resolved.Repo.Root,
							Source:                  harnessSource,
							InstallSource:           installSource,
							BindingsFilePath:        bindingsPath,
							OverwriteExisting:       overwriteExisting,
							Interactive:             interactive,
							Prompter:                prompter,
							EditorMode:              editorMode,
							Editor:                  editor,
							RequireResolvedBindings: !allowUnresolvedBindings,
							Now:                     now,
						})
						if err != nil {
							return fmt.Errorf("build harness template install preview: %w", err)
						}
						result, err := harnesspkg.ApplyTemplateInstallPreview(cmd.Context(), resolved.Repo.Root, preview, overwriteExisting)
						if err != nil {
							return fmt.Errorf("install harness template: %w", err)
						}
						appendRunViewPresentationToHarnessTemplateInstallResult(cmd.Context(), resolved.Repo.Root, &result)
						return emitHarnessTemplateInstallResult(cmd, resolved.Repo.Root, result, jsonOutput)
					}
				}
				return fmt.Errorf("build harness install preview: %w", err)
			}
			if preview.RemoteResolutionKind == orbittemplate.RemoteTemplateResolutionSourceAlias {
				if err := stageProgress(progress, "source branch detected; resolving published template"); err != nil {
					return err
				}
			}
			if err := stageProgress(progress, "resolving bindings"); err != nil {
				return err
			}
			if err := stageProgress(progress, "checking conflicts"); err != nil {
				return err
			}
			targetState, err := inspectInstallTargetState(resolved.Repo.Root, resolved.Runtime, preview.Source.Manifest.Template.OrbitID)
			if err != nil {
				return err
			}
			if err := validateOrbitInstallTargetState(targetState, preview.Source.Manifest.Template.OrbitID, preview.InstallRecord.Template, overwriteExisting, overrideIDs); err != nil {
				return err
			}
			var cleanupPlan orbittemplate.InstallOwnedCleanupPlan
			if targetState.RequiresOverwrite {
				cleanupPlan, err = orbittemplate.BuildInstallOwnedCleanupPlan(cmd.Context(), resolved.Repo.Root, targetState.ExistingRecord, preview)
				if err != nil {
					return fmt.Errorf("reconstruct existing install ownership: %w", err)
				}
			}
			bundleShrinkPlan, hasBundleShrinkPlan, err := buildBundleTransferShrinkPlan(
				cmd.Context(),
				resolved.Repo.Root,
				preview.Source.Manifest.Template.OrbitID,
				targetState,
				preview,
			)
			if err != nil {
				return fmt.Errorf("prepare bundle-backed transfer cleanup: %w", err)
			}
			effectiveOverwriteExisting := overwriteExisting || overrideRequested(overrideIDs, preview.Source.Manifest.Template.OrbitID)
			if dryRun {
				if err := stageProgress(progress, "install complete"); err != nil {
					return err
				}
				return emitInstallPreview(cmd, resolved.Repo.Root, preview, jsonOutput, overwriteExisting)
			}

			var bundleShrinkPlanPtr *harnesspkg.BundleMemberShrinkPlan
			if hasBundleShrinkPlan {
				bundleShrinkPlanPtr = &bundleShrinkPlan
			}
			transactionPaths := buildInstallTransactionPaths(resolved.Repo.Root, preview, cleanupPlan, bundleShrinkPlanPtr)
			previewInput.OverwriteExisting = effectiveOverwriteExisting
			previewInput.SkipSharedAgentsWrite = true
			if err := stageProgress(progress, "writing files"); err != nil {
				return err
			}
			tx, err := harnesspkg.BeginInstallTransaction(cmd.Context(), resolved.Repo.Root, transactionPaths)
			if err != nil {
				return fmt.Errorf("begin install transaction: %w", err)
			}
			rollbackOnError := func(cause error) error {
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					return errors.Join(cause, fmt.Errorf("rollback install transaction: %w", rollbackErr))
				}
				return cause
			}
			result, err := orbittemplate.ApplyRemoteTemplate(cmd.Context(), orbittemplate.RemoteTemplateApplyInput{Preview: previewInput})
			if err != nil {
				return rollbackOnError(fmt.Errorf("install external template: %w", err))
			}
			if targetState.RequiresOverwrite {
				runBeforeInstallOwnedCleanupHook(resolved.Repo.Root, result.Preview.Source.Manifest.Template.OrbitID, cleanupPlan)
				removedPaths, err := orbittemplate.ApplyInstallOwnedCleanup(resolved.Repo.Root, result.Preview.Source.Manifest.Template.OrbitID, cleanupPlan)
				if err != nil {
					return rollbackOnError(fmt.Errorf("remove stale install-owned paths: %w", err))
				}
				result.WrittenPaths = append(result.WrittenPaths, removedPaths...)
			}
			if err := stageProgress(progress, "updating runtime metadata"); err != nil {
				return rollbackOnError(err)
			}
			memberResult, err := upsertInstallMemberForState(cmd.Context(), resolved.Repo.Root, result.Preview.Source.Manifest.Template.OrbitID, now, targetState)
			if err != nil {
				return rollbackOnError(fmt.Errorf("record install-backed member: %w", err))
			}
			if hasBundleShrinkPlan {
				removedPaths, err := harnesspkg.ApplyBundleMemberShrinkPlan(resolved.Repo.Root, bundleShrinkPlan)
				if err != nil {
					return rollbackOnError(fmt.Errorf("apply bundle-backed transfer cleanup: %w", err))
				}
				result.WrittenPaths = append(result.WrittenPaths, removedPaths...)
			}
			tx.Commit()
			guidanceOutcome := composeInstallScopedGuidance(cmd.Context(), resolved.Repo.Root, []string{result.Preview.Source.Manifest.Template.OrbitID}, effectiveOverwriteExisting, composeRuntimeGuidanceForInstall)
			result.Preview.Warnings = append(result.Preview.Warnings, guidanceOutcome.Warnings...)
			appendInstallGuidancePaths(resolved.Repo.Root, result.Preview, &result, guidanceOutcome.WrittenPaths)
			if err := stageProgress(progress, "install complete"); err != nil {
				return err
			}
			return emitInstallResult(cmd, resolved.Repo.Root, result, memberResult.ManifestPath, memberResult.Runtime, jsonOutput)
		},
	}

	cmd.Flags().String("bindings", "", "Path to an explicit bindings YAML file")
	cmd.Flags().String("ref", "", "Select one template branch explicitly when installing from an external Git source")
	cmd.Flags().Bool("overwrite-existing", false, "Allow overwriting an existing install-backed orbit and removing stale install-owned files")
	cmd.Flags().StringSlice("override", nil, "Explicitly transfer ownership of one or more existing orbit members from another install unit")
	cmd.Flags().Bool("allow-unresolved-bindings", false, "Compatibility no-op: unresolved required bindings are preserved by default")
	cmd.Flags().Bool("strict-bindings", false, "Fail when required bindings are unresolved instead of preserving placeholders")
	cmd.Flags().Bool("dry-run", false, "Preview harness install without writing files")
	addProgressFlag(cmd)
	cmd.Flags().Bool("interactive", false, "Prompt for missing bindings interactively")
	cmd.Flags().Bool("editor", false, "Open an editor-backed bindings skeleton for missing required values")
	addInstallPackageMetadataFlags(cmd)
	addPathFlag(cmd)
	addJSONFlag(cmd)
	cmd.AddCommand(NewInstallBatchCommand())

	return cmd
}

type installPackageMetadata struct {
	Name        string
	Coordinate  string
	LocatorKind string
	Locator     string
}

func addInstallPackageMetadataFlags(cmd *cobra.Command) {
	cmd.Flags().String("package-name", "", "User-facing package name for install output")
	cmd.Flags().String("package-version", "", "User-facing package version for install output")
	cmd.Flags().String("package-publish-kind", "", "User-facing package publish kind for install output")
	cmd.Flags().String("package-coordinate", "", "User-facing package coordinate for install output")
	cmd.Flags().String("package-locator-kind", "", "User-facing package locator kind for install output")
	cmd.Flags().String("package-locator", "", "User-facing package locator for install output")
	for _, flagName := range []string{"package-name", "package-version", "package-publish-kind", "package-coordinate", "package-locator-kind", "package-locator"} {
		if err := cmd.Flags().MarkHidden(flagName); err != nil {
			panic(err)
		}
	}
}

func readInstallPackageMetadata(cmd *cobra.Command) (installPackageMetadata, error) {
	name, err := cmd.Flags().GetString("package-name")
	if err != nil {
		return installPackageMetadata{}, fmt.Errorf("read --package-name flag: %w", err)
	}
	coordinate, err := cmd.Flags().GetString("package-coordinate")
	if err != nil {
		return installPackageMetadata{}, fmt.Errorf("read --package-coordinate flag: %w", err)
	}
	locatorKind, err := cmd.Flags().GetString("package-locator-kind")
	if err != nil {
		return installPackageMetadata{}, fmt.Errorf("read --package-locator-kind flag: %w", err)
	}
	locator, err := cmd.Flags().GetString("package-locator")
	if err != nil {
		return installPackageMetadata{}, fmt.Errorf("read --package-locator flag: %w", err)
	}

	return installPackageMetadata{
		Name:        name,
		Coordinate:  coordinate,
		LocatorKind: locatorKind,
		Locator:     locator,
	}, nil
}

func allowUnresolvedBindingsFromFlags(cmd *cobra.Command) (bool, error) {
	allowUnresolvedBindings, err := cmd.Flags().GetBool("allow-unresolved-bindings")
	if err != nil {
		return false, fmt.Errorf("read --allow-unresolved-bindings flag: %w", err)
	}
	strictBindings, err := cmd.Flags().GetBool("strict-bindings")
	if err != nil {
		return false, fmt.Errorf("read --strict-bindings flag: %w", err)
	}
	if allowUnresolvedBindings && strictBindings {
		return false, fmt.Errorf("--strict-bindings cannot be used with --allow-unresolved-bindings")
	}
	return !strictBindings, nil
}

func installOverrideIDsFromFlags(cmd *cobra.Command) (map[string]struct{}, error) {
	values, err := cmd.Flags().GetStringSlice("override")
	if err != nil {
		return nil, fmt.Errorf("read --override flag: %w", err)
	}

	overrideIDs := make(map[string]struct{}, len(values))
	for _, value := range values {
		orbitID := strings.TrimSpace(value)
		if err := ids.ValidateOrbitID(orbitID); err != nil {
			return nil, fmt.Errorf("validate --override value %q: %w", value, err)
		}
		overrideIDs[orbitID] = struct{}{}
	}

	return overrideIDs, nil
}

func installSourceUsesLocalRevision(ctx context.Context, repoRoot string, source string) (bool, error) {
	exists, err := gitpkg.RevisionExists(ctx, repoRoot, source)
	if err != nil {
		return false, fmt.Errorf("check local template source %q: %w", source, err)
	}

	return exists, nil
}

func preflightExplicitRemoteOrbitInstallConflict(
	repoRoot string,
	runtimeFile harnesspkg.RuntimeFile,
	remoteURL string,
	requestedRef string,
	overwriteExisting bool,
	overrideIDs map[string]struct{},
) error {
	orbitID, ok := inferRemoteOrbitInstallTargetID(requestedRef)
	if !ok {
		return nil
	}

	targetState, err := inspectInstallTargetState(repoRoot, runtimeFile, orbitID)
	if err != nil {
		return err
	}
	if err := validateOrbitInstallTargetState(targetState, orbitID, orbittemplate.Source{
		SourceKind: orbittemplate.InstallSourceKindExternalGit,
		SourceRepo: remoteURL,
		SourceRef:  normalizeExplicitRemoteTemplateRef(requestedRef),
	}, overwriteExisting, overrideIDs); err != nil {
		return err
	}

	return nil
}

func inferRemoteOrbitInstallTargetID(requestedRef string) (string, bool) {
	trimmedRef := normalizeExplicitRemoteTemplateRef(requestedRef)
	if !strings.HasPrefix(trimmedRef, "orbit-template/") {
		return "", false
	}

	orbitID := strings.TrimPrefix(trimmedRef, "orbit-template/")
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", false
	}

	return orbitID, true
}

func classifyExplicitRemoteTemplateRef(requestedRef string) explicitRemoteTemplateRefKind {
	trimmedRef := normalizeExplicitRemoteTemplateRef(requestedRef)
	switch {
	case strings.HasPrefix(trimmedRef, "orbit-template/"):
		return explicitRemoteTemplateRefOrbit
	case strings.HasPrefix(trimmedRef, "harness-template/"):
		return explicitRemoteTemplateRefHarness
	default:
		return explicitRemoteTemplateRefUnknown
	}
}

func normalizeExplicitRemoteTemplateRef(requestedRef string) string {
	trimmedRef := strings.TrimSpace(requestedRef)
	return strings.TrimPrefix(trimmedRef, "refs/heads/")
}

func preflightRemoteInstallLocalInputs(ctx context.Context, repoRoot string, bindingsPath string) error {
	if err := orbittemplate.PreflightRemoteTemplateApplyLocalInputs(ctx, repoRoot, bindingsPath); err != nil {
		return fmt.Errorf("preflight remote install inputs: %w", err)
	}

	return nil
}

func inspectInstallTargetState(repoRoot string, runtimeFile harnesspkg.RuntimeFile, orbitID string) (installTargetState, error) {
	state := installTargetState{}
	for _, member := range runtimeFile.Members {
		if member.OrbitID != orbitID {
			continue
		}
		memberCopy := member
		state.Member = &memberCopy
		break
	}
	if state.Member != nil && state.Member.Source == harnesspkg.MemberSourceInstallBundle {
		if strings.TrimSpace(state.Member.OwnerHarnessID) == "" {
			return installTargetState{}, fmt.Errorf("bundle-backed member %q is missing owner_harness_id", orbitID)
		}
		record, err := harnesspkg.LoadBundleRecord(repoRoot, state.Member.OwnerHarnessID)
		if err != nil {
			return installTargetState{}, fmt.Errorf(
				"load existing bundle record for orbit %q owner_harness_id %q: %w",
				orbitID,
				state.Member.OwnerHarnessID,
				err,
			)
		}
		state.HasBundleRecord = true
		state.ExistingBundleRecord = record
		state.RequiresBundleTransfer = true
	}

	definitionPath, err := orbitpkg.HostedDefinitionPath(repoRoot, orbitID)
	if err != nil {
		return installTargetState{}, fmt.Errorf("build orbit definition path: %w", err)
	}
	if _, err := os.Stat(definitionPath); err == nil {
		state.HasDefinition = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return installTargetState{}, fmt.Errorf("stat %s: %w", definitionPath, err)
	}

	installPath, err := harnesspkg.InstallRecordPath(repoRoot, orbitID)
	if err != nil {
		return installTargetState{}, fmt.Errorf("build install record path: %w", err)
	}
	record, loadErr := harnesspkg.LoadInstallRecord(repoRoot, orbitID)
	if loadErr == nil {
		state.HasInstallRecord = true
		state.ExistingRecord = record
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		return installTargetState{}, fmt.Errorf("load existing install record for orbit %q: %w", orbitID, loadErr)
	} else if _, err := os.Stat(installPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return installTargetState{}, fmt.Errorf("stat %s: %w", installPath, err)
	}

	state.RequiresOverwrite = state.HasInstallRecord

	return state, nil
}

func validateInstallTargetState(state installTargetState, orbitID string, overwriteExisting bool) error {
	if state.Member != nil {
		switch state.Member.Source {
		case harnesspkg.MemberSourceManual:
			return fmt.Errorf("orbit %q is already present as a manual member", orbitID)
		case harnesspkg.MemberSourceInstallOrbit:
			if !overwriteExisting {
				return fmt.Errorf("orbit %q is already installed; repeated install requires --overwrite-existing", orbitID)
			}
		default:
			return fmt.Errorf("orbit %q already exists in harness runtime", orbitID)
		}
	}

	if state.HasInstallRecord {
		if orbittemplate.EffectiveInstallRecordStatus(state.ExistingRecord) == orbittemplate.InstallRecordStatusDetached {
			if !overwriteExisting {
				return fmt.Errorf("orbit %q is detached; reinstall requires --overwrite-existing", orbitID)
			}
			return nil
		}
		if !overwriteExisting {
			return fmt.Errorf("orbit %q is already installed; repeated install requires --overwrite-existing", orbitID)
		}
		return nil
	}

	if state.HasDefinition {
		return fmt.Errorf("orbit definition %q already exists in the runtime repository", orbitID)
	}

	return nil
}

func validateOrbitInstallTargetState(
	state installTargetState,
	orbitID string,
	incomingSource orbittemplate.Source,
	overwriteExisting bool,
	overrideIDs map[string]struct{},
) error {
	if state.Member != nil {
		switch state.Member.Source {
		case harnesspkg.MemberSourceManual:
			return fmt.Errorf("orbit %q is already present as a manual member", orbitID)
		case harnesspkg.MemberSourceInstallOrbit:
			if state.HasInstallRecord && !sameInstallUnit(state.ExistingRecord.Template, incomingSource) {
				if !overrideRequested(overrideIDs, orbitID) {
					return fmt.Errorf("orbit %q is already installed from %q; cross-install override requires --override %s", orbitID, state.ExistingRecord.Template.SourceRef, orbitID)
				}
				return nil
			}
			if !overwriteExisting {
				return fmt.Errorf("orbit %q is already installed; repeated install requires --overwrite-existing", orbitID)
			}
			return nil
		case harnesspkg.MemberSourceInstallBundle:
			if !overrideRequested(overrideIDs, orbitID) {
				return fmt.Errorf(
					"orbit %q is already bundle-owned by harness %q; cross-install override requires --override %s",
					orbitID,
					state.Member.OwnerHarnessID,
					orbitID,
				)
			}
			return nil
		default:
			return fmt.Errorf("orbit %q already exists in harness runtime", orbitID)
		}
	}

	if state.HasInstallRecord {
		if orbittemplate.EffectiveInstallRecordStatus(state.ExistingRecord) == orbittemplate.InstallRecordStatusDetached {
			if !overwriteExisting {
				return fmt.Errorf("orbit %q is detached; reinstall requires --overwrite-existing", orbitID)
			}
			return nil
		}
		if !sameInstallUnit(state.ExistingRecord.Template, incomingSource) {
			if !overrideRequested(overrideIDs, orbitID) {
				return fmt.Errorf("orbit %q is already installed from %q; cross-install override requires --override %s", orbitID, state.ExistingRecord.Template.SourceRef, orbitID)
			}
			return nil
		}
		if !overwriteExisting {
			return fmt.Errorf("orbit %q is already installed; repeated install requires --overwrite-existing", orbitID)
		}
		return nil
	}

	if state.HasDefinition {
		return fmt.Errorf("orbit definition %q already exists in the runtime repository", orbitID)
	}

	return nil
}

func overrideRequested(overrideIDs map[string]struct{}, orbitID string) bool {
	_, ok := overrideIDs[orbitID]
	return ok
}

func sameInstallUnit(existing orbittemplate.Source, incoming orbittemplate.Source) bool {
	return normalizedInstallSourceKind(existing.SourceKind) == normalizedInstallSourceKind(incoming.SourceKind) &&
		normalizedInstallSourceRepo(existing.SourceRepo) == normalizedInstallSourceRepo(incoming.SourceRepo) &&
		normalizedInstallSourceRef(existing.SourceRef) == normalizedInstallSourceRef(incoming.SourceRef)
}

func normalizedInstallSourceKind(value string) string {
	return strings.TrimSpace(value)
}

func normalizedInstallSourceRepo(value string) string {
	return strings.TrimSpace(value)
}

func normalizedInstallSourceRef(value string) string {
	trimmed := strings.TrimSpace(value)
	return strings.TrimPrefix(trimmed, "refs/heads/")
}

func upsertInstallMemberForState(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	now time.Time,
	state installTargetState,
) (harnesspkg.MutateMembersResult, error) {
	if state.RequiresBundleTransfer {
		result, err := harnesspkg.ReplaceMemberWithInstall(ctx, repoRoot, orbitID, now)
		if err != nil {
			return harnesspkg.MutateMembersResult{}, fmt.Errorf("replace bundle-backed member with install-backed member: %w", err)
		}
		return result, nil
	}
	if state.RequiresOverwrite {
		result, err := harnesspkg.UpsertInstallMember(ctx, repoRoot, orbitID, now)
		if err != nil {
			return harnesspkg.MutateMembersResult{}, fmt.Errorf("upsert install-backed member: %w", err)
		}
		return result, nil
	}

	result, err := harnesspkg.AddInstallMember(ctx, repoRoot, orbitID, now)
	if err != nil {
		return harnesspkg.MutateMembersResult{}, fmt.Errorf("add install-backed member: %w", err)
	}

	return result, nil
}

func buildInstallPrompter(cmd *cobra.Command, interactive bool) orbittemplate.BindingPrompter {
	if !interactive {
		return nil
	}

	return orbittemplate.LineBindingPrompter{
		Reader: cmd.InOrStdin(),
		Writer: cmd.ErrOrStderr(),
	}
}

func emitInstallPreview(cmd *cobra.Command, harnessRoot string, preview orbittemplate.TemplateApplyPreview, jsonOutput bool, overwriteExisting bool) error {
	sourcePayload, err := installSourcePayloadFromCommand(cmd, preview)
	if err != nil {
		return err
	}
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), installPreviewJSON{
			DryRun:            true,
			HarnessRoot:       harnessRoot,
			TemplateKind:      installTemplateKindOrbit,
			OverwriteExisting: overwriteExisting,
			Source:            sourcePayload,
			OrbitID:           preview.Source.Manifest.Template.OrbitID,
			Bindings:          installBindingsPayload(preview.ResolvedBindings),
			AgentAddons:       installAgentAddonsPayload(preview.InstallRecord.AgentAddons),
			Files:             installPreviewPaths(harnessRoot, preview),
			Warnings:          append([]string(nil), preview.Warnings...),
			Conflicts:         preview.Conflicts,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness install dry-run from %s\n", preview.Source.Ref); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_ref: %s\n", preview.InstallRecord.Template.SourceRef); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if strings.TrimSpace(preview.RemoteRequestedRef) != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "requested_ref: %s\n", preview.RemoteRequestedRef); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if preview.RemoteResolutionKind != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolved_ref: %s\n", preview.InstallRecord.Template.SourceRef); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolution_kind: %s\n", preview.RemoteResolutionKind); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_kind: %s\n", preview.InstallRecord.Template.SourceKind); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if strings.TrimSpace(preview.InstallRecord.Template.SourceRepo) != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_repo: %s\n", preview.InstallRecord.Template.SourceRepo); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_commit: %s\n", preview.Source.Commit); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_id: %s\n", preview.Source.Manifest.Template.OrbitID); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := emitInstallWarnings(cmd, preview.Warnings); err != nil {
		return err
	}
	if err := emitInstallAgentAddons(cmd, installAgentAddonsPayload(preview.InstallRecord.AgentAddons)); err != nil {
		return err
	}

	if len(preview.ResolvedBindings) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "bindings: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "bindings:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, item := range installBindingsPayload(preview.ResolvedBindings) {
			namespace := ""
			if item.Namespace != "" {
				namespace = fmt.Sprintf(" (namespace: %s)", item.Namespace)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s <- %s%s\n", item.Name, item.Source, namespace); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "files:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, path := range installPreviewPaths(harnessRoot, preview) {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), path); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	if len(preview.Conflicts) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "conflicts: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "conflicts:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, conflict := range preview.Conflicts {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", conflict.Path, conflict.Message); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func emitHarnessTemplateInstallPreview(
	cmd *cobra.Command,
	harnessRoot string,
	source installSourceJSON,
	preview harnesspkg.TemplateInstallPreview,
	jsonOutput bool,
) error {
	files := harnessTemplateInstallPreviewPaths(preview)
	memberIDs := preview.Source.MemberIDs()
	source, err := installSourceJSONFromCommand(cmd, source)
	if err != nil {
		return err
	}

	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), installPreviewJSON{
			DryRun:       true,
			HarnessRoot:  harnessRoot,
			TemplateKind: installTemplateKindHarness,
			Source:       source,
			HarnessID:    preview.Source.Manifest.Template.HarnessID,
			MemberIDs:    memberIDs,
			Bindings:     installBindingsPayload(preview.ResolvedBindings),
			AgentAddons:  installAgentAddonsPayload(preview.BundleRecord.AgentAddons),
			Files:        files,
			Warnings:     append([]string(nil), preview.Warnings...),
			Conflicts:    append([]orbittemplate.ApplyConflict(nil), preview.Conflicts...),
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness install dry-run from %s\n", source.Ref); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_ref: %s\n", source.Ref); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_kind: %s\n", source.Kind); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if strings.TrimSpace(source.Repo) != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_repo: %s\n", source.Repo); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_commit: %s\n", source.Commit); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "template_kind: %s\n", installTemplateKindHarness); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", preview.Source.Manifest.Template.HarnessID); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "member_ids:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, memberID := range memberIDs {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), memberID); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if len(preview.ResolvedBindings) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "bindings: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "bindings:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, item := range installBindingsPayload(preview.ResolvedBindings) {
			namespace := ""
			if item.Namespace != "" {
				namespace = fmt.Sprintf(" (namespace: %s)", item.Namespace)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s <- %s%s\n", item.Name, item.Source, namespace); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "files:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, path := range files {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), path); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if len(preview.Conflicts) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "conflicts: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "conflicts:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, conflict := range preview.Conflicts {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", conflict.Path, conflict.Message); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}
	if err := emitInstallWarnings(cmd, preview.Warnings); err != nil {
		return err
	}
	if err := emitInstallAgentAddons(cmd, installAgentAddonsPayload(preview.BundleRecord.AgentAddons)); err != nil {
		return err
	}

	return nil
}

func harnessTemplateInstallPreviewPaths(preview harnesspkg.TemplateInstallPreview) []string {
	paths := make([]string, 0, len(preview.RenderedDefinitionFiles)+len(preview.RenderedFiles)+4)
	for _, file := range preview.RenderedDefinitionFiles {
		paths = append(paths, file.Path)
	}
	for _, file := range preview.RenderedFiles {
		paths = append(paths, file.Path)
	}
	if preview.RenderedRootAgentsFile != nil {
		paths = append(paths, preview.RenderedRootAgentsFile.Path)
	}
	paths = append(paths,
		fmt.Sprintf(".harness/bundles/%s.yaml", preview.Source.Manifest.Template.HarnessID),
		harnesspkg.ManifestRepoPath(),
	)
	if preview.VarsFile != nil {
		paths = append(paths, ".harness/vars.yaml")
	}

	sort.Strings(paths)

	return paths
}

func emitHarnessTemplateInstallResult(
	cmd *cobra.Command,
	harnessRoot string,
	result harnesspkg.TemplateInstallResult,
	jsonOutput bool,
) error {
	readiness, err := evaluateCommandReadiness(cmd.Context(), harnessRoot)
	if err != nil {
		return err
	}
	memberIDs := result.Preview.Source.MemberIDs()
	bundleIDs, err := harnesspkg.ListBundleRecordIDs(harnessRoot)
	if err != nil {
		return fmt.Errorf("list harness bundle records: %w", err)
	}
	bundleCount := len(bundleIDs)
	sourcePayload, err := installSourceJSONFromCommand(cmd, installSourceJSON{
		Kind:   result.Preview.InstallSource.SourceKind,
		Repo:   result.Preview.InstallSource.SourceRepo,
		Ref:    result.Preview.InstallSource.SourceRef,
		Commit: result.Preview.Source.Commit,
	})
	if err != nil {
		return err
	}

	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), harnessTemplateInstallResultJSON{
			DryRun:       false,
			HarnessRoot:  harnessRoot,
			TemplateKind: installTemplateKindHarness,
			Source:       sourcePayload,
			HarnessID:    result.Preview.Source.Manifest.Template.HarnessID,
			MemberIDs:    memberIDs,
			WrittenPaths: append([]string(nil), result.WrittenPaths...),
			Warnings:     append([]string(nil), result.Preview.Warnings...),
			AgentAddons:  installAgentAddonsPayload(result.Preview.BundleRecord.AgentAddons),
			MemberCount:  len(result.Runtime.Members),
			BundleCount:  bundleCount,
			Readiness:    readiness,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "installed harness template %s into harness %s\n", result.Preview.Source.Manifest.Template.HarnessID, harnessRoot); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_ref: %s\n", result.Preview.InstallSource.SourceRef); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count: %d\n", len(result.Runtime.Members)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "bundle_count: %d\n", bundleCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "files: %d\n", len(result.WrittenPaths)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := emitInstallWarnings(cmd, result.Preview.Warnings); err != nil {
		return err
	}
	if err := emitInstallAgentAddons(cmd, installAgentAddonsPayload(result.Preview.BundleRecord.AgentAddons)); err != nil {
		return err
	}
	if err := emitPostActionReadinessText(cmd.OutOrStdout(), readiness); err != nil {
		return err
	}

	return nil
}

func emitInstallResult(cmd *cobra.Command, harnessRoot string, result orbittemplate.TemplateApplyResult, manifestPath string, runtimeFile harnesspkg.RuntimeFile, jsonOutput bool) error {
	writtenPaths := installResultPaths(harnessRoot, result.WrittenPaths, manifestPath)
	readiness, err := evaluateCommandReadiness(cmd.Context(), harnessRoot)
	if err != nil {
		return err
	}
	sourcePayload, err := installSourcePayloadFromCommand(cmd, result.Preview)
	if err != nil {
		return err
	}

	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), installResultJSON{
			DryRun:       false,
			HarnessRoot:  harnessRoot,
			Source:       sourcePayload,
			OrbitID:      result.Preview.Source.Manifest.Template.OrbitID,
			WrittenPaths: writtenPaths,
			Warnings:     append([]string(nil), result.Preview.Warnings...),
			AgentAddons:  installAgentAddonsPayload(result.Preview.InstallRecord.AgentAddons),
			MemberCount:  len(runtimeFile.Members),
			Readiness:    readiness,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "installed orbit %s into harness %s\n", result.Preview.Source.Manifest.Template.OrbitID, harnessRoot); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_ref: %s\n", result.Preview.InstallRecord.Template.SourceRef); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if strings.TrimSpace(result.Preview.RemoteRequestedRef) != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "requested_ref: %s\n", result.Preview.RemoteRequestedRef); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if result.Preview.RemoteResolutionKind != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolved_ref: %s\n", result.Preview.InstallRecord.Template.SourceRef); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolution_kind: %s\n", result.Preview.RemoteResolutionKind); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count: %d\n", len(runtimeFile.Members)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "files: %d\n", len(writtenPaths)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := emitInstallWarnings(cmd, result.Preview.Warnings); err != nil {
		return err
	}
	if err := emitInstallAgentAddons(cmd, installAgentAddonsPayload(result.Preview.InstallRecord.AgentAddons)); err != nil {
		return err
	}
	if err := emitPostActionReadinessText(cmd.OutOrStdout(), readiness); err != nil {
		return err
	}

	return nil
}

func emitInstallWarnings(cmd *cobra.Command, warnings []string) error {
	if len(warnings) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "warnings: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "warnings:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, warning := range warnings {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), warning); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func installAgentAddonsPayload(snapshot *orbittemplate.AgentAddonsSnapshot) *installAgentAddonsJSON {
	if snapshot == nil || len(snapshot.Hooks) == 0 {
		return nil
	}
	payload := &installAgentAddonsJSON{
		Hooks: make([]installAgentAddonHookJSON, 0, len(snapshot.Hooks)),
	}
	for _, hook := range snapshot.Hooks {
		payload.Hooks = append(payload.Hooks, installAgentAddonHookJSON{
			OrbitID:         hook.OrbitID,
			Package:         hook.Package,
			ID:              hook.ID,
			DisplayID:       hook.DisplayID,
			Required:        hook.Required,
			Description:     hook.Description,
			EventKind:       hook.EventKind,
			Tools:           append([]string(nil), hook.Tools...),
			CommandPatterns: append([]string(nil), hook.CommandPatterns...),
			HandlerType:     hook.HandlerType,
			HandlerPath:     hook.HandlerPath,
			HandlerDigest:   hook.HandlerDigest,
			Targets:         cloneInstallAgentAddonTargets(hook.Targets),
			Activation:      "not_applied",
		})
	}

	return payload
}

func emitInstallAgentAddons(cmd *cobra.Command, payload *installAgentAddonsJSON) error {
	if payload == nil || len(payload.Hooks) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "agent_addons:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, hook := range payload.Hooks {
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"hook: %s activation=%s handler=%s digest=%s\n",
			hook.DisplayID,
			hook.Activation,
			hook.HandlerPath,
			hook.HandlerDigest,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func cloneInstallAgentAddonTargets(targets map[string]bool) map[string]bool {
	if len(targets) == 0 {
		return nil
	}
	cloned := make(map[string]bool, len(targets))
	for target, enabled := range targets {
		cloned[target] = enabled
	}

	return cloned
}

func installSourcePayload(preview orbittemplate.TemplateApplyPreview) installSourceJSON {
	return installSourceJSON{
		Kind:           preview.InstallRecord.Template.SourceKind,
		Repo:           preview.InstallRecord.Template.SourceRepo,
		Ref:            preview.InstallRecord.Template.SourceRef,
		RequestedRef:   preview.RemoteRequestedRef,
		ResolvedRef:    resolvedInstallRef(preview),
		ResolutionKind: resolvedInstallResolutionKind(preview),
		Commit:         preview.Source.Commit,
	}
}

func installSourcePayloadFromCommand(cmd *cobra.Command, preview orbittemplate.TemplateApplyPreview) (installSourceJSON, error) {
	return installSourceJSONFromCommand(cmd, installSourcePayload(preview))
}

func installSourceJSONFromCommand(cmd *cobra.Command, source installSourceJSON) (installSourceJSON, error) {
	metadata, err := readInstallPackageMetadata(cmd)
	if err != nil {
		return installSourceJSON{}, err
	}
	source.PackageName = metadata.Name
	source.PackageCoordinate = metadata.Coordinate
	source.PackageLocatorKind = metadata.LocatorKind
	source.PackageLocator = metadata.Locator

	return source, nil
}

func resolvedInstallRef(preview orbittemplate.TemplateApplyPreview) string {
	if preview.RemoteResolutionKind == "" {
		return ""
	}

	return preview.InstallRecord.Template.SourceRef
}

func resolvedInstallResolutionKind(preview orbittemplate.TemplateApplyPreview) string {
	if preview.RemoteResolutionKind == "" {
		return ""
	}

	return string(preview.RemoteResolutionKind)
}

func installBindingsPayload(resolvedBindings map[string]bindings.ResolvedBinding) []installBindingJSON {
	names := make([]string, 0, len(resolvedBindings))
	for name := range resolvedBindings {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]installBindingJSON, 0, len(names))
	for _, name := range names {
		resolved := resolvedBindings[name]
		source := string(resolved.Source)
		if strings.TrimSpace(source) == "" {
			source = "unknown"
		}
		items = append(items, installBindingJSON{
			Name:      name,
			Source:    source,
			Namespace: resolved.Namespace,
		})
	}

	return items
}

func installPreviewPaths(repoRoot string, preview orbittemplate.TemplateApplyPreview) []string {
	paths := make([]string, 0, len(preview.RenderedFiles)+5)
	for _, file := range preview.RenderedFiles {
		paths = append(paths, file.Path)
	}
	if preview.RenderedSharedAgentsFile != nil {
		paths = append(paths, preview.RenderedSharedAgentsFile.Path)
	}
	paths = append(paths, installPreviewGuidancePaths(repoRoot, preview)...)
	paths = append(paths,
		fmt.Sprintf(".harness/orbits/%s.yaml", preview.Source.Manifest.Template.OrbitID),
		fmt.Sprintf(".harness/installs/%s.yaml", preview.Source.Manifest.Template.OrbitID),
		harnesspkg.ManifestRepoPath(),
		".harness/vars.yaml",
	)

	return dedupeSortedStrings(paths)
}

func buildInstallTransactionPaths(
	repoRoot string,
	preview orbittemplate.TemplateApplyPreview,
	cleanupPlan orbittemplate.InstallOwnedCleanupPlan,
	bundleShrinkPlan *harnesspkg.BundleMemberShrinkPlan,
) []string {
	paths := append([]string(nil), installPreviewPaths(repoRoot, preview)...)
	paths = append(paths, cleanupPlan.DeletePaths...)
	if cleanupPlan.RemoveSharedAgentsFile {
		paths = append(paths, "AGENTS.md")
	}
	if bundleShrinkPlan != nil {
		paths = append(paths, bundleShrinkPlan.DeletePaths...)
		if bundleShrinkPlan.RemoveRootAgentsBlock {
			paths = append(paths, "AGENTS.md")
		}
		if bundleRepoPath, err := harnesspkg.BundleRecordRepoPath(bundleShrinkPlan.ExistingRecord.HarnessID); err == nil {
			paths = append(paths, bundleRepoPath)
		}
	}

	return dedupeSortedStrings(paths)
}

func buildBundleTransferShrinkPlan(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	state installTargetState,
	preview orbittemplate.TemplateApplyPreview,
) (harnesspkg.BundleMemberShrinkPlan, bool, error) {
	if !state.RequiresBundleTransfer {
		return harnesspkg.BundleMemberShrinkPlan{}, false, nil
	}

	plan, err := harnesspkg.BuildBundleMemberShrinkPlan(ctx, repoRoot, state.ExistingBundleRecord, []string{orbitID})
	if err != nil {
		return harnesspkg.BundleMemberShrinkPlan{}, false, fmt.Errorf("build bundle shrink plan: %w", err)
	}
	filtered := harnesspkg.FilterBundleMemberShrinkPlanDeletePaths(plan, installPreviewPreservedPaths(preview))
	return filtered, true, nil
}

func installPreviewPreservedPaths(preview orbittemplate.TemplateApplyPreview) map[string]struct{} {
	paths := make(map[string]struct{}, len(preview.RenderedFiles)+1)
	for _, file := range preview.RenderedFiles {
		paths[file.Path] = struct{}{}
	}
	if definitionPath, err := orbitpkg.HostedDefinitionRelativePath(preview.Source.Manifest.Template.OrbitID); err == nil {
		paths[definitionPath] = struct{}{}
	}
	return paths
}

func runBeforeInstallOwnedCleanupHook(repoRoot string, orbitID string, plan orbittemplate.InstallOwnedCleanupPlan) {
	if beforeInstallOwnedCleanupHook == nil {
		return
	}
	beforeInstallOwnedCleanupHook(repoRoot, orbitID, plan)
}

func installResultPaths(repoRoot string, writtenPaths []string, manifestPath string) []string {
	merged := append([]string(nil), writtenPaths...)
	if strings.TrimSpace(manifestPath) != "" {
		if relativePath, err := filepath.Rel(repoRoot, manifestPath); err == nil {
			merged = append(merged, filepath.ToSlash(relativePath))
		}
	}

	sort.Strings(merged)

	deduped := merged[:0]
	for _, path := range merged {
		if len(deduped) > 0 && deduped[len(deduped)-1] == path {
			continue
		}
		deduped = append(deduped, path)
	}

	return deduped
}
