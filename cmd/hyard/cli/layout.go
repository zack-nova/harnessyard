package cli

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const layoutOptimizeSchemaVersion = "1.0"

var errLayoutNoPathRewriteMatch = errors.New("no include pattern matched")

type layoutOptimizeOutput struct {
	SchemaVersion  string                  `json:"schema_version"`
	RepoRoot       string                  `json:"repo_root"`
	Mode           string                  `json:"mode"`
	RepositoryMode string                  `json:"repository_mode"`
	MovePlan       layoutOptimizePlan      `json:"move_plan"`
	Check          *harnesspkg.CheckResult `json:"check,omitempty"`
}

type layoutOptimizePlan struct {
	OrbitID   string                  `json:"orbit_id"`
	Moves     []layoutOptimizeMove    `json:"moves"`
	Conflicts []layoutOptimizeProblem `json:"conflicts"`
	Warnings  []layoutOptimizeProblem `json:"warnings"`
}

type layoutOptimizeMove struct {
	From                 string                  `json:"from"`
	To                   string                  `json:"to"`
	Reason               string                  `json:"reason"`
	AffectedTruthUpdates []layoutTruthUpdate     `json:"affected_truth_updates"`
	Conflicts            []layoutOptimizeProblem `json:"conflicts"`
	Warnings             []layoutOptimizeProblem `json:"warnings"`
}

type layoutTruthUpdate struct {
	Path  string `json:"path"`
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

type layoutOptimizeProblem struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func newLayoutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "layout",
		Short: "Inspect and optimize Harness Yard repository layout",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newLayoutOptimizeCommand())

	return cmd
}

func newLayoutOptimizeCommand() *cobra.Command {
	var check bool
	var orbitID string
	var yes bool

	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Optimize Harness Yard-friendly file placement",
		Long: "Optimize Harness Yard-friendly file placement across adopted member candidates,\n" +
			"agent assets, and existing Harness Runtime truth.",
		Example: "" +
			"  hyard layout optimize --check --json\n" +
			"  hyard layout optimize --yes --json\n" +
			"  hyard layout optimize --check --json --orbit docs\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if check {
				if yes {
					return fmt.Errorf("--yes cannot be used with --check")
				}
				output, err := buildLayoutOptimizeCheckOutput(cmd, orbitID)
				if err != nil {
					return err
				}
				if jsonOutput {
					return emitHyardJSON(cmd, output)
				}

				return printLayoutOptimizeText(cmd, output)
			}

			output, err := runLayoutOptimizeApply(cmd, orbitID, yes)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			return printLayoutOptimizeText(cmd, output)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Preview layout recommendations without mutating")
	cmd.Flags().BoolVar(&yes, "yes", false, "Apply default recommendations without prompting")
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Orbit id to use for Ordinary Repository preview or to filter a Harness Runtime")
	addHyardJSONFlag(cmd)

	return cmd
}

func runLayoutOptimizeApply(cmd *cobra.Command, explicitOrbitID string, yes bool) (layoutOptimizeOutput, error) {
	output, err := buildLayoutOptimizeCheckOutput(cmd, explicitOrbitID)
	if err != nil {
		return layoutOptimizeOutput{}, err
	}
	output.Mode = "apply"

	if len(output.MovePlan.Conflicts) > 0 {
		return layoutOptimizeOutput{}, formatLayoutOptimizeBlockedError(output.MovePlan)
	}
	if len(output.MovePlan.Moves) == 0 {
		return output, nil
	}
	if !yes {
		if err := printLayoutOptimizeText(cmd, output); err != nil {
			return layoutOptimizeOutput{}, err
		}
		prompter := orbittemplate.LineConfirmPrompter{
			Reader: cmd.InOrStdin(),
			Writer: cmd.ErrOrStderr(),
		}
		confirmed, err := prompter.Confirm(cmd.Context(), "Apply layout optimization moves? [y/N] ")
		if err != nil {
			return layoutOptimizeOutput{}, fmt.Errorf("confirm layout optimization: %w", err)
		}
		if !confirmed {
			return layoutOptimizeOutput{}, fmt.Errorf("layout optimize canceled")
		}
	}

	if output.RepositoryMode == "ordinary_repository" {
		if _, err := buildAdoptWriteOutput(cmd, explicitOrbitID); err != nil {
			return layoutOptimizeOutput{}, fmt.Errorf("adopt ordinary repository before layout optimization: %w", err)
		}
	}
	if err := applyLayoutOptimizePlan(cmd, output); err != nil {
		return layoutOptimizeOutput{}, err
	}
	checkResult, err := harnesspkg.CheckRuntime(cmd.Context(), output.RepoRoot)
	if err != nil {
		return layoutOptimizeOutput{}, fmt.Errorf("validate updated Harness Yard truth: %w", err)
	}
	if !checkResult.OK {
		return layoutOptimizeOutput{}, fmt.Errorf("validate updated Harness Yard truth: harness check reported %d findings", checkResult.FindingCount)
	}
	output.Check = &checkResult

	return output, nil
}

