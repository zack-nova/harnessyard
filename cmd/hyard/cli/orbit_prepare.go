package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type hyardOrbitPackageContext struct {
	RepoRoot      string
	PackageName   string
	OrbitID       string
	State         orbittemplate.CurrentRepoState
	Config        orbitpkg.RepositoryConfig
	Spec          orbitpkg.OrbitSpec
	TrackedPlan   orbitpkg.ProjectionPlan
	WorktreePlan  orbitpkg.ProjectionPlan
	TrackedFiles  []string
	WorktreeFiles []string
}

type orbitPrepareOutput struct {
	RepoRoot     string                         `json:"repo_root"`
	PackageName  string                         `json:"package_name"`
	OrbitID      string                         `json:"orbit"`
	RevisionKind string                         `json:"revision_kind"`
	Ready        bool                           `json:"ready"`
	Blocked      bool                           `json:"blocked"`
	NextActions  []string                       `json:"next_actions"`
	ContentHints orbitPrepareContentHintsOutput `json:"content_hints"`
	Guidance     orbitPrepareGuidanceOutput     `json:"guidance"`
	LocalSkills  orbitPrepareLocalSkillsOutput  `json:"local_skills"`
	RemoteSkills orbitPrepareRemoteSkillsOutput `json:"remote_skills"`
	Schema       orbitPrepareSchemaOutput       `json:"schema"`
	Checkpoint   orbitCheckpointPlanOutput      `json:"checkpoint"`
}

type orbitPrepareContentHintsOutput struct {
	DriftDetected   bool     `json:"drift_detected"`
	BackfillAllowed bool     `json:"backfill_allowed"`
	HintCount       int      `json:"hint_count"`
	HintPaths       []string `json:"hint_paths,omitempty"`
	Applied         bool     `json:"applied"`
}

type orbitPrepareGuidanceOutput struct {
	DriftDetected bool                           `json:"drift_detected"`
	Artifacts     []orbitPrepareGuidanceArtifact `json:"artifacts"`
}

type orbitPrepareGuidanceArtifact struct {
	Target          string `json:"target"`
	State           string `json:"state"`
	BackfillAllowed bool   `json:"backfill_allowed"`
	HasRootArtifact bool   `json:"has_root_artifact"`
	HasOrbitBlock   bool   `json:"has_orbit_block"`
	NeedsSave       bool   `json:"needs_save"`
}

type orbitPrepareLocalSkillsOutput struct {
	DetectedCount int      `json:"detected_count"`
	DetectedRoots []string `json:"detected_roots"`
}

type orbitPrepareRemoteSkillsOutput struct {
	DependencyCount int      `json:"dependency_count"`
	RequiredCount   int      `json:"required_count"`
	Diagnostics     []string `json:"diagnostics,omitempty"`
}

type orbitPrepareSchemaOutput struct {
	Applied            bool     `json:"applied"`
	LegacyRulesPresent bool     `json:"legacy_rules_present"`
	BehaviorPresent    bool     `json:"behavior_present"`
	MigrationRequired  bool     `json:"migration_required"`
	Blocked            bool     `json:"blocked"`
	Diagnostics        []string `json:"diagnostics,omitempty"`
}

type orbitCheckpointPlanOutput struct {
	Required             bool     `json:"required"`
	Blocked              bool     `json:"blocked"`
	CandidatePaths       []string `json:"candidate_paths"`
	BlockedPaths         []string `json:"blocked_paths"`
	UntrackedExportPaths []string `json:"untracked_export_paths"`
}

type orbitCheckpointOutput struct {
	RepoRoot             string   `json:"repo_root"`
	PackageName          string   `json:"package_name"`
	OrbitID              string   `json:"orbit"`
	Ready                bool     `json:"ready"`
	Blocked              bool     `json:"blocked"`
	Committed            bool     `json:"committed"`
	Commit               string   `json:"commit,omitempty"`
	CandidatePaths       []string `json:"candidate_paths"`
	BlockedPaths         []string `json:"blocked_paths"`
	UntrackedExportPaths []string `json:"untracked_export_paths"`
}

