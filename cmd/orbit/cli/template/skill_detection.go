package orbittemplate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type TemplateLocalSkillDetectionInput struct {
	RepoRoot                string
	OrbitID                 string
	AggregateDetectedSkills bool
	AllowOutOfRangeSkills   bool
	ConfirmPrompter         ConfirmPrompter
}

type TemplateLocalSkillDetectionResult struct {
	Detected   []orbitpkg.ResolvedLocalSkillCapability
	Warnings   []string
	Aggregated bool
}

func RunTemplateLocalSkillDetection(
	ctx context.Context,
	input TemplateLocalSkillDetectionInput,
) (TemplateLocalSkillDetectionResult, error) {
	if input.AggregateDetectedSkills && input.AllowOutOfRangeSkills {
		return TemplateLocalSkillDetectionResult{}, fmt.Errorf(
			"--aggregate-detected-skills cannot be combined with --allow-out-of-range-skills",
		)
	}

	repoConfig, err := loadTemplateSaveRepositoryConfig(ctx, input.RepoRoot)
	if err != nil {
		return TemplateLocalSkillDetectionResult{}, fmt.Errorf("load repository config: %w", err)
	}
	if len(repoConfig.Orbits) != 1 {
		return TemplateLocalSkillDetectionResult{}, nil
	}

	definition, found := repoConfig.OrbitByID(input.OrbitID)
	if !found {
		return TemplateLocalSkillDetectionResult{}, nil
	}

	trackedFiles, err := gitpkg.TrackedFiles(ctx, input.RepoRoot)
	if err != nil {
		return TemplateLocalSkillDetectionResult{}, fmt.Errorf("load tracked files: %w", err)
	}
	spec, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(ctx, input.RepoRoot, repoConfig, definition.ID, trackedFiles)
	if err != nil {
		return TemplateLocalSkillDetectionResult{}, fmt.Errorf("load orbit export plan: %w", err)
	}

	declared, ok := resolveDeclaredLocalSkillsForTemplateDetection(input.RepoRoot, spec, trackedFiles, plan.ExportPaths)
	if !ok {
		// Let the main save/publish preflight surface the authoritative capability error.
		return TemplateLocalSkillDetectionResult{}, nil
	}
	candidates, err := orbitpkg.DetectValidLocalSkillCapabilities(input.RepoRoot, trackedFiles, plan.ExportPaths)
	if err != nil {
		return TemplateLocalSkillDetectionResult{}, fmt.Errorf("detect valid local skills: %w", err)
	}

	declaredRoots := make(map[string]struct{}, len(declared))
	for _, skill := range declared {
		declaredRoots[skill.RootPath] = struct{}{}
	}

	detected := make([]orbitpkg.ResolvedLocalSkillCapability, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := declaredRoots[candidate.RootPath]; ok {
			continue
		}
		detected = append(detected, candidate)
	}
	if len(detected) == 0 {
		return TemplateLocalSkillDetectionResult{}, nil
	}

	result := TemplateLocalSkillDetectionResult{
		Detected: append([]orbitpkg.ResolvedLocalSkillCapability(nil), detected...),
	}

	switch {
	case input.AggregateDetectedSkills:
		if err := aggregateDetectedLocalSkills(ctx, input.RepoRoot, spec, detected); err != nil {
			return TemplateLocalSkillDetectionResult{}, err
		}
		result.Aggregated = true
		return result, nil
	case input.AllowOutOfRangeSkills:
		result.Warnings = []string{formatOutOfRangeLocalSkillWarning(input.OrbitID, detected)}
		return result, nil
	case input.ConfirmPrompter != nil:
		confirmed, err := input.ConfirmPrompter.Confirm(ctx, formatDetectedLocalSkillPrompt(input.OrbitID, detected))
		if err != nil {
			return TemplateLocalSkillDetectionResult{}, fmt.Errorf("confirm detected local skill aggregation: %w", err)
		}
		if confirmed {
			if err := aggregateDetectedLocalSkills(ctx, input.RepoRoot, spec, detected); err != nil {
				return TemplateLocalSkillDetectionResult{}, err
			}
			result.Aggregated = true
			return result, nil
		}
		result.Warnings = []string{formatOutOfRangeLocalSkillWarning(input.OrbitID, detected)}
		return result, nil
	default:
		return TemplateLocalSkillDetectionResult{}, fmt.Errorf(
			"%s; rerun with --aggregate-detected-skills to move them under %s, or --allow-out-of-range-skills to continue with a warning",
			formatDetectedLocalSkillSummary(detected),
			defaultLocalSkillInclude(input.OrbitID),
		)
	}
}