func formatLayoutOptimizeBlockedError(plan layoutOptimizePlan) error {
	if len(plan.Conflicts) == 0 {
		return nil
	}
	first := plan.Conflicts[0]
	if first.Path != "" {
		return fmt.Errorf("layout optimize blocked by %d conflict(s): %s at %s: %s", len(plan.Conflicts), first.Code, first.Path, first.Message)
	}

	return fmt.Errorf("layout optimize blocked by %d conflict(s): %s: %s", len(plan.Conflicts), first.Code, first.Message)
}

func buildLayoutOptimizeCheckOutput(cmd *cobra.Command, explicitOrbitID string) (layoutOptimizeOutput, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return layoutOptimizeOutput{}, err
	}
	repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
	if err != nil {
		return layoutOptimizeOutput{}, fmt.Errorf("discover git repository: %w", err)
	}

	manifest, manifestErr := harnesspkg.LoadManifestFile(repo.Root)
	if manifestErr == nil && manifest.Kind == harnesspkg.ManifestKindRuntime {
		plan, err := buildRuntimeLayoutOptimizePlan(cmd, repo.Root, manifest, explicitOrbitID)
		if err != nil {
			return layoutOptimizeOutput{}, err
		}
		return layoutOptimizeOutput{
			SchemaVersion:  layoutOptimizeSchemaVersion,
			RepoRoot:       repo.Root,
			Mode:           "check",
			RepositoryMode: "harness_runtime",
			MovePlan:       plan,
		}, nil
	}
	if manifestErr != nil && !errors.Is(manifestErr, os.ErrNotExist) {
		return layoutOptimizeOutput{}, fmt.Errorf("inspect harness manifest: %w", manifestErr)
	}

	plan, err := buildOrdinaryLayoutOptimizePlan(cmd, explicitOrbitID)
	if err != nil {
		return layoutOptimizeOutput{}, err
	}
	return layoutOptimizeOutput{
		SchemaVersion:  layoutOptimizeSchemaVersion,
		RepoRoot:       repo.Root,
		Mode:           "check",
		RepositoryMode: "ordinary_repository",
		MovePlan:       plan,
	}, nil
}

func buildOrdinaryLayoutOptimizePlan(cmd *cobra.Command, explicitOrbitID string) (layoutOptimizePlan, error) {
	adoptionPreview, err := buildAdoptCheckOutput(cmd, explicitOrbitID)
	if err != nil {
		return layoutOptimizePlan{}, err
	}

	return buildAdoptionLayoutOptimizePlan(cmd, adoptionPreview)
}

func buildAdoptionLayoutOptimizePlan(_ *cobra.Command, adoptionPreview adoptCheckOutput) (layoutOptimizePlan, error) {
	plan := newLayoutOptimizePlan(adoptionPreview.AdoptedOrbit.ID)
	for _, diagnostic := range adoptionPreview.Diagnostics {
		if diagnostic.Severity == "error" {
			plan.Conflicts = append(plan.Conflicts, layoutProblemFromAdoptDiagnostic(diagnostic))
			continue
		}
		plan.Warnings = append(plan.Warnings, layoutProblemFromAdoptDiagnostic(diagnostic))
	}

	for _, candidate := range adoptionPreview.Candidates {
		switch candidate.Kind {
		case "local_skill_capability":
			name := layoutCandidateEvidenceDetail(candidate, "codex_skill_root")
			if strings.TrimSpace(name) == "" {
				name = path.Base(candidate.Path)
			}
			plan.Moves = append(plan.Moves, layoutMoveIfDifferent(
				candidate.Path,
				path.Join("skills", adoptionPreview.AdoptedOrbit.ID, name),
				"local_skill_recommended_position",
				layoutOrbitTruthPath(adoptionPreview.AdoptedOrbit.ID),
				"capabilities.skills.local.paths.include",
			)...)
		case "prompt_command_capability":
			plan.Moves = append(plan.Moves, layoutMoveIfDifferent(
				candidate.Path,
				layoutRecommendedCommandPath(adoptionPreview.AdoptedOrbit.ID, candidate.Path),
				"prompt_command_recommended_position",
				layoutOrbitTruthPath(adoptionPreview.AdoptedOrbit.ID),
				"capabilities.commands.paths.include",
			)...)
		case "referenced_guidance_document":
			plan.Moves = append(plan.Moves, layoutMoveIfDifferent(
				candidate.Path,
				layoutRecommendedGuidancePath(adoptionPreview.AdoptedOrbit.ID, candidate.Path),
				"referenced_guidance_recommended_position",
				layoutOrbitTruthPath(adoptionPreview.AdoptedOrbit.ID),
				"members[].paths.include",
			)...)
		case "codex_hook_handler":
			plan.Moves = append(plan.Moves, layoutMoveIfDifferent(
				candidate.Path,
				layoutRecommendedHookHandlerPath(adoptionPreview.AdoptedOrbit.ID, candidate.Path),
				"hook_handler_recommended_position",
				layoutOrbitTruthPath(adoptionPreview.AdoptedOrbit.ID),
				"members[].paths.include",
			)...)
		}
	}

	finalizeLayoutOptimizePlan(adoptionPreview.RepoRoot, &plan)

	return plan, nil
}