func newOrbitPrepareCommand() *cobra.Command {
	var check bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "prepare <package>",
		Short: "Inspect package readiness before publish",
		Long: "Inspect package readiness before publish.\n" +
			"Use --check --json to report content hints, guide drift, skill diagnostics,\n" +
			"and checkpoint candidates without mutating. Without --check, prepare prompts\n" +
			"before applying safe content-hint fixes.",
		Example: "" +
			"  hyard orbit prepare docs --check --json\n" +
			"  hyard orbit prepare docs\n" +
			"  hyard orbit prepare docs --yes --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			var output orbitPrepareOutput
			if check {
				if yes {
					return fmt.Errorf("--yes cannot be used with --check")
				}
				output, err = buildHyardOrbitPrepareOutput(cmd.Context(), cmd, args[0])
				if err != nil {
					return err
				}
			} else {
				output, err = runHyardOrbitPrepareMutating(cmd.Context(), cmd, args[0], yes)
				if err != nil {
					return err
				}
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			return printHyardOrbitPrepareText(cmd, output)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Inspect package readiness without mutating")
	cmd.Flags().BoolVar(&yes, "yes", false, "Apply safe prepare fixes without prompting")
	addHyardJSONFlag(cmd)

	return cmd
}

func newOrbitCheckpointCommand() *cobra.Command {
	var check bool
	var message string

	cmd := &cobra.Command{
		Use:   "checkpoint <package>",
		Short: "Commit package-relevant authoring changes",
		Long: "Commit package-relevant authoring changes.\n" +
			"Checkpoint stages only tracked package authoring and export inputs, and refuses\n" +
			"unrelated tracked dirty paths or untracked export-surface files.",
		Example: "" +
			"  hyard orbit checkpoint docs --check --json\n" +
			"  hyard orbit checkpoint docs -m \"Update docs\"\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packageCtx, err := loadHyardOrbitPackageContext(cmd.Context(), cmd, args[0], "orbit checkpoint")
			if err != nil {
				return err
			}
			plan, err := buildHyardOrbitCheckpointPlan(cmd.Context(), packageCtx)
			if err != nil {
				return err
			}

			output := orbitCheckpointOutput{
				RepoRoot:             packageCtx.RepoRoot,
				PackageName:          packageCtx.PackageName,
				OrbitID:              packageCtx.OrbitID,
				Ready:                !plan.Required && !plan.Blocked,
				Blocked:              plan.Blocked,
				CandidatePaths:       append([]string(nil), plan.CandidatePaths...),
				BlockedPaths:         append([]string(nil), plan.BlockedPaths...),
				UntrackedExportPaths: append([]string(nil), plan.UntrackedExportPaths...),
			}

			if !check {
				if plan.Blocked {
					return fmt.Errorf("checkpoint blocked for package %q; unrelated tracked paths or untracked export files must be handled first", packageCtx.PackageName)
				}
				if len(plan.CandidatePaths) == 0 {
					return fmt.Errorf("no package changes to checkpoint for %q", packageCtx.PackageName)
				}
				if strings.TrimSpace(message) == "" {
					return fmt.Errorf("checkpoint requires -m/--message")
				}
				if err := gitpkg.StageAllPathspec(cmd.Context(), packageCtx.RepoRoot, plan.CandidatePaths); err != nil {
					return fmt.Errorf("stage package checkpoint paths: %w", err)
				}
				if err := gitpkg.CommitPathspec(cmd.Context(), packageCtx.RepoRoot, plan.CandidatePaths, message); err != nil {
					return fmt.Errorf("create package checkpoint commit: %w", err)
				}
				commit, err := gitpkg.HeadCommit(cmd.Context(), packageCtx.RepoRoot)
				if err != nil {
					return fmt.Errorf("resolve checkpoint commit: %w", err)
				}
				output.Committed = true
				output.Commit = commit
				output.Ready = true
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			return printHyardOrbitCheckpointText(cmd, output)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Inspect package checkpoint paths without mutating")
	cmd.Flags().StringVarP(&message, "message", "m", "", "Checkpoint commit message")
	addHyardJSONFlag(cmd)

	return cmd
}

func buildHyardOrbitPrepareOutput(ctx context.Context, cmd *cobra.Command, rawPackage string) (orbitPrepareOutput, error) {
	packageCtx, err := loadHyardOrbitPackageContext(ctx, cmd, rawPackage, "orbit prepare")
	if err != nil {
		return orbitPrepareOutput{}, err
	}

	contentHints, err := inspectHyardOrbitContentHints(packageCtx)
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	guidance, err := inspectHyardOrbitGuidance(ctx, packageCtx)
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	localSkills, err := inspectHyardOrbitLocalSkills(packageCtx)
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	remoteSkills := inspectHyardOrbitRemoteSkills(packageCtx)
	schema, err := inspectHyardOrbitSchema(ctx, packageCtx)
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	checkpoint, err := buildHyardOrbitCheckpointPlan(ctx, packageCtx)
	if err != nil {
		return orbitPrepareOutput{}, err
	}

	nextActions := hyardOrbitPrepareNextActions(packageCtx.PackageName, contentHints, guidance, localSkills, remoteSkills, checkpoint)
	ready := len(nextActions) == 0 && !checkpoint.Required && !contentHints.DriftDetected && !guidance.DriftDetected && localSkills.DetectedCount == 0 && len(remoteSkills.Diagnostics) == 0 && !schema.Blocked

	return orbitPrepareOutput{
		RepoRoot:     packageCtx.RepoRoot,
		PackageName:  packageCtx.PackageName,
		OrbitID:      packageCtx.OrbitID,
		RevisionKind: packageCtx.State.Kind,
		Ready:        ready,
		Blocked:      !ready,
		NextActions:  nextActions,
		ContentHints: contentHints,
		Guidance:     guidance,
		LocalSkills:  localSkills,
		RemoteSkills: remoteSkills,
		Schema:       schema,
		Checkpoint:   checkpoint,
	}, nil
}

func runHyardOrbitPrepareMutating(ctx context.Context, cmd *cobra.Command, packageName string, yes bool) (orbitPrepareOutput, error) {
	packageCtx, err := loadHyardOrbitPackageContext(ctx, cmd, packageName, "orbit prepare")
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	contentHints, err := inspectHyardOrbitContentHints(packageCtx)
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	schema, err := inspectHyardOrbitSchema(ctx, packageCtx)
	if err != nil {
		return orbitPrepareOutput{}, err
	}

	schemaApplied := false
	if schema.Blocked {
		return orbitPrepareOutput{}, fmt.Errorf("prepare blocked for package %q; %s", packageCtx.PackageName, strings.Join(schema.Diagnostics, "; "))
	}
	if schema.MigrationRequired {
		if !yes {
			prompter := orbittemplate.LineConfirmPrompter{
				Reader: cmd.InOrStdin(),
				Writer: cmd.ErrOrStderr(),
			}
			confirmed, err := prompter.Confirm(ctx, formatHyardOrbitPrepareSchemaPrompt(packageCtx.PackageName))
			if err != nil {
				return orbitPrepareOutput{}, fmt.Errorf("confirm schema migration: %w", err)
			}
			if !confirmed {
				return orbitPrepareOutput{}, fmt.Errorf("prepare canceled for package %q; schema migration was not applied", packageCtx.PackageName)
			}
		}
		if _, err := orbitpkg.WriteHostedOrbitSpec(packageCtx.RepoRoot, packageCtx.Spec); err != nil {
			return orbitPrepareOutput{}, fmt.Errorf("write canonical behavior schema: %w", err)
		}
		schemaApplied = true
		packageCtx, err = loadHyardOrbitPackageContext(ctx, cmd, packageName, "orbit prepare")
		if err != nil {
			return orbitPrepareOutput{}, err
		}
		contentHints, err = inspectHyardOrbitContentHints(packageCtx)
		if err != nil {
			return orbitPrepareOutput{}, err
		}
	}

	applied := false
	if contentHints.DriftDetected {
		if !contentHints.BackfillAllowed {
			return orbitPrepareOutput{}, fmt.Errorf(
				"prepare blocked for package %q; content hint diagnostics require review; run `hyard orbit content apply %s --check --json`",
				packageCtx.PackageName,
				packageCtx.PackageName,
			)
		}
		if !yes {
			prompter := orbittemplate.LineConfirmPrompter{
				Reader: cmd.InOrStdin(),
				Writer: cmd.ErrOrStderr(),
			}
			confirmed, err := prompter.Confirm(ctx, formatHyardOrbitPrepareContentHintPrompt(packageCtx.PackageName, contentHints.HintCount))
			if err != nil {
				return orbitPrepareOutput{}, fmt.Errorf("confirm content hint apply: %w", err)
			}
			if !confirmed {
				return orbitPrepareOutput{}, fmt.Errorf("prepare canceled for package %q; content hints were not applied", packageCtx.PackageName)
			}
		}
		if _, err := orbitpkg.BackfillMemberHints(packageCtx.RepoRoot, packageCtx.Spec, packageCtx.WorktreeFiles); err != nil {
			return orbitPrepareOutput{}, fmt.Errorf("apply package content hints: %w", err)
		}
		applied = true
	}

	packageCtx, err = loadHyardOrbitPackageContext(ctx, cmd, packageName, "orbit prepare")
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	guidance, err := inspectHyardOrbitGuidance(ctx, packageCtx)
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	if guidance.DriftDetected {
		if !yes {
			prompter := orbittemplate.LineConfirmPrompter{
				Reader: cmd.InOrStdin(),
				Writer: cmd.ErrOrStderr(),
			}
			confirmed, err := prompter.Confirm(ctx, formatHyardOrbitPrepareGuidancePrompt(packageCtx.PackageName, guidance))
			if err != nil {
				return orbitPrepareOutput{}, fmt.Errorf("confirm guide save: %w", err)
			}
			if !confirmed {
				return orbitPrepareOutput{}, fmt.Errorf("prepare canceled for package %q; guide changes were not saved", packageCtx.PackageName)
			}
		}
		if err := applyHyardOrbitGuidanceSaves(ctx, packageCtx, guidance); err != nil {
			return orbitPrepareOutput{}, err
		}
	}

	output, err := buildHyardOrbitPrepareOutput(ctx, cmd, packageName)
	if err != nil {
		return orbitPrepareOutput{}, err
	}
	output.ContentHints.Applied = applied
	output.Schema.Applied = schemaApplied

	return output, nil
}

func formatHyardOrbitPrepareSchemaPrompt(packageName string) string {
	return fmt.Sprintf("Normalize legacy top-level rules to behavior for package %q? [y/N] ", packageName)
}

func formatHyardOrbitPrepareContentHintPrompt(packageName string, hintCount int) string {
	noun := "content hints"
	if hintCount == 1 {
		noun = "content hint"
	}
	return fmt.Sprintf("Apply %d %s to package %q? [y/N] ", hintCount, noun, packageName)
}

func formatHyardOrbitPrepareGuidancePrompt(packageName string, guidance orbitPrepareGuidanceOutput) string {
	return fmt.Sprintf("Save guide changes from %s for package %q? [y/N] ", strings.Join(hyardGuidanceSavePaths(guidance), " "), packageName)
}

func runHyardPublishPrepareCheckpointFlow(
	ctx context.Context,
	cmd *cobra.Command,
	packageName string,
	prepare bool,
	checkpoint bool,
	trackNew bool,
	message string,
) error {
	if checkpoint {
		packageCtx, err := loadHyardOrbitPackageContext(ctx, cmd, packageName, "publish checkpoint")
		if err != nil {
			return err
		}
		plan, err := buildHyardOrbitCheckpointPlan(ctx, packageCtx)
		if err != nil {
			return err
		}
		if prepare {
			plan, err = relaxHyardCheckpointPreflightForPrepareHints(ctx, packageCtx, plan)
			if err != nil {
				return err
			}
		}
		if trackNew {
			plan = includeHyardCheckpointUntrackedExportPaths(plan)
		}
		if plan.Blocked {
			return formatHyardOrbitCheckpointBlockedError(packageCtx.PackageName, plan)
		}
	}
	if prepare {
		if err := runHyardOrbitPrepareScripted(ctx, cmd, packageName); err != nil {
			return err
		}
	}
	if checkpoint {
		if err := runHyardOrbitCheckpointScripted(ctx, cmd, packageName, message, true, trackNew); err != nil {
			return err
		}
	}

	return nil
}

func relaxHyardCheckpointPreflightForPrepareHints(
	ctx context.Context,
	packageCtx hyardOrbitPackageContext,
	plan orbitCheckpointPlanOutput,
) (orbitCheckpointPlanOutput, error) {
	inspection, err := orbitpkg.InspectMemberHints(packageCtx.RepoRoot, packageCtx.Spec, packageCtx.WorktreeFiles)
	if err != nil {
		return orbitCheckpointPlanOutput{}, fmt.Errorf("inspect content hints: %w", err)
	}
	if len(inspection.Hints) == 0 {
		return plan, nil
	}

	hintPaths := make(map[string]struct{}, len(inspection.Hints))
	for _, hint := range inspection.Hints {
		hintPaths[hint.HintPath] = struct{}{}
	}

	var blocked []string
	for _, blockedPath := range plan.BlockedPaths {
		if _, ok := hintPaths[blockedPath]; ok {
			continue
		}
		blocked = append(blocked, blockedPath)
	}

	untrackedExport := append([]string(nil), plan.UntrackedExportPaths...)
	statusEntries, err := gitpkg.WorktreeStatus(ctx, packageCtx.RepoRoot)
	if err != nil {
		return orbitCheckpointPlanOutput{}, fmt.Errorf("load worktree status: %w", err)
	}
	for _, entry := range statusEntries {
		if entry.Tracked {
			continue
		}
		if _, ok := hintPaths[entry.Path]; ok {
			untrackedExport = append(untrackedExport, entry.Path)
		}
	}

	plan.BlockedPaths = sortedUniqueStrings(blocked)
	plan.UntrackedExportPaths = sortedUniqueStrings(untrackedExport)
	plan.Blocked = len(plan.BlockedPaths) > 0 || len(plan.UntrackedExportPaths) > 0
	plan.Required = len(plan.CandidatePaths) > 0 || plan.Blocked

	return plan, nil
}

func includeHyardCheckpointUntrackedExportPaths(plan orbitCheckpointPlanOutput) orbitCheckpointPlanOutput {
	if len(plan.UntrackedExportPaths) == 0 {
		return plan
	}
	plan.CandidatePaths = sortedUniqueStrings(append(plan.CandidatePaths, plan.UntrackedExportPaths...))
	plan.UntrackedExportPaths = nil
	plan.Blocked = len(plan.BlockedPaths) > 0
	plan.Required = len(plan.CandidatePaths) > 0 || plan.Blocked
	return plan
}

func runHyardOrbitPrepareScripted(ctx context.Context, cmd *cobra.Command, packageName string) error {
	packageCtx, err := loadHyardOrbitPackageContext(ctx, cmd, packageName, "publish prepare")
	if err != nil {
		return err
	}
	schema, err := inspectHyardOrbitSchema(ctx, packageCtx)
	if err != nil {
		return err
	}
	if schema.Blocked {
		return fmt.Errorf("prepare blocked for package %q; %s", packageCtx.PackageName, strings.Join(schema.Diagnostics, "; "))
	}
	if schema.MigrationRequired {
		if _, err := orbitpkg.WriteHostedOrbitSpec(packageCtx.RepoRoot, packageCtx.Spec); err != nil {
			return fmt.Errorf("write canonical behavior schema: %w", err)
		}
		packageCtx, err = loadHyardOrbitPackageContext(ctx, cmd, packageName, "publish prepare")
		if err != nil {
			return err
		}
	}
	contentHints, err := inspectHyardOrbitContentHints(packageCtx)
	if err != nil {
		return err
	}
	if contentHints.DriftDetected {
		if !contentHints.BackfillAllowed {
			return fmt.Errorf(
				"prepare blocked for package %q; content hint diagnostics require review; run `hyard orbit content apply %s --check --json`, resolve the reported hint diagnostics, then publish again",
				packageCtx.PackageName,
				packageCtx.PackageName,
			)
		}
		if _, err := orbitpkg.BackfillMemberHints(packageCtx.RepoRoot, packageCtx.Spec, packageCtx.WorktreeFiles); err != nil {
			return fmt.Errorf("apply package content hints: %w", err)
		}
	}

	packageCtx, err = loadHyardOrbitPackageContext(ctx, cmd, packageName, "publish prepare")
	if err != nil {
		return err
	}
	contentHints, err = inspectHyardOrbitContentHints(packageCtx)
	if err != nil {
		return err
	}
	if contentHints.DriftDetected {
		return fmt.Errorf("prepare blocked for package %q; content hints still require `hyard orbit content apply %s --check --json`", packageCtx.PackageName, packageCtx.PackageName)
	}
	guidance, err := inspectHyardOrbitGuidance(ctx, packageCtx)
	if err != nil {
		return err
	}
	if guidance.DriftDetected {
		if err := applyHyardOrbitGuidanceSaves(ctx, packageCtx, guidance); err != nil {
			return err
		}
		packageCtx, err = loadHyardOrbitPackageContext(ctx, cmd, packageName, "publish prepare")
		if err != nil {
			return err
		}
		guidance, err = inspectHyardOrbitGuidance(ctx, packageCtx)
		if err != nil {
			return err
		}
		if guidance.DriftDetected {
			return fmt.Errorf("prepare blocked for package %q; guide changes still require `hyard guide save --orbit %s --target all`", packageCtx.PackageName, packageCtx.PackageName)
		}
	}
	localSkills, err := inspectHyardOrbitLocalSkills(packageCtx)
	if err != nil {
		return err
	}
	if localSkills.DetectedCount > 0 {
		return fmt.Errorf("prepare blocked for package %q; detected local skill roots require explicit aggregation; run `hyard publish orbit %s --aggregate-detected-skills`", packageCtx.PackageName, packageCtx.PackageName)
	}
	remoteSkills := inspectHyardOrbitRemoteSkills(packageCtx)
	if len(remoteSkills.Diagnostics) > 0 {
		return fmt.Errorf("prepare blocked for package %q; remote skill dependency diagnostics require `hyard orbit skill inspect --orbit %s --json`", packageCtx.PackageName, packageCtx.PackageName)
	}

	return nil
}

func applyHyardOrbitGuidanceSaves(ctx context.Context, packageCtx hyardOrbitPackageContext, guidance orbitPrepareGuidanceOutput) error {
	for _, artifact := range guidance.Artifacts {
		if !artifact.NeedsSave {
			continue
		}
		switch artifact.Target {
		case string(orbittemplate.GuidanceTargetAgents):
			if _, err := orbittemplate.BackfillOrbitBrief(ctx, orbittemplate.BriefBackfillInput{
				RepoRoot: packageCtx.RepoRoot,
				OrbitID:  packageCtx.OrbitID,
			}); err != nil {
				return fmt.Errorf("save AGENTS.md guide changes for package %q: %w", packageCtx.PackageName, err)
			}
		case string(orbittemplate.GuidanceTargetHumans):
			if _, err := orbittemplate.BackfillOrbitHumans(ctx, orbittemplate.HumansBackfillInput{
				RepoRoot: packageCtx.RepoRoot,
				OrbitID:  packageCtx.OrbitID,
			}); err != nil {
				return fmt.Errorf("save HUMANS.md guide changes for package %q: %w", packageCtx.PackageName, err)
			}
		case string(orbittemplate.GuidanceTargetBootstrap):
			if _, err := orbittemplate.BackfillOrbitBootstrap(ctx, orbittemplate.BootstrapBackfillInput{
				RepoRoot: packageCtx.RepoRoot,
				OrbitID:  packageCtx.OrbitID,
			}); err != nil {
				return fmt.Errorf("save BOOTSTRAP.md guide changes for package %q: %w", packageCtx.PackageName, err)
			}
		default:
			return fmt.Errorf("save guide changes for package %q: unsupported guidance target %q", packageCtx.PackageName, artifact.Target)
		}
	}
	return nil
}

func hyardGuidanceSavePaths(guidance orbitPrepareGuidanceOutput) []string {
	paths := make([]string, 0, len(guidance.Artifacts))
	for _, artifact := range guidance.Artifacts {
		if !artifact.NeedsSave {
			continue
		}
		paths = append(paths, hyardPublishGuidanceArtifactPath(artifact.Target))
	}
	return sortedUniqueStrings(paths)
}

func runHyardOrbitCheckpointScripted(ctx context.Context, cmd *cobra.Command, packageName string, message string, allowNoop bool, trackNew bool) error {
	packageCtx, err := loadHyardOrbitPackageContext(ctx, cmd, packageName, "publish checkpoint")
	if err != nil {
		return err
	}
	plan, err := buildHyardOrbitCheckpointPlan(ctx, packageCtx)
	if err != nil {
		return err
	}
	if trackNew {
		plan = includeHyardCheckpointUntrackedExportPaths(plan)
	}
	if plan.Blocked {
		return formatHyardOrbitCheckpointBlockedError(packageCtx.PackageName, plan)
	}
	if len(plan.CandidatePaths) == 0 {
		if allowNoop {
			return nil
		}
		return fmt.Errorf("no package changes to checkpoint for %q", packageCtx.PackageName)
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("checkpoint requires -m/--message")
	}
	if err := gitpkg.StageAllPathspec(ctx, packageCtx.RepoRoot, plan.CandidatePaths); err != nil {
		return fmt.Errorf("stage package checkpoint paths: %w", err)
	}
	if err := gitpkg.CommitPathspec(ctx, packageCtx.RepoRoot, plan.CandidatePaths, message); err != nil {
		return fmt.Errorf("create package checkpoint commit: %w", err)
	}

	return nil
}

func formatHyardOrbitCheckpointBlockedError(packageName string, plan orbitCheckpointPlanOutput) error {
	reasons := make([]string, 0, 2)
	if len(plan.BlockedPaths) > 0 {
		reasons = append(reasons, "unrelated tracked paths: "+strings.Join(plan.BlockedPaths, " "))
	}
	if len(plan.UntrackedExportPaths) > 0 {
		reasons = append(reasons, "untracked export files: "+strings.Join(plan.UntrackedExportPaths, " ")+"; run `git add "+strings.Join(plan.UntrackedExportPaths, " ")+"` or remove them")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "package checkpoint is blocked")
	}

	return fmt.Errorf("checkpoint blocked for package %q; %s", packageName, strings.Join(reasons, "; "))
}

func loadHyardOrbitPackageContext(ctx context.Context, cmd *cobra.Command, rawPackage string, operation string) (hyardOrbitPackageContext, error) {
	coordinate, err := parseHyardPackageCoordinate(rawPackage)
	if err != nil {
		return hyardOrbitPackageContext{}, err
	}
	if coordinate.Kind != ids.PackageCoordinateName {
		return hyardOrbitPackageContext{}, fmt.Errorf("%s only supports the current workspace package; use the bare package name %q", operation, coordinate.Name)
	}

	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return hyardOrbitPackageContext{}, err
	}
	repo, err := gitpkg.DiscoverRepo(ctx, workingDir)
	if err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("discover git repository: %w", err)
	}
	state, err := orbittemplate.LoadCurrentRepoState(ctx, repo.Root)
	if err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("load current repo state: %w", err)
	}
	switch state.Kind {
	case "source", "orbit_template":
	default:
		return hyardOrbitPackageContext{}, fmt.Errorf("%s requires a source or orbit_template revision; current revision kind is %q", operation, state.Kind)
	}
	if _, err := orbittemplate.RequireCurrentBranch(state, operation); err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("require current branch for %s: %w", operation, err)
	}
	if state.OrbitID != "" && state.OrbitID != coordinate.Name {
		return hyardOrbitPackageContext{}, fmt.Errorf("package target %q must match current orbit package %q", coordinate.Name, state.OrbitID)
	}

	config, err := orbitpkg.LoadHostedRepositoryConfig(ctx, repo.Root)
	if err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("load hosted orbit config: %w", err)
	}
	definition, found := config.OrbitByID(coordinate.Name)
	if !found {
		return hyardOrbitPackageContext{}, fmt.Errorf("orbit package %q not found", coordinate.Name)
	}
	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repo.Root, definition.ID)
	if err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repo.Root)
	if err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("load tracked files: %w", err)
	}
	worktreeFiles, err := gitpkg.WorktreeFiles(ctx, repo.Root)
	if err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("load worktree files: %w", err)
	}
	trackedPlan, err := orbitpkg.ResolveProjectionPlan(config, spec, trackedFiles)
	if err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("resolve tracked package plan: %w", err)
	}
	worktreePlan, err := orbitpkg.ResolveProjectionPlan(config, spec, worktreeFiles)
	if err != nil {
		return hyardOrbitPackageContext{}, fmt.Errorf("resolve worktree package plan: %w", err)
	}

	return hyardOrbitPackageContext{
		RepoRoot:      repo.Root,
		PackageName:   coordinate.Name,
		OrbitID:       definition.ID,
		State:         state,
		Config:        config,
		Spec:          spec,
		TrackedPlan:   trackedPlan,
		WorktreePlan:  worktreePlan,
		TrackedFiles:  append([]string(nil), trackedFiles...),
		WorktreeFiles: append([]string(nil), worktreeFiles...),
	}, nil
}