func aggregateDetectedLocalSkills(
	ctx context.Context,
	repoRoot string,
	spec orbitpkg.OrbitSpec,
	detected []orbitpkg.ResolvedLocalSkillCapability,
) error {
	destinations := make(map[string]string, len(detected))
	for _, skill := range detected {
		destinationRoot := path.Join("skills", spec.ID, skill.Name)
		if existingRoot, ok := destinations[destinationRoot]; ok {
			return fmt.Errorf(
				"aggregate detected local skills: multiple skill roots would collide at %q: %q and %q",
				destinationRoot,
				existingRoot,
				skill.RootPath,
			)
		}
		destinations[destinationRoot] = skill.RootPath
	}
	updatedPaths := updatedLocalSkillCapabilityPathsForAggregation(spec, spec.ID)
	for destinationRoot := range destinations {
		matches, err := orbitpkg.MemberMatchesPath(orbitpkg.OrbitMember{
			Paths: updatedPaths,
		}, destinationRoot)
		if err != nil {
			return fmt.Errorf("aggregate detected local skills: validate destination %q against capabilities.skills.local.paths: %w", destinationRoot, err)
		}
		if !matches {
			return fmt.Errorf(
				"aggregate detected local skills: destination %q would remain outside capabilities.skills.local.paths after applying the existing include/exclude rules",
				destinationRoot,
			)
		}
	}

	roots := make([]string, 0, len(detected))
	for _, skill := range detected {
		roots = append(roots, skill.RootPath)
	}
	sort.Strings(roots)

	for _, sourceRoot := range roots {
		destinationRoot := path.Join("skills", spec.ID, path.Base(sourceRoot))
		if sourceRoot == destinationRoot {
			continue
		}
		destinationPath := filepath.Join(repoRoot, filepath.FromSlash(destinationRoot))
		if _, err := os.Stat(destinationPath); err == nil {
			return fmt.Errorf("aggregate detected local skills: destination %q already exists", destinationRoot)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("aggregate detected local skills: stat destination %q: %w", destinationRoot, err)
		}
		if err := gitMovePath(ctx, repoRoot, sourceRoot, destinationRoot); err != nil {
			return err
		}
	}

	spec, err := orbitpkg.EnsureHostedMemberSchema(spec)
	if err != nil {
		return fmt.Errorf("prepare hosted orbit spec for detected skill aggregation: %w", err)
	}
	if spec.Capabilities == nil {
		spec.Capabilities = &orbitpkg.OrbitCapabilities{}
	}
	if spec.Capabilities.Skills == nil {
		spec.Capabilities.Skills = &orbitpkg.OrbitSkillCapabilities{}
	}
	if spec.Capabilities.Skills.Local == nil {
		spec.Capabilities.Skills.Local = &orbitpkg.OrbitLocalSkillCapabilityPaths{}
	}

	spec.Capabilities.Skills.Local.Paths = updatedPaths

	if _, err := orbitpkg.WriteHostedOrbitSpec(repoRoot, spec); err != nil {
		return fmt.Errorf("write hosted orbit spec after detected skill aggregation: %w", err)
	}

	return nil
}