func buildRuntimeLayoutOptimizePlan(
	cmd *cobra.Command,
	repoRoot string,
	manifest harnesspkg.ManifestFile,
	explicitOrbitID string,
) (layoutOptimizePlan, error) {
	trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), repoRoot)
	if err != nil {
		return layoutOptimizePlan{}, fmt.Errorf("load tracked files for runtime layout preview: %w", err)
	}

	orbitIDs := layoutRuntimeOrbitIDs(manifest, explicitOrbitID)
	planOrbitID := ""
	if len(orbitIDs) == 1 {
		planOrbitID = orbitIDs[0]
	}
	plan := newLayoutOptimizePlan(planOrbitID)
	for _, orbitID := range orbitIDs {
		spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repoRoot, orbitID)
		if err != nil {
			return layoutOptimizePlan{}, fmt.Errorf("load hosted orbit spec %q: %w", orbitID, err)
		}

		commands, err := orbitpkg.ResolveCommandCapabilities(spec, trackedFiles, trackedFiles)
		if err != nil {
			return layoutOptimizePlan{}, fmt.Errorf("resolve command capabilities for %q: %w", orbitID, err)
		}
		for _, command := range commands {
			plan.Moves = append(plan.Moves, layoutMoveIfDifferent(
				command.Path,
				layoutRecommendedCommandPath(orbitID, command.Path),
				"prompt_command_recommended_position",
				layoutOrbitTruthPath(orbitID),
				"capabilities.commands.paths.include",
			)...)
		}

		localSkills, err := orbitpkg.ResolveLocalSkillCapabilities(repoRoot, spec, trackedFiles, trackedFiles)
		if err != nil {
			return layoutOptimizePlan{}, fmt.Errorf("resolve local skill capabilities for %q: %w", orbitID, err)
		}
		for _, skill := range localSkills {
			plan.Moves = append(plan.Moves, layoutMoveIfDifferent(
				skill.RootPath,
				path.Join("skills", orbitID, skill.Name),
				"local_skill_recommended_position",
				layoutOrbitTruthPath(orbitID),
				"capabilities.skills.local.paths.include",
			)...)
		}

		hookHandlerPaths := map[string]struct{}{}
		hooks, err := orbitpkg.ResolveAgentAddonHooks(spec, trackedFiles, trackedFiles)
		if err != nil {
			return layoutOptimizePlan{}, fmt.Errorf("resolve agent add-on hooks for %q: %w", orbitID, err)
		}
		for _, hook := range hooks {
			hookHandlerPaths[hook.HandlerPath] = struct{}{}
			moves := layoutMoveIfDifferent(
				hook.HandlerPath,
				layoutRecommendedHookHandlerPath(orbitID, hook.HandlerPath),
				"hook_handler_recommended_position",
				layoutOrbitTruthPath(orbitID),
				"agent_addons.hooks.entries[].handler.path",
			)
			for index := range moves {
				if layoutSpecMemberIncludesPath(spec, hook.HandlerPath) {
					moves[index].AffectedTruthUpdates = append(moves[index].AffectedTruthUpdates, layoutTruthUpdate{
						Path:  layoutOrbitTruthPath(orbitID),
						Field: "members[].paths.include",
						From:  hook.HandlerPath,
						To:    moves[index].To,
					})
				}
			}
			plan.Moves = append(plan.Moves, moves...)
		}

		for _, member := range spec.Members {
			if len(member.Paths.Include) != 1 || len(member.Paths.Exclude) > 0 {
				continue
			}
			fromPath := strings.TrimSuffix(member.Paths.Include[0], "/**")
			if _, ok := hookHandlerPaths[fromPath]; ok {
				continue
			}
			plan.Moves = append(plan.Moves, layoutMoveIfDifferent(
				fromPath,
				layoutRecommendedGuidancePath(orbitID, fromPath),
				"member_recommended_position",
				layoutOrbitTruthPath(orbitID),
				"members[].paths.include",
			)...)
		}
	}

	finalizeLayoutOptimizePlan(repoRoot, &plan)

	return plan, nil
}

func newLayoutOptimizePlan(orbitID string) layoutOptimizePlan {
	return layoutOptimizePlan{
		OrbitID:   orbitID,
		Moves:     []layoutOptimizeMove{},
		Conflicts: []layoutOptimizeProblem{},
		Warnings:  []layoutOptimizeProblem{},
	}
}