func inspectHyardOrbitContentHints(packageCtx hyardOrbitPackageContext) (orbitPrepareContentHintsOutput, error) {
	inspection, err := orbitpkg.InspectMemberHints(packageCtx.RepoRoot, packageCtx.Spec, packageCtx.WorktreeFiles)
	if err != nil {
		return orbitPrepareContentHintsOutput{}, fmt.Errorf("inspect content hints: %w", err)
	}

	hintPaths := make([]string, 0, len(inspection.Hints))
	for _, hint := range inspection.Hints {
		hintPaths = append(hintPaths, hint.HintPath)
	}

	return orbitPrepareContentHintsOutput{
		DriftDetected:   inspection.DriftDetected,
		BackfillAllowed: inspection.BackfillAllowed,
		HintCount:       len(inspection.Hints),
		HintPaths:       sortedUniqueStrings(hintPaths),
	}, nil
}

func inspectHyardOrbitGuidance(ctx context.Context, packageCtx hyardOrbitPackageContext) (orbitPrepareGuidanceOutput, error) {
	artifacts := make([]orbitPrepareGuidanceArtifact, 0, 3)
	agents, err := orbittemplate.InspectOrbitBriefLaneForOperation(ctx, packageCtx.RepoRoot, packageCtx.OrbitID, "backfill")
	if err != nil {
		return orbitPrepareGuidanceOutput{}, fmt.Errorf("inspect agents guidance: %w", err)
	}
	artifacts = append(artifacts, guidanceArtifactFromStatus("agents", string(agents.State), agents.BackfillAllowed, agents.HasRootAgents, agents.HasOrbitBlock))

	humans, err := orbittemplate.InspectOrbitHumansLaneForOperation(ctx, packageCtx.RepoRoot, packageCtx.OrbitID, "backfill")
	if err != nil {
		return orbitPrepareGuidanceOutput{}, fmt.Errorf("inspect humans guidance: %w", err)
	}
	artifacts = append(artifacts, guidanceArtifactFromStatus("humans", string(humans.State), humans.BackfillAllowed, humans.HasRootHumans, humans.HasOrbitBlock))

	bootstrap, err := orbittemplate.InspectOrbitBootstrapLaneForOperation(ctx, packageCtx.RepoRoot, packageCtx.OrbitID, "backfill")
	if err != nil {
		return orbitPrepareGuidanceOutput{}, fmt.Errorf("inspect bootstrap guidance: %w", err)
	}
	artifacts = append(artifacts, guidanceArtifactFromStatus("bootstrap", string(bootstrap.State), bootstrap.BackfillAllowed, bootstrap.HasRootBootstrap, bootstrap.HasOrbitBlock))

	drift := false
	for _, artifact := range artifacts {
		if artifact.NeedsSave {
			drift = true
			break
		}
	}

	return orbitPrepareGuidanceOutput{
		DriftDetected: drift,
		Artifacts:     artifacts,
	}, nil
}