func gitMovePath(ctx context.Context, repoRoot string, sourcePath string, destinationPath string) error {
	absoluteDestination := filepath.Join(repoRoot, filepath.FromSlash(destinationPath))
	if err := os.MkdirAll(filepath.Dir(absoluteDestination), 0o750); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", destinationPath, err)
	}

	//nolint:gosec // Git is invoked with explicit argument lists over validated repo-relative paths.
	command := exec.CommandContext(
		ctx,
		"git",
		"-C",
		repoRoot,
		"mv",
		"--",
		sourcePath,
		destinationPath,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"move detected local skill %q -> %q: %w%s",
			sourcePath,
			destinationPath,
			err,
			formatGitCommandOutput(output),
		)
	}

	return nil
}

func formatGitCommandOutput(output []byte) string {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return ""
	}

	return ": " + trimmed
}

func formatDetectedLocalSkillPrompt(orbitID string, detected []orbitpkg.ResolvedLocalSkillCapability) string {
	lines := []string{
		"detected valid local skills outside capabilities.skills.local.paths:",
	}
	for _, skill := range detected {
		lines = append(lines, "  - "+skill.RootPath+" -> "+path.Join("skills", orbitID, skill.Name))
	}
	lines = append(lines, "aggregate them to the default skill location before continuing? [y/N] ")
	return strings.Join(lines, "\n")
}

func formatDetectedLocalSkillSummary(detected []orbitpkg.ResolvedLocalSkillCapability) string {
	paths := make([]string, 0, len(detected))
	for _, skill := range detected {
		paths = append(paths, skill.RootPath)
	}
	sort.Strings(paths)
	return fmt.Sprintf(
		"detected valid local skills outside capabilities.skills.local.paths: %s",
		strings.Join(paths, ", "),
	)
}

func formatOutOfRangeLocalSkillWarning(orbitID string, detected []orbitpkg.ResolvedLocalSkillCapability) string {
	return fmt.Sprintf(
		"%s; these skills will not take effect unless you expand capabilities.skills.local.paths or move them under %s",
		formatDetectedLocalSkillSummary(detected),
		defaultLocalSkillInclude(orbitID),
	)
}

func defaultLocalSkillInclude(orbitID string) string {
	return path.Join("skills", orbitID, "*")
}

func skillPathsDefaultEquivalent(paths orbitpkg.OrbitMemberPaths, orbitID string) bool {
	if len(paths.Exclude) > 0 {
		return false
	}
	if len(paths.Include) == 0 {
		return true
	}
	return len(paths.Include) == 1 && paths.Include[0] == defaultLocalSkillInclude(orbitID)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}

	return false
}

func resolveDeclaredLocalSkillsForTemplateDetection(
	repoRoot string,
	spec orbitpkg.OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
) ([]orbitpkg.ResolvedLocalSkillCapability, bool) {
	declared, err := orbitpkg.ResolveLocalSkillCapabilities(repoRoot, spec, trackedFiles, exportPaths)
	if err != nil {
		return nil, false
	}

	return declared, true
}

func updatedLocalSkillCapabilityPathsForAggregation(spec orbitpkg.OrbitSpec, orbitID string) orbitpkg.OrbitMemberPaths {
	current := orbitpkg.OrbitMemberPaths{}
	if spec.Capabilities != nil && spec.Capabilities.Skills != nil && spec.Capabilities.Skills.Local != nil {
		current = spec.Capabilities.Skills.Local.Paths
	}

	defaultInclude := defaultLocalSkillInclude(orbitID)
	if skillPathsDefaultEquivalent(current, orbitID) {
		return orbitpkg.OrbitMemberPaths{
			Include: []string{defaultInclude},
		}
	}

	updated := orbitpkg.OrbitMemberPaths{
		Include: append([]string(nil), current.Include...),
		Exclude: append([]string(nil), current.Exclude...),
	}
	if !containsString(updated.Include, defaultInclude) {
		updated.Include = append(updated.Include, defaultInclude)
		sort.Strings(updated.Include)
	}

	return updated
}