func layoutMoveIfDifferent(fromPath string, toPath string, reason string, truthPath string, truthField string) []layoutOptimizeMove {
	if fromPath == toPath {
		return nil
	}
	return []layoutOptimizeMove{
		{
			From:   fromPath,
			To:     toPath,
			Reason: reason,
			AffectedTruthUpdates: []layoutTruthUpdate{
				{
					Path:  truthPath,
					Field: truthField,
					From:  fromPath,
					To:    toPath,
				},
			},
			Conflicts: []layoutOptimizeProblem{},
			Warnings:  []layoutOptimizeProblem{},
		},
	}
}

func finalizeLayoutOptimizePlan(repoRoot string, plan *layoutOptimizePlan) {
	sort.Slice(plan.Moves, func(left, right int) bool {
		if plan.Moves[left].From == plan.Moves[right].From {
			return plan.Moves[left].To < plan.Moves[right].To
		}
		return plan.Moves[left].From < plan.Moves[right].From
	})

	seenDestinations := map[string]int{}
	for index := range plan.Moves {
		move := &plan.Moves[index]
		if !layoutSourceExists(repoRoot, move.From) {
			conflict := layoutOptimizeProblem{
				Code:    "source_missing",
				Path:    move.From,
				Message: "recommended move source is missing",
			}
			move.Conflicts = append(move.Conflicts, conflict)
			plan.Conflicts = append(plan.Conflicts, conflict)
		}
		if layoutDestinationExists(repoRoot, move.From, move.To) {
			conflict := layoutOptimizeProblem{
				Code:    "destination_exists",
				Path:    move.To,
				Message: "recommended destination already exists",
			}
			move.Conflicts = append(move.Conflicts, conflict)
			plan.Conflicts = append(plan.Conflicts, conflict)
		}
		if firstIndex, ok := seenDestinations[move.To]; ok {
			conflict := layoutOptimizeProblem{
				Code:    "duplicate_destination",
				Path:    move.To,
				Message: "multiple recommendations target the same destination",
			}
			move.Conflicts = append(move.Conflicts, conflict)
			plan.Moves[firstIndex].Conflicts = append(plan.Moves[firstIndex].Conflicts, conflict)
			plan.Conflicts = append(plan.Conflicts, conflict)
			continue
		}
		seenDestinations[move.To] = index
	}
	for left := range plan.Moves {
		for right := left + 1; right < len(plan.Moves); right++ {
			if !layoutPathsOverlap(plan.Moves[left].From, plan.Moves[right].From) {
				continue
			}
			pathValue := plan.Moves[right].From
			if len(plan.Moves[left].From) > len(plan.Moves[right].From) {
				pathValue = plan.Moves[left].From
			}
			conflict := layoutOptimizeProblem{
				Code:    "overlapping_moves",
				Path:    pathValue,
				Message: "multiple recommendations move overlapping source paths",
			}
			plan.Moves[left].Conflicts = append(plan.Moves[left].Conflicts, conflict)
			plan.Moves[right].Conflicts = append(plan.Moves[right].Conflicts, conflict)
			plan.Conflicts = append(plan.Conflicts, conflict)
		}
	}
}

func layoutSourceExists(repoRoot string, fromPath string) bool {
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(fromPath))); err == nil {
		return true
	}

	return false
}

func layoutDestinationExists(repoRoot string, fromPath string, toPath string) bool {
	if fromPath == toPath {
		return false
	}
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(toPath))); err == nil {
		return true
	}

	return false
}