func guidanceArtifactFromStatus(target string, state string, backfillAllowed bool, hasRootArtifact bool, hasOrbitBlock bool) orbitPrepareGuidanceArtifact {
	needsSave := backfillAllowed && state == string(orbittemplate.BriefLaneStateMaterializedDrifted)
	return orbitPrepareGuidanceArtifact{
		Target:          target,
		State:           state,
		BackfillAllowed: backfillAllowed,
		HasRootArtifact: hasRootArtifact,
		HasOrbitBlock:   hasOrbitBlock,
		NeedsSave:       needsSave,
	}
}

func inspectHyardOrbitLocalSkills(packageCtx hyardOrbitPackageContext) (orbitPrepareLocalSkillsOutput, error) {
	declared, err := orbitpkg.ResolveLocalSkillCapabilities(packageCtx.RepoRoot, packageCtx.Spec, packageCtx.TrackedFiles, packageCtx.TrackedPlan.ExportPaths)
	if err != nil {
		return orbitPrepareLocalSkillsOutput{}, fmt.Errorf("resolve declared local skills: %w", err)
	}
	candidates, err := orbitpkg.DetectValidLocalSkillCapabilities(packageCtx.RepoRoot, packageCtx.TrackedFiles, packageCtx.TrackedPlan.ExportPaths)
	if err != nil {
		return orbitPrepareLocalSkillsOutput{}, fmt.Errorf("detect local skills: %w", err)
	}
	declaredRoots := make(map[string]struct{}, len(declared))
	for _, skill := range declared {
		declaredRoots[skill.RootPath] = struct{}{}
	}

	roots := make([]string, 0)
	for _, candidate := range candidates {
		if _, ok := declaredRoots[candidate.RootPath]; ok {
			continue
		}
		roots = append(roots, candidate.RootPath)
	}
	roots = sortedUniqueStrings(roots)

	return orbitPrepareLocalSkillsOutput{
		DetectedCount: len(roots),
		DetectedRoots: roots,
	}, nil
}

