package cli

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const (
	hyardRemoveTargetOrbit   = "orbit"
	hyardRemoveTargetHarness = "harness"

	hyardRemoveModeRuntimeCleanup = "runtime_cleanup"
	hyardRemoveModeHarnessPackage = "harness_package_remove"
)

type hyardPackageRemovalSurface struct {
	Command    string
	Action     string
	ResultVerb string
}

var (
	hyardRemoveSurface = hyardPackageRemovalSurface{
		Command:    "remove",
		ResultVerb: "removed",
	}
	hyardUninstallSurface = hyardPackageRemovalSurface{
		Command:    "uninstall",
		Action:     "uninstall",
		ResultVerb: "uninstalled",
	}
)

type hyardRemoveOutput struct {
	Action                string                      `json:"action,omitempty"`
	HarnessRoot           string                      `json:"harness_root"`
	TargetType            string                      `json:"target_type"`
	OrbitPackage          string                      `json:"orbit_package,omitempty"`
	OrbitID               string                      `json:"orbit_id,omitempty"`
	MemberSource          string                      `json:"member_source,omitempty"`
	HarnessPackage        string                      `json:"harness_package,omitempty"`
	HarnessID             string                      `json:"harness_id,omitempty"`
	RemoveMode            string                      `json:"remove_mode"`
	DryRun                bool                        `json:"dry_run"`
	OrbitPackages         []string                    `json:"orbit_packages,omitempty"`
	OrbitIDs              []string                    `json:"orbit_ids,omitempty"`
	ManifestPath          string                      `json:"manifest_path,omitempty"`
	MemberCount           int                         `json:"member_count"`
	RemainingMemberCount  int                         `json:"remaining_member_count"`
	RemovedPaths          []string                    `json:"removed_paths,omitempty"`
	RemovedPathCount      int                         `json:"removed_path_count"`
	RemovedAgentsBlock    bool                        `json:"removed_agents_block"`
	DeleteBundleRecord    bool                        `json:"delete_bundle_record"`
	DeletedBundleRecord   bool                        `json:"deleted_bundle_record"`
	AutoLeftCurrentOrbit  bool                        `json:"auto_left_current_orbit"`
	DetachedInstallRecord bool                        `json:"detached_install_record"`
	AgentCleanup          hyardAgentCleanupOutput     `json:"agent_cleanup"`
	Readiness             *harnesspkg.ReadinessReport `json:"readiness,omitempty"`
}