func layoutPathsOverlap(left string, right string) bool {
	if left == right {
		return true
	}
	return strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func layoutSpecMemberIncludesPath(spec orbitpkg.OrbitSpec, pathValue string) bool {
	for _, member := range spec.Members {
		if layoutPathsMatchPath(member.Paths.Include, pathValue) {
			return true
		}
	}

	return false
}

func applyLayoutOptimizePlan(cmd *cobra.Command, output layoutOptimizeOutput) error {
	allowCreateMissingTruth := output.RepositoryMode == "ordinary_repository"
	if err := validateLayoutOptimizeTruthUpdates(cmd, output.RepoRoot, output.MovePlan, allowCreateMissingTruth); err != nil {
		return err
	}
	if err := applyLayoutOptimizeMoves(cmd, output); err != nil {
		return err
	}
	if err := applyLayoutOptimizeTruthUpdates(cmd, output.RepoRoot, output.MovePlan, allowCreateMissingTruth); err != nil {
		return err
	}
	if err := applyLayoutOptimizeAgentConfigHookPaths(output.RepoRoot, output.MovePlan); err != nil {
		return err
	}
	if err := applyLayoutOptimizeGuidanceLinks(output.RepoRoot, output.MovePlan); err != nil {
		return err
	}

	return nil
}

func validateLayoutOptimizeTruthUpdates(cmd *cobra.Command, repoRoot string, plan layoutOptimizePlan, allowCreateMissingTruth bool) error {
	if _, err := buildLayoutOptimizeUpdatedSpecs(cmd, repoRoot, plan, allowCreateMissingTruth); err != nil {
		return err
	}

	return nil
}

func applyLayoutOptimizeMoves(cmd *cobra.Command, output layoutOptimizeOutput) error {
	for _, move := range output.MovePlan.Moves {
		if err := gitpkg.MovePath(cmd.Context(), output.RepoRoot, move.From, move.To); err != nil {
			return fmt.Errorf("apply layout move %q -> %q: %w", move.From, move.To, err)
		}
	}

	return nil
}

func applyLayoutOptimizeTruthUpdates(cmd *cobra.Command, repoRoot string, plan layoutOptimizePlan, allowCreateMissingTruth bool) error {
	specs, err := buildLayoutOptimizeUpdatedSpecs(cmd, repoRoot, plan, allowCreateMissingTruth)
	if err != nil {
		return err
	}
	return writeLayoutOptimizeUpdatedSpecs(repoRoot, specs)
}

func buildLayoutOptimizeUpdatedSpecs(cmd *cobra.Command, repoRoot string, plan layoutOptimizePlan, allowCreateMissingTruth bool) (map[string]orbitpkg.OrbitSpec, error) {
	specs := map[string]orbitpkg.OrbitSpec{}
	for _, move := range plan.Moves {
		for _, update := range move.AffectedTruthUpdates {
			orbitID, err := layoutOrbitIDFromTruthPath(update.Path)
			if err != nil {
				return nil, err
			}
			spec, ok := specs[orbitID]
			if !ok {
				loaded, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repoRoot, orbitID)
				if err != nil {
					return nil, fmt.Errorf("load hosted orbit spec %q for layout update: %w", orbitID, err)
				}
				spec = loaded
			}
			updated, err := applyLayoutTruthUpdateToSpec(spec, update, allowCreateMissingTruth)
			if err != nil {
				return nil, fmt.Errorf("update %s %s: %w", update.Path, update.Field, err)
			}
			specs[orbitID] = updated
		}
	}

	return specs, nil
}

func writeLayoutOptimizeUpdatedSpecs(repoRoot string, specs map[string]orbitpkg.OrbitSpec) error {
	orbitIDs := make([]string, 0, len(specs))
	for orbitID := range specs {
		orbitIDs = append(orbitIDs, orbitID)
	}
	sort.Strings(orbitIDs)
	for _, orbitID := range orbitIDs {
		if _, err := orbitpkg.WriteHostedOrbitSpec(repoRoot, specs[orbitID]); err != nil {
			return fmt.Errorf("write hosted orbit spec %q after layout optimization: %w", orbitID, err)
		}
	}

	return nil
}

func applyLayoutOptimizeAgentConfigHookPaths(repoRoot string, plan layoutOptimizePlan) error {
	configFile, hasConfig, err := harnesspkg.LoadOptionalAgentUnifiedConfigFile(repoRoot)
	if err != nil {
		return fmt.Errorf("load agent config for layout hook path updates: %w", err)
	}
	if !hasConfig || len(configFile.Hooks.Entries) == 0 {
		return nil
	}

	changed := false
	for entryIndex := range configFile.Hooks.Entries {
		handlerPath := configFile.Hooks.Entries[entryIndex].Handler.Path
		if strings.TrimSpace(handlerPath) == "" {
			continue
		}
		updatedPath := handlerPath
		for _, move := range plan.Moves {
			nextPath, pathChanged, err := layoutRewritePathPattern(updatedPath, move.From, move.To)
			if err != nil {
				return fmt.Errorf("rewrite agent hook handler path %q: %w", handlerPath, err)
			}
			if pathChanged {
				updatedPath = nextPath
				changed = true
			}
		}
		configFile.Hooks.Entries[entryIndex].Handler.Path = updatedPath
	}
	if !changed {
		return nil
	}
	if _, err := harnesspkg.WriteAgentUnifiedConfigFile(repoRoot, configFile); err != nil {
		return fmt.Errorf("write agent config after layout hook path updates: %w", err)
	}

	return nil
}