func inspectHyardOrbitRemoteSkills(packageCtx hyardOrbitPackageContext) orbitPrepareRemoteSkillsOutput {
	resolved, err := orbitpkg.ResolveRemoteSkillCapabilities(packageCtx.Spec)
	if err != nil {
		return orbitPrepareRemoteSkillsOutput{Diagnostics: []string{err.Error()}}
	}
	required := 0
	for _, dependency := range resolved {
		if dependency.Required {
			required++
		}
	}

	return orbitPrepareRemoteSkillsOutput{
		DependencyCount: len(resolved),
		RequiredCount:   required,
	}
}

func inspectHyardOrbitSchema(ctx context.Context, packageCtx hyardOrbitPackageContext) (orbitPrepareSchemaOutput, error) {
	relativePath, err := orbitpkg.HostedDefinitionRelativePath(packageCtx.OrbitID)
	if err != nil {
		return orbitPrepareSchemaOutput{}, fmt.Errorf("build hosted orbit spec path: %w", err)
	}
	data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, packageCtx.RepoRoot, relativePath)
	if err != nil {
		return orbitPrepareSchemaOutput{}, fmt.Errorf("read hosted orbit spec schema: %w", err)
	}

	keys, err := yamlRootMappingKeys(data)
	if err != nil {
		return orbitPrepareSchemaOutput{}, fmt.Errorf("inspect hosted orbit spec schema: %w", err)
	}
	output := orbitPrepareSchemaOutput{
		LegacyRulesPresent: keys["rules"],
		BehaviorPresent:    keys["behavior"],
	}
	switch {
	case output.LegacyRulesPresent && output.BehaviorPresent:
		output.Blocked = true
		output.Diagnostics = []string{"legacy top-level rules and canonical behavior cannot both be present; keep behavior and remove rules"}
	case output.LegacyRulesPresent:
		output.MigrationRequired = true
		output.Diagnostics = []string{"legacy top-level rules is accepted as behavior input and will be written as behavior on the next explicit OrbitSpec write"}
	}

	return output, nil
}