type hyardAgentCleanupOutput struct {
	Status               string   `json:"status"`
	RemovedOutputs       []string `json:"removed_outputs,omitempty"`
	RecompiledOutputs    []string `json:"recompiled_outputs,omitempty"`
	GlobalOutputsTouched []string `json:"global_outputs_touched,omitempty"`
	BlockedOutputs       []string `json:"blocked_outputs,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
}

func newRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <package>",
		Short: "Remove an orbit or harness package from the current runtime",
		Long: "Remove an orbit or harness package from the current runtime through the canonical hyard user surface.\n" +
			"Use `hyard remove orbit <name>` or `hyard remove harness <name>` when a package name is ambiguous.",
		Example: "" +
			"  hyard remove docs\n" +
			"  hyard remove orbit docs\n" +
			"  hyard remove harness frontend-lab\n" +
			"  hyard remove harness frontend-lab --dry-run\n" +
			"  hyard remove harness frontend-lab --yes --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHyardRemoveAuto(cmd, args[0], hyardRemoveSurface)
		},
	}
	cmd.PersistentFlags().Bool("json", false, "Output machine-readable JSON")
	cmd.PersistentFlags().Bool("dry-run", false, "Preview harness package removal without applying")
	cmd.PersistentFlags().Bool("yes", false, "Confirm package removal and user-level agent cleanup without prompting")
	cmd.AddCommand(newRemoveOrbitCommand(), newRemoveHarnessCommand())

	return cmd
}

func newUninstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall <package>",
		Short: "Uninstall a package from the current runtime",
		Long: "Uninstall a package from the current runtime through the canonical hyard package lifecycle surface.\n" +
			"Use `hyard uninstall orbit <name>` or `hyard uninstall harness <name>` when a package name is ambiguous.",
		Example: "" +
			"  hyard uninstall docs\n" +
			"  hyard uninstall orbit docs\n" +
			"  hyard uninstall harness frontend-lab\n" +
			"  hyard uninstall harness frontend-lab --dry-run\n" +
			"  hyard uninstall harness frontend-lab --yes --json\n" +
			"  hyard uninstall orbit docs --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHyardRemoveAuto(cmd, args[0], hyardUninstallSurface)
		},
	}
	cmd.PersistentFlags().Bool("json", false, "Output machine-readable JSON")
	cmd.PersistentFlags().Bool("dry-run", false, "Preview package uninstallation without applying when supported")
	cmd.PersistentFlags().Bool("yes", false, "Confirm package uninstallation and user-level agent cleanup without prompting")
	cmd.AddCommand(newUninstallOrbitCommand(), newUninstallHarnessCommand())

	return cmd
}

func newRemoveOrbitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "orbit <orbit-package>",
		Short: "Remove one orbit package from the current runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			orbitPackage, err := parseHyardRemovePackageName(args[0])
			if err != nil {
				return err
			}
			return runHyardRemoveOrbit(cmd, orbitPackage, hyardRemoveSurface)
		},
	}
}

func newUninstallOrbitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "orbit <orbit-package>",
		Short: "Uninstall one Orbit Package from the current runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			orbitPackage, err := parseHyardPackageRemovalName(args[0], hyardUninstallSurface.Command)
			if err != nil {
				return err
			}
			return runHyardRemoveOrbit(cmd, orbitPackage, hyardUninstallSurface)
		},
	}
}

func newRemoveHarnessCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "harness <harness-package>",
		Short: "Remove one harness package from the current runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			harnessPackage, err := parseHyardRemovePackageName(args[0])
			if err != nil {
				return err
			}
			return runHyardRemoveHarness(cmd, harnessPackage, hyardRemoveSurface)
		},
	}
}

func newUninstallHarnessCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "harness <harness-package>",
		Short: "Uninstall one Harness Package from the current runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			harnessPackage, err := parseHyardPackageRemovalName(args[0], hyardUninstallSurface.Command)
			if err != nil {
				return err
			}
			return runHyardRemoveHarness(cmd, harnessPackage, hyardUninstallSurface)
		},
	}
}

func runHyardRemoveAuto(cmd *cobra.Command, rawPackage string, surface hyardPackageRemovalSurface) error {
	packageName, err := parseHyardPackageRemovalName(rawPackage, surface.Command)
	if err != nil {
		return err
	}
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return err
	}
	resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
	if err != nil {
		return fmt.Errorf("resolve harness root: %w", err)
	}

	matchesOrbit := runtimeHasOrbitPackage(resolved.Runtime, packageName)
	matchesHarness, err := runtimeHasHarnessPackage(resolved.Repo.Root, resolved.Runtime, packageName)
	if err != nil {
		return err
	}
	switch {
	case matchesOrbit && matchesHarness:
		return fmt.Errorf(
			"%s target %q is ambiguous; use `hyard %s orbit %s` or `hyard %s harness %s`",
			surface.Command,
			packageName,
			surface.Command,
			packageName,
			surface.Command,
			packageName,
		)
	case matchesOrbit:
		return runHyardRemoveOrbitWithResolvedRoot(cmd, resolved, packageName, surface)
	case matchesHarness:
		return runHyardRemoveHarnessWithResolvedRoot(cmd, resolved, packageName, surface)
	default:
		return fmt.Errorf(
			"%s target %q was not found in the current runtime; use `hyard %s orbit %s` or `hyard %s harness %s` when the package type is known",
			surface.Command,
			packageName,
			surface.Command,
			packageName,
			surface.Command,
			packageName,
		)
	}
}

func runHyardRemoveOrbit(cmd *cobra.Command, orbitPackage string, surface hyardPackageRemovalSurface) error {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return err
	}
	resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
	if err != nil {
		return fmt.Errorf("resolve harness root: %w", err)
	}
	return runHyardRemoveOrbitWithResolvedRoot(cmd, resolved, orbitPackage, surface)
}

func runHyardRemoveOrbitWithResolvedRoot(cmd *cobra.Command, resolved harnesspkg.ResolvedRoot, orbitPackage string, surface hyardPackageRemovalSurface) error {
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return fmt.Errorf("read --dry-run flag: %w", err)
	}
	if dryRun {
		return fmt.Errorf("%s orbit --dry-run is not supported; use `hyard %s harness <name> --dry-run` for harness package previews", surface.Command, surface.Command)
	}
	jsonOutput, err := wantHyardJSON(cmd)
	if err != nil {
		return err
	}
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return fmt.Errorf("read --yes flag: %w", err)
	}
	memberSource := runtimeMemberSource(resolved.Runtime, orbitPackage)

	result, err := harnesspkg.RemoveRuntimeMemberWithOptions(cmd.Context(), resolved.Repo, orbitPackage, time.Now().UTC(), harnesspkg.RemoveRuntimeMemberOptions{
		AllowGlobalAgentCleanup: yes,
	})
	if err != nil {
		return fmt.Errorf("%s orbit package: %w", surface.Command, err)
	}
	readiness, err := harnesspkg.EvaluateRuntimeReadiness(cmd.Context(), resolved.Repo.Root)
	if err != nil {
		return fmt.Errorf("evaluate harness readiness: %w", err)
	}
	output := hyardRemoveOutput{
		HarnessRoot:           resolved.Repo.Root,
		TargetType:            hyardRemoveTargetOrbit,
		OrbitPackage:          orbitPackage,
		OrbitID:               orbitPackage,
		RemoveMode:            hyardRemoveModeRuntimeCleanup,
		MemberCount:           len(result.Runtime.Members),
		RemainingMemberCount:  len(result.Runtime.Members),
		ManifestPath:          result.ManifestPath,
		RemovedPaths:          result.RemovedPaths,
		RemovedPathCount:      len(result.RemovedPaths),
		RemovedAgentsBlock:    result.RemovedAgentsBlock,
		AutoLeftCurrentOrbit:  result.AutoLeftCurrentOrbit,
		DetachedInstallRecord: result.DetachedInstallRecord,
		AgentCleanup:          hyardAgentCleanupFromHarness(result.AgentCleanup),
		Readiness:             &readiness,
	}
	if surface.Action != "" {
		output.Action = surface.Action
		output.MemberSource = memberSource
	}
	if jsonOutput {
		return emitHyardJSON(cmd, output)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s orbit package %s from %s\n", surface.ResultVerb, orbitPackage, resolved.Repo.Root); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if surface.Action == hyardUninstallSurface.Action && memberSource == harnesspkg.MemberSourceManual {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "member_source: manual"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if err := writeHyardPostActionReadinessText(cmd, readiness); err != nil {
		return err
	}

	return nil
}

func runHyardRemoveHarness(cmd *cobra.Command, harnessPackage string, surface hyardPackageRemovalSurface) error {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return err
	}
	resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
	if err != nil {
		return fmt.Errorf("resolve harness root: %w", err)
	}
	return runHyardRemoveHarnessWithResolvedRoot(cmd, resolved, harnessPackage, surface)
}

func runHyardRemoveHarnessWithResolvedRoot(cmd *cobra.Command, resolved harnesspkg.ResolvedRoot, harnessPackage string, surface hyardPackageRemovalSurface) error {
	jsonOutput, err := wantHyardJSON(cmd)
	if err != nil {
		return err
	}
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return fmt.Errorf("read --dry-run flag: %w", err)
	}
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return fmt.Errorf("read --yes flag: %w", err)
	}
	if jsonOutput && !dryRun && !yes {
		return fmt.Errorf("%s harness --json requires --yes or --dry-run", surface.Command)
	}

	plan, err := harnesspkg.BuildRemoveRuntimeHarnessPackagePlanWithOptions(cmd.Context(), resolved.Repo, harnessPackage, harnesspkg.RemoveRuntimeHarnessPackageOptions{
		AllowGlobalAgentCleanup: yes,
	})
	if err != nil {
		return fmt.Errorf("plan harness package %s: %w", surface.Command, err)
	}
	if dryRun {
		output := hyardRemoveOutputFromHarnessPlan(resolved.Repo.Root, plan, true)
		applyHyardRemoveSurface(&output, surface)
		if jsonOutput {
			return emitHyardJSON(cmd, output)
		}
		return writeHyardRemoveHarnessPlan(cmd, plan, true, surface)
	}

	allowGlobalAgentCleanup := yes
	if !yes {
		if err := writeHyardRemoveHarnessPlan(cmd, plan, false, surface); err != nil {
			return err
		}
		prompter := orbittemplate.LineConfirmPrompter{
			Reader: cmd.InOrStdin(),
			Writer: cmd.ErrOrStderr(),
		}
		confirmed, err := prompter.Confirm(cmd.Context(), "Continue? [y/N] ")
		if err != nil {
			return fmt.Errorf("confirm harness package %s: %w", surface.Command, err)
		}
		if !confirmed {
			return fmt.Errorf("%s canceled for harness package %q", surface.Command, harnessPackage)
		}
		allowGlobalAgentCleanup = true
	}

	result, err := harnesspkg.ApplyRemoveRuntimeHarnessPackagePlanWithOptions(cmd.Context(), resolved.Repo, plan, time.Now().UTC(), harnesspkg.RemoveRuntimeHarnessPackageOptions{
		AllowGlobalAgentCleanup: allowGlobalAgentCleanup,
	})
	if err != nil {
		return fmt.Errorf("%s harness package: %w", surface.Command, err)
	}
	output := hyardRemoveOutputFromHarnessResult(resolved.Repo.Root, result)
	applyHyardRemoveSurface(&output, surface)
	readiness, err := harnesspkg.EvaluateRuntimeReadiness(cmd.Context(), resolved.Repo.Root)
	if err != nil {
		return fmt.Errorf("evaluate harness readiness: %w", err)
	}
	output.Readiness = &readiness
	if jsonOutput {
		return emitHyardJSON(cmd, output)
	}
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"%s harness package %s from %s\n%s_orbits: %s\n",
		surface.ResultVerb,
		harnessPackage,
		resolved.Repo.Root,
		surface.ResultVerb,
		strings.Join(result.OrbitIDs, ", "),
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := writeHyardPostActionReadinessText(cmd, readiness); err != nil {
		return err
	}

	return nil
}

func hyardRemoveOutputFromHarnessPlan(repoRoot string, plan harnesspkg.RemoveRuntimeHarnessPackagePlan, dryRun bool) hyardRemoveOutput {
	remaining := len(plan.Runtime.Members) - len(plan.OrbitIDs)
	if remaining < 0 {
		remaining = 0
	}
	return hyardRemoveOutput{
		HarnessRoot:          repoRoot,
		TargetType:           hyardRemoveTargetHarness,
		HarnessPackage:       plan.HarnessID,
		HarnessID:            plan.HarnessID,
		RemoveMode:           hyardRemoveModeHarnessPackage,
		DryRun:               dryRun,
		OrbitPackages:        append([]string(nil), plan.OrbitIDs...),
		OrbitIDs:             append([]string(nil), plan.OrbitIDs...),
		MemberCount:          len(plan.Runtime.Members),
		RemainingMemberCount: remaining,
		RemovedPaths:         append([]string(nil), plan.RemovedPaths...),
		RemovedPathCount:     len(plan.RemovedPaths),
		RemovedAgentsBlock:   plan.RemoveRootAgents,
		DeleteBundleRecord:   plan.DeleteBundleRecord,
		DeletedBundleRecord:  false,
		AgentCleanup:         hyardAgentCleanupFromHarness(plan.AgentCleanup),
	}
}

func hyardRemoveOutputFromHarnessResult(repoRoot string, result harnesspkg.RemoveRuntimeHarnessPackageResult) hyardRemoveOutput {
	return hyardRemoveOutput{
		HarnessRoot:           repoRoot,
		TargetType:            hyardRemoveTargetHarness,
		HarnessPackage:        result.HarnessID,
		HarnessID:             result.HarnessID,
		RemoveMode:            hyardRemoveModeHarnessPackage,
		OrbitPackages:         append([]string(nil), result.OrbitIDs...),
		OrbitIDs:              append([]string(nil), result.OrbitIDs...),
		ManifestPath:          result.ManifestPath,
		MemberCount:           len(result.Runtime.Members),
		RemainingMemberCount:  len(result.Runtime.Members),
		RemovedPaths:          append([]string(nil), result.RemovedPaths...),
		RemovedPathCount:      len(result.RemovedPaths),
		RemovedAgentsBlock:    result.RemovedAgentsBlock,
		DeleteBundleRecord:    result.DeletedBundleRecord,
		DeletedBundleRecord:   result.DeletedBundleRecord,
		AutoLeftCurrentOrbit:  result.AutoLeftCurrentOrbit,
		DetachedInstallRecord: false,
		AgentCleanup:          hyardAgentCleanupFromHarness(result.AgentCleanup),
	}
}

func applyHyardRemoveSurface(output *hyardRemoveOutput, surface hyardPackageRemovalSurface) {
	if surface.Action != "" {
		output.Action = surface.Action
	}
}

func hyardAgentCleanupFromHarness(cleanup harnesspkg.AgentCleanupResult) hyardAgentCleanupOutput {
	status := cleanup.Status
	if status == "" {
		status = harnesspkg.AgentCleanupStatusNotNeeded
	}

	return hyardAgentCleanupOutput{
		Status:               status,
		RemovedOutputs:       append([]string(nil), cleanup.RemovedOutputs...),
		RecompiledOutputs:    append([]string(nil), cleanup.RecompiledOutputs...),
		GlobalOutputsTouched: append([]string(nil), cleanup.GlobalOutputsTouched...),
		BlockedOutputs:       append([]string(nil), cleanup.BlockedOutputs...),
		Warnings:             append([]string(nil), cleanup.Warnings...),
	}
}

func writeHyardPostActionReadinessText(cmd *cobra.Command, report harnesspkg.ReadinessReport) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "readiness_status: %s\n", report.Status); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if report.Status != harnesspkg.ReadinessStatusReady {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "readiness_hint: run `hyard ready` for detailed readiness reasons"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, step := range report.NextSteps {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "next_step: %s intent=%s\n", step.Command, step.Intent); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func writeHyardRemoveHarnessPlan(cmd *cobra.Command, plan harnesspkg.RemoveRuntimeHarnessPackagePlan, dryRun bool, surface hyardPackageRemovalSurface) error {
	verbTitle := titleHyardSurfaceCommand(surface)
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s harness package %s?\n", verbTitle, plan.HarnessID); err != nil {
		return fmt.Errorf("write harness remove preview: %w", err)
	}
	if dryRun {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "dry_run: true"); err != nil {
			return fmt.Errorf("write harness remove preview: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Orbits to %s:\n", surface.Command); err != nil {
		return fmt.Errorf("write harness remove preview: %w", err)
	}
	for _, orbitID := range plan.OrbitIDs {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", orbitID); err != nil {
			return fmt.Errorf("write harness remove preview: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "paths_to_%s: %d\n", surface.Command, len(plan.RemovedPaths)); err != nil {
		return fmt.Errorf("write harness remove preview: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "delete_bundle_record: %t\n", plan.DeleteBundleRecord); err != nil {
		return fmt.Errorf("write harness remove preview: %w", err)
	}
	if plan.AgentCleanup.Status != "" && plan.AgentCleanup.Status != harnesspkg.AgentCleanupStatusNotNeeded {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent_cleanup: %s\n", plan.AgentCleanup.Status); err != nil {
			return fmt.Errorf("write harness remove preview: %w", err)
		}
		for _, path := range plan.AgentCleanup.GlobalOutputsTouched {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  user_level_output: %s\n", path); err != nil {
				return fmt.Errorf("write harness remove preview: %w", err)
			}
		}
	}

	return nil
}

func titleHyardSurfaceCommand(surface hyardPackageRemovalSurface) string {
	if surface.Command == "" {
		return ""
	}
	runes := []rune(surface.Command)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}

func parseHyardRemovePackageName(raw string) (string, error) {
	return parseHyardPackageRemovalName(raw, hyardRemoveSurface.Command)
}

func parseHyardPackageRemovalName(raw string, command string) (string, error) {
	coordinate, err := parseHyardPackageCoordinate(raw)
	if err != nil {
		return "", err
	}
	if coordinate.String() != coordinate.Name {
		return "", fmt.Errorf("%s package %q must use the installed package name", command, coordinate.String())
	}

	return coordinate.Name, nil
}

func runtimeMemberSource(runtimeFile harnesspkg.RuntimeFile, orbitPackage string) string {
	for _, member := range runtimeFile.Members {
		if member.OrbitID == orbitPackage {
			return member.Source
		}
	}

	return ""
}

func runtimeHasOrbitPackage(runtimeFile harnesspkg.RuntimeFile, packageName string) bool {
	for _, member := range runtimeFile.Members {
		if member.OrbitID == packageName {
			return true
		}
	}

	return false
}

func runtimeHasHarnessPackage(repoRoot string, runtimeFile harnesspkg.RuntimeFile, packageName string) (bool, error) {
	bundleIDs, err := harnesspkg.ListBundleRecordIDs(repoRoot)
	if err != nil {
		return false, fmt.Errorf("list installed harness packages: %w", err)
	}
	for _, bundleID := range bundleIDs {
		if bundleID == packageName {
			return true, nil
		}
	}
	for _, member := range runtimeFile.Members {
		if member.OwnerHarnessID == packageName {
			return true, nil
		}
	}

	return false, nil
}