func applyLayoutOptimizeGuidanceLinks(repoRoot string, plan layoutOptimizePlan) error {
	for _, guidancePath := range layoutRootGuidancePaths() {
		filename := filepath.Join(repoRoot, filepath.FromSlash(guidancePath))
		data, err := os.ReadFile(filename) //nolint:gosec // filename is built from the repo root and fixed root guidance paths.
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("read guidance links in %s: %w", guidancePath, err)
		}
		info, err := os.Stat(filename)
		if err != nil {
			return fmt.Errorf("stat guidance links in %s: %w", guidancePath, err)
		}

		updated := string(data)
		changed := false
		for _, move := range plan.Moves {
			next, moveChanged := layoutRewriteMarkdownLinkTargets(updated, move.From, move.To)
			if moveChanged {
				updated = next
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := os.WriteFile(filename, []byte(updated), info.Mode().Perm()); err != nil {
			return fmt.Errorf("write guidance links in %s: %w", guidancePath, err)
		}
	}

	return nil
}

func layoutRootGuidancePaths() []string {
	return []string{
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
		"HUMANS.md",
		"BOOTSTRAP.md",
	}
}

func layoutRewriteMarkdownLinkTargets(content string, fromPath string, toPath string) (string, bool) {
	rewritten := content
	for _, prefix := range []string{"", "./"} {
		rewritten = strings.ReplaceAll(rewritten, "]("+prefix+fromPath+")", "]("+toPath+")")
	}

	return rewritten, rewritten != content
}

func layoutOrbitIDFromTruthPath(truthPath string) (string, error) {
	const prefix = ".harness/orbits/"
	if !strings.HasPrefix(truthPath, prefix) || !strings.HasSuffix(truthPath, ".yaml") {
		return "", fmt.Errorf("unsupported layout truth path %q", truthPath)
	}
	orbitID := strings.TrimSuffix(strings.TrimPrefix(truthPath, prefix), ".yaml")
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("unsupported layout truth path %q: %w", truthPath, err)
	}

	return orbitID, nil
}

func applyLayoutTruthUpdateToSpec(spec orbitpkg.OrbitSpec, update layoutTruthUpdate, allowCreateMissingTruth bool) (orbitpkg.OrbitSpec, error) {
	switch update.Field {
	case "capabilities.commands.paths.include":
		if spec.Capabilities == nil {
			if !allowCreateMissingTruth {
				return spec, fmt.Errorf("command capability paths are not present")
			}
			spec.Capabilities = &orbitpkg.OrbitCapabilities{}
		}
		if spec.Capabilities.Commands == nil {
			if !allowCreateMissingTruth {
				return spec, fmt.Errorf("command capability paths are not present")
			}
			spec.Capabilities.Commands = &orbitpkg.OrbitCommandCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{update.To},
				},
			}
			return spec, nil
		}
		if len(spec.Capabilities.Commands.Paths.Include) == 0 && allowCreateMissingTruth {
			spec.Capabilities.Commands.Paths.Include = []string{update.To}
			return spec, nil
		}
		if len(spec.Capabilities.Commands.Paths.Include) == 0 {
			return spec, fmt.Errorf("command capability paths are not present")
		}
		paths, err := layoutRewritePaths(spec.Capabilities.Commands.Paths, update.From, update.To)
		if err != nil {
			if allowCreateMissingTruth && errors.Is(err, errLayoutNoPathRewriteMatch) {
				spec.Capabilities.Commands.Paths.Include = layoutAppendUniqueString(spec.Capabilities.Commands.Paths.Include, update.To)
				return spec, nil
			}
			return spec, err
		}
		spec.Capabilities.Commands.Paths = paths
	case "capabilities.skills.local.paths.include":
		if spec.Capabilities == nil {
			if !allowCreateMissingTruth {
				return spec, fmt.Errorf("local skill capability paths are not present")
			}
			spec.Capabilities = &orbitpkg.OrbitCapabilities{}
		}
		if spec.Capabilities.Skills == nil {
			if !allowCreateMissingTruth {
				return spec, fmt.Errorf("local skill capability paths are not present")
			}
			spec.Capabilities.Skills = &orbitpkg.OrbitSkillCapabilities{}
		}
		if spec.Capabilities.Skills.Local == nil {
			if !allowCreateMissingTruth {
				return spec, fmt.Errorf("local skill capability paths are not present")
			}
			spec.Capabilities.Skills.Local = &orbitpkg.OrbitLocalSkillCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{update.To},
				},
			}
			return spec, nil
		}
		if len(spec.Capabilities.Skills.Local.Paths.Include) == 0 && allowCreateMissingTruth {
			spec.Capabilities.Skills.Local.Paths.Include = []string{update.To}
			return spec, nil
		}
		if len(spec.Capabilities.Skills.Local.Paths.Include) == 0 {
			return spec, fmt.Errorf("local skill capability paths are not present")
		}
		paths, err := layoutRewritePaths(spec.Capabilities.Skills.Local.Paths, update.From, update.To)
		if err != nil {
			if allowCreateMissingTruth && errors.Is(err, errLayoutNoPathRewriteMatch) {
				spec.Capabilities.Skills.Local.Paths.Include = layoutAppendUniqueString(spec.Capabilities.Skills.Local.Paths.Include, update.To)
				return spec, nil
			}
			return spec, err
		}
		spec.Capabilities.Skills.Local.Paths = paths
	case "members[].paths.include":
		updatedAny := false
		for index := range spec.Members {
			include, changed, err := layoutRewritePathPatterns(spec.Members[index].Paths.Include, update.From, update.To)
			if err != nil {
				return spec, err
			}
			if !changed {
				continue
			}
			updatedAny = true
			spec.Members[index].Paths.Include = include
		}
		if !updatedAny && !layoutAnyMemberPathMatches(spec.Members, update.To) {
			if allowCreateMissingTruth {
				spec.Members = append(spec.Members, layoutMemberFromMoveDestination(update.To))
				return spec, nil
			}
			return spec, fmt.Errorf("no member path includes matched %q", update.From)
		}
	case "agent_addons.hooks.entries[].handler.path":
		if spec.AgentAddons == nil || spec.AgentAddons.Hooks == nil {
			return spec, fmt.Errorf("agent add-on hooks are not present")
		}
		updatedAny := false
		for index := range spec.AgentAddons.Hooks.Entries {
			rewritten, changed, err := layoutRewritePathPattern(spec.AgentAddons.Hooks.Entries[index].Handler.Path, update.From, update.To)
			if err != nil {
				return spec, err
			}
			if !changed {
				continue
			}
			spec.AgentAddons.Hooks.Entries[index].Handler.Path = rewritten
			updatedAny = true
		}
		if !updatedAny {
			for _, entry := range spec.AgentAddons.Hooks.Entries {
				if entry.Handler.Path == update.To {
					return spec, nil
				}
			}
			return spec, fmt.Errorf("no hook handler path matched %q", update.From)
		}
	default:
		return spec, fmt.Errorf("unsupported truth field %q", update.Field)
	}

	return spec, nil
}

