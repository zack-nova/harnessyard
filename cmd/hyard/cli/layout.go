package cli

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

const layoutOptimizeSchemaVersion = "1.0"

type layoutOptimizeOutput struct {
	SchemaVersion  string             `json:"schema_version"`
	RepoRoot       string             `json:"repo_root"`
	Mode           string             `json:"mode"`
	RepositoryMode string             `json:"repository_mode"`
	MovePlan       layoutOptimizePlan `json:"move_plan"`
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

	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Preview Harness Yard-friendly file placement",
		Long: "Preview Harness Yard-friendly file placement across adopted member candidates,\n" +
			"agent assets, and existing Harness Runtime truth.",
		Example: "" +
			"  hyard layout optimize --check --json\n" +
			"  hyard layout optimize --check --json --orbit docs\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !check {
				return fmt.Errorf("layout optimize currently supports preview mode only; pass --check")
			}

			output, err := buildLayoutOptimizeCheckOutput(cmd, orbitID)
			if err != nil {
				return err
			}

			jsonOutput, err := wantHyardJSON(cmd)
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
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Orbit id to use for Ordinary Repository preview or to filter a Harness Runtime")
	addHyardJSONFlag(cmd)

	return cmd
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

	trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), adoptionPreview.RepoRoot)
	if err != nil {
		return layoutOptimizePlan{}, fmt.Errorf("load tracked files for prompt command discovery: %w", err)
	}
	for _, promptPath := range layoutCodexPromptCommandPaths(trackedFiles) {
		plan.Moves = append(plan.Moves, layoutMoveIfDifferent(
			promptPath,
			layoutRecommendedCommandPath(adoptionPreview.AdoptedOrbit.ID, promptPath),
			"prompt_command_recommended_position",
			layoutOrbitTruthPath(adoptionPreview.AdoptedOrbit.ID),
			"capabilities.commands.paths.include",
		)...)
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

		for _, member := range spec.Members {
			if len(member.Paths.Include) != 1 || len(member.Paths.Exclude) > 0 {
				continue
			}
			fromPath := strings.TrimSuffix(member.Paths.Include[0], "/**")
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
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"layout optimize %s preview: %d moves, %d conflicts, %d warnings\n",
		output.RepositoryMode,
		len(output.MovePlan.Moves),
		len(output.MovePlan.Conflicts),
		len(output.MovePlan.Warnings),
	); err != nil {
		return fmt.Errorf("write layout optimize summary: %w", err)
	}

	return nil
}