func yamlRootMappingKeys(data []byte) (map[string]bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode yaml document: %w", err)
	}
	keys := make(map[string]bool)
	if len(document.Content) == 0 {
		return keys, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return keys, nil
	}
	for index := 0; index < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		keys[keyNode.Value] = true
	}

	return keys, nil
}

func buildHyardOrbitCheckpointPlan(ctx context.Context, packageCtx hyardOrbitPackageContext) (orbitCheckpointPlanOutput, error) {
	statusEntries, err := gitpkg.WorktreeStatus(ctx, packageCtx.RepoRoot)
	if err != nil {
		return orbitCheckpointPlanOutput{}, fmt.Errorf("load worktree status: %w", err)
	}
	trackedExportPaths := stringSet(packageCtx.TrackedPlan.ExportPaths)
	worktreeExportPaths := stringSet(packageCtx.WorktreePlan.ExportPaths)
	controlPaths := hyardPackageCheckpointControlPaths(packageCtx)

	var candidates []string
	var blocked []string
	var untrackedExport []string
	for _, entry := range statusEntries {
		if entry.Tracked {
			if _, ok := controlPaths[entry.Path]; ok {
				candidates = append(candidates, entry.Path)
				continue
			}
			if _, ok := trackedExportPaths[entry.Path]; ok {
				candidates = append(candidates, entry.Path)
				continue
			}
			blocked = append(blocked, entry.Path)
			continue
		}

		if _, ok := controlPaths[entry.Path]; ok {
			candidates = append(candidates, entry.Path)
			continue
		}
		if _, ok := worktreeExportPaths[entry.Path]; ok {
			untrackedExport = append(untrackedExport, entry.Path)
		}
	}

	candidates = sortedUniqueStrings(candidates)
	blocked = sortedUniqueStrings(blocked)
	untrackedExport = sortedUniqueStrings(untrackedExport)

	return orbitCheckpointPlanOutput{
		Required:             len(candidates) > 0 || len(blocked) > 0 || len(untrackedExport) > 0,
		Blocked:              len(blocked) > 0 || len(untrackedExport) > 0,
		CandidatePaths:       candidates,
		BlockedPaths:         blocked,
		UntrackedExportPaths: untrackedExport,
	}, nil
}