func layoutRewritePaths(paths orbitpkg.OrbitMemberPaths, fromPath string, toPath string) (orbitpkg.OrbitMemberPaths, error) {
	include, changed, err := layoutRewritePathPatterns(paths.Include, fromPath, toPath)
	if err != nil {
		return paths, err
	}
	if !changed {
		if layoutPathsMatchPath(paths.Include, toPath) {
			return paths, nil
		}
		return paths, fmt.Errorf("%w %q", errLayoutNoPathRewriteMatch, fromPath)
	}

	updated := paths
	updated.Include = include

	return updated, nil
}

func layoutRewritePathPatterns(patterns []string, fromPath string, toPath string) ([]string, bool, error) {
	updated := append([]string(nil), patterns...)
	changed := false
	for index, pattern := range updated {
		nextPattern, patternChanged, err := layoutRewritePathPattern(pattern, fromPath, toPath)
		if err != nil {
			return nil, false, err
		}
		if !patternChanged {
			continue
		}
		updated[index] = nextPattern
		changed = true
	}
	if changed {
		updated = layoutUniqueStrings(updated)
	}

	return updated, changed, nil
}

func layoutRewritePathPattern(pattern string, fromPath string, toPath string) (string, bool, error) {
	normalizedPattern, err := ids.NormalizeRepoRelativePath(pattern)
	if err != nil {
		return "", false, fmt.Errorf("normalize pattern %q: %w", pattern, err)
	}
	normalizedFrom, err := ids.NormalizeRepoRelativePath(fromPath)
	if err != nil {
		return "", false, fmt.Errorf("normalize source path %q: %w", fromPath, err)
	}
	normalizedTo, err := ids.NormalizeRepoRelativePath(toPath)
	if err != nil {
		return "", false, fmt.Errorf("normalize destination path %q: %w", toPath, err)
	}
	matched, err := doublestar.Match(normalizedPattern, normalizedFrom)
	if err != nil {
		return "", false, fmt.Errorf("match pattern %q: %w", normalizedPattern, err)
	}
	if !matched {
		return pattern, false, nil
	}
	if normalizedPattern == normalizedFrom {
		return normalizedTo, true, nil
	}
	if strings.HasSuffix(normalizedPattern, "/**") {
		root := strings.TrimSuffix(normalizedPattern, "/**")
		if root == normalizedFrom {
			return normalizedTo + "/**", true, nil
		}
	}
	globIndex := strings.IndexAny(normalizedPattern, "*?[")
	if globIndex < 0 {
		return normalizedTo, true, nil
	}
	lastSlashBeforeGlob := strings.LastIndex(normalizedPattern[:globIndex], "/")
	if lastSlashBeforeGlob < 0 {
		return "", false, fmt.Errorf("unsafe_path_rewrite: pattern %q matches %q but cannot be safely rewritten to %q", normalizedPattern, normalizedFrom, normalizedTo)
	}
	suffix := normalizedPattern[lastSlashBeforeGlob+1:]
	if strings.Contains(suffix, "/") {
		return "", false, fmt.Errorf("unsafe_path_rewrite: pattern %q matches %q but cannot be safely rewritten to %q", normalizedPattern, normalizedFrom, normalizedTo)
	}
	return path.Join(path.Dir(normalizedTo), suffix), true, nil
}

func layoutPathsMatchPath(patterns []string, pathValue string) bool {
	normalizedPath, err := ids.NormalizeRepoRelativePath(pathValue)
	if err != nil {
		return false
	}
	for _, pattern := range patterns {
		normalizedPattern, err := ids.NormalizeRepoRelativePath(pattern)
		if err != nil {
			continue
		}
		matched, err := doublestar.Match(normalizedPattern, normalizedPath)
		if err == nil && matched {
			return true
		}
	}

	return false
}

func layoutAnyMemberPathMatches(members []orbitpkg.OrbitMember, pathValue string) bool {
	for _, member := range members {
		if layoutPathsMatchPath(member.Paths.Include, pathValue) {
			return true
		}
	}

	return false
}

func layoutMemberFromMoveDestination(toPath string) orbitpkg.OrbitMember {
	base := path.Base(strings.TrimSuffix(toPath, "/**"))
	name := strings.TrimSuffix(base, path.Ext(base))
	if err := ids.ValidateOrbitID(name); err != nil {
		name = "moved-member"
	}

	return orbitpkg.OrbitMember{
		Name: name,
		Role: orbitpkg.OrbitMemberRule,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{toPath},
		},
	}
}

func layoutUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
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

func layoutAppendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}

	return append(values, value)
}

func layoutRuntimeOrbitIDs(manifest harnesspkg.ManifestFile, explicitOrbitID string) []string {
	seen := map[string]struct{}{}
	orbitIDs := []string{}
	for _, member := range manifest.Members {
		orbitID := strings.TrimSpace(member.OrbitID)
		if orbitID == "" {
			continue
		}
		if explicitOrbitID != "" && orbitID != explicitOrbitID {
			continue
		}
		if _, ok := seen[orbitID]; ok {
			continue
		}
		seen[orbitID] = struct{}{}
		orbitIDs = append(orbitIDs, orbitID)
	}
	sort.Strings(orbitIDs)

	return orbitIDs
}

func layoutCodexPromptCommandPaths(trackedFiles []string) []string {
	paths := []string{}
	for _, trackedFile := range trackedFiles {
		if !strings.HasPrefix(trackedFile, ".codex/prompts/") || !strings.HasSuffix(trackedFile, ".md") {
			continue
		}
		paths = append(paths, trackedFile)
	}
	sort.Strings(paths)

	return paths
}

func layoutRecommendedCommandPath(orbitID string, currentPath string) string {
	if strings.HasPrefix(currentPath, ".codex/prompts/") {
		return path.Join("commands", orbitID, strings.TrimPrefix(currentPath, ".codex/prompts/"))
	}

	return path.Join("commands", orbitID, path.Base(currentPath))
}

func layoutRecommendedGuidancePath(orbitID string, currentPath string) string {
	trimmed := strings.TrimSuffix(currentPath, "/**")

	return path.Join("guidance", orbitID, path.Base(trimmed))
}

func layoutRecommendedHookHandlerPath(orbitID string, currentPath string) string {
	return path.Join("hooks", orbitID, path.Base(path.Dir(currentPath)), path.Base(currentPath))
}

func layoutOrbitTruthPath(orbitID string) string {
	truthPath, err := orbitpkg.HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return path.Join(".harness", "orbits", orbitID+".yaml")
	}

	return truthPath
}

func layoutCandidateEvidenceDetail(candidate adoptCheckCandidate, evidenceKind string) string {
	for _, evidence := range candidate.Evidence {
		if evidence.Kind == evidenceKind {
			return strings.TrimSpace(evidence.Detail)
		}
	}

	return ""
}

func layoutProblemFromAdoptDiagnostic(diagnostic adoptCheckDiagnostic) layoutOptimizeProblem {
	return layoutOptimizeProblem{
		Code:    diagnostic.Code,
		Path:    layoutProblemPathFromAdoptEvidence(diagnostic.Evidence),
		Message: diagnostic.Message,
	}
}

func layoutProblemPathFromAdoptEvidence(evidence []adoptCheckEvidence) string {
	for _, item := range evidence {
		switch item.Kind {
		case "markdown_link", "path_mention":
			if strings.TrimSpace(item.Detail) != "" {
				return strings.TrimSpace(item.Detail)
			}
		}
	}
	for _, item := range evidence {
		if strings.TrimSpace(item.Path) != "" {
			return strings.TrimSpace(item.Path)
		}
	}

	return ""
}

func printLayoutOptimizeText(cmd *cobra.Command, output layoutOptimizeOutput) error {
	mode := output.Mode
	if mode == "check" {
		mode = "preview"
	}
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"layout optimize %s %s: %d moves, %d conflicts, %d warnings\n",
		output.RepositoryMode,
		mode,
		len(output.MovePlan.Moves),
		len(output.MovePlan.Conflicts),
		len(output.MovePlan.Warnings),
	); err != nil {
		return fmt.Errorf("write layout optimize summary: %w", err)
	}

	return nil
}