func hyardPackageCheckpointControlPaths(packageCtx hyardOrbitPackageContext) map[string]struct{} {
	paths := []string{
		".orbit/config.yaml",
		".harness/manifest.yaml",
		".harness/orbits/" + packageCtx.OrbitID + ".yaml",
		"AGENTS.md",
		"HUMANS.md",
		"BOOTSTRAP.md",
	}
	return stringSet(paths)
}

func hyardOrbitPrepareNextActions(
	packageName string,
	contentHints orbitPrepareContentHintsOutput,
	guidance orbitPrepareGuidanceOutput,
	localSkills orbitPrepareLocalSkillsOutput,
	remoteSkills orbitPrepareRemoteSkillsOutput,
	checkpoint orbitCheckpointPlanOutput,
) []string {
	actions := make([]string, 0)
	if contentHints.DriftDetected {
		if contentHints.BackfillAllowed {
			actions = append(actions, "hyard orbit content apply "+packageName)
		} else {
			actions = append(actions, "hyard orbit content apply "+packageName+" --check --json")
		}
	}
	if guidance.DriftDetected {
		actions = append(actions, "hyard guide save --orbit "+packageName+" --target all")
	}
	if localSkills.DetectedCount > 0 {
		actions = append(actions, "hyard publish orbit "+packageName+" --aggregate-detected-skills")
	}
	if len(remoteSkills.Diagnostics) > 0 {
		actions = append(actions, "hyard orbit skill inspect --orbit "+packageName+" --json")
	}
	if len(checkpoint.UntrackedExportPaths) > 0 {
		actions = append(actions, "git add "+strings.Join(checkpoint.UntrackedExportPaths, " "))
	}
	if len(checkpoint.CandidatePaths) > 0 {
		actions = append(actions, fmt.Sprintf("hyard orbit checkpoint %s -m %q", packageName, "Update "+packageName))
	}
	if len(actions) > 0 {
		actions = append(actions, "hyard publish orbit "+packageName)
	}

	return sortedUniquePreserveOrder(actions)
}

func printHyardOrbitPrepareText(cmd *cobra.Command, output orbitPrepareOutput) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package: %s\nready: %t\nblocked: %t\n", output.PackageName, output.Ready, output.Blocked); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, action := range output.NextActions {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "next_action: %s\n", action); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	return nil
}

func printHyardOrbitCheckpointText(cmd *cobra.Command, output orbitCheckpointOutput) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package: %s\nready: %t\nblocked: %t\n", output.PackageName, output.Ready, output.Blocked); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if output.Committed {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "commit: %s\n", output.Commit); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(filepath.ToSlash(value))
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	return set
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := stringSet(values)
	unique := make([]string, 0, len(set))
	for value := range set {
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

func sortedUniquePreserveOrder(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
