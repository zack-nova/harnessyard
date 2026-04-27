package orbit

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// ResolveCommandCapabilities derives command capabilities from the authored path scope.
func ResolveCommandCapabilities(
	spec OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
) ([]ResolvedCommandCapability, error) {
	if spec.Capabilities == nil || spec.Capabilities.Commands == nil {
		return nil, nil
	}

	trackedSet, err := normalizedPathSet(trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("normalize tracked files: %w", err)
	}
	exportSet, err := normalizedPathSet(exportPaths)
	if err != nil {
		return nil, fmt.Errorf("normalize export paths: %w", err)
	}

	matches, err := matchCapabilityFiles(spec.Capabilities.Commands.Paths, trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("resolve command paths: %w", err)
	}

	resolved := make([]ResolvedCommandCapability, 0, len(matches))
	seenNames := make(map[string]string, len(matches))
	for _, matchedPath := range matches {
		if _, ok := trackedSet[matchedPath]; !ok {
			return nil, fmt.Errorf("command path %q must reference one tracked file", matchedPath)
		}
		if _, ok := exportSet[matchedPath]; !ok {
			return nil, fmt.Errorf("command path %q must resolve inside the export surface", matchedPath)
		}
		if !strings.HasSuffix(matchedPath, ".md") {
			return nil, fmt.Errorf(`command path %q must end with ".md"`, matchedPath)
		}

		name := strings.TrimSuffix(path.Base(matchedPath), ".md")
		if err := ids.ValidateOrbitID(name); err != nil {
			return nil, fmt.Errorf("command path %q: invalid command basename: %w", matchedPath, err)
		}
		if existingPath, ok := seenNames[name]; ok {
			return nil, fmt.Errorf(`resolved command name %q is declared by multiple files: %q and %q`, name, existingPath, matchedPath)
		}
		seenNames[name] = matchedPath
		resolved = append(resolved, ResolvedCommandCapability{
			Name: name,
			Path: matchedPath,
		})
	}

	sort.Slice(resolved, func(left, right int) bool {
		return resolved[left].Path < resolved[right].Path
	})

	return resolved, nil
}

// ResolveLocalSkillCapabilities derives local skill capabilities from the authored path scope.
func ResolveLocalSkillCapabilities(
	repoRoot string,
	spec OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
) ([]ResolvedLocalSkillCapability, error) {
	return resolveLocalSkillCapabilitiesWithLoader(spec, trackedFiles, exportPaths, func(skillMDPath string) (skillFrontmatter, error) {
		return loadSkillFrontmatter(repoRoot, skillMDPath)
	})
}

// DetectValidLocalSkillCapabilities scans the export surface for valid local skill roots
// whether or not they are currently declared inside capabilities.skills.local.paths.
func DetectValidLocalSkillCapabilities(
	repoRoot string,
	trackedFiles []string,
	exportPaths []string,
) ([]ResolvedLocalSkillCapability, error) {
	return detectValidLocalSkillCapabilitiesWithLoader(trackedFiles, exportPaths, func(skillMDPath string) (skillFrontmatter, error) {
		return loadSkillFrontmatter(repoRoot, skillMDPath)
	})
}

// ResolveLocalSkillCapabilitiesFromFiles derives local skill capabilities from the authored path scope
// using repo-relative file contents supplied by the caller.
func ResolveLocalSkillCapabilitiesFromFiles(
	spec OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
	files map[string][]byte,
) ([]ResolvedLocalSkillCapability, error) {
	return resolveLocalSkillCapabilitiesWithLoader(spec, trackedFiles, exportPaths, func(skillMDPath string) (skillFrontmatter, error) {
		data, ok := files[skillMDPath]
		if !ok {
			return skillFrontmatter{}, fmt.Errorf("read SKILL.md: repo-relative path %q not found", skillMDPath)
		}
		return parseSkillFrontmatter(data)
	})
}

// DetectValidLocalSkillCapabilitiesFromFiles scans one exported repo-relative file payload
// for valid local skill roots.
func DetectValidLocalSkillCapabilitiesFromFiles(
	trackedFiles []string,
	exportPaths []string,
	files map[string][]byte,
) ([]ResolvedLocalSkillCapability, error) {
	return detectValidLocalSkillCapabilitiesWithLoader(trackedFiles, exportPaths, func(skillMDPath string) (skillFrontmatter, error) {
		data, ok := files[skillMDPath]
		if !ok {
			return skillFrontmatter{}, fmt.Errorf("read SKILL.md: repo-relative path %q not found", skillMDPath)
		}
		return parseSkillFrontmatter(data)
	})
}

func resolveLocalSkillCapabilitiesWithLoader(
	spec OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
	loadFrontmatter func(skillMDPath string) (skillFrontmatter, error),
) ([]ResolvedLocalSkillCapability, error) {
	if spec.Capabilities == nil || spec.Capabilities.Skills == nil || spec.Capabilities.Skills.Local == nil {
		return nil, nil
	}

	trackedSet, err := normalizedPathSet(trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("normalize tracked files: %w", err)
	}
	exportSet, err := normalizedPathSet(exportPaths)
	if err != nil {
		return nil, fmt.Errorf("normalize export paths: %w", err)
	}

	roots, err := matchCapabilityDirectories(spec.Capabilities.Skills.Local.Paths, trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("resolve local skill roots: %w", err)
	}

	resolved := make([]ResolvedLocalSkillCapability, 0, len(roots))
	seenNames := make(map[string]string, len(roots))
	for _, rootPath := range roots {
		skillMDPath := rootPath + "/SKILL.md"
		if _, ok := trackedSet[skillMDPath]; !ok {
			return nil, fmt.Errorf(`local skill root %q: SKILL.md must exist and be tracked`, rootPath)
		}
		if err := ensureCapabilityRootInsideExport(rootPath, trackedSet, exportSet); err != nil {
			return nil, fmt.Errorf("local skill root %q: %w", rootPath, err)
		}

		name := path.Base(rootPath)
		if err := ids.ValidateOrbitID(name); err != nil {
			return nil, fmt.Errorf("local skill root %q: invalid skill basename: %w", rootPath, err)
		}

		frontmatter, err := loadFrontmatter(skillMDPath)
		if err != nil {
			return nil, fmt.Errorf("local skill root %q: %w", rootPath, err)
		}
		if frontmatter.Name != name {
			return nil, fmt.Errorf(`local skill root %q: SKILL.md frontmatter name %q must match directory basename %q`, rootPath, frontmatter.Name, name)
		}
		if existingPath, ok := seenNames[name]; ok {
			return nil, fmt.Errorf(`resolved local skill name %q is declared by multiple roots: %q and %q`, name, existingPath, rootPath)
		}
		seenNames[name] = rootPath
		resolved = append(resolved, ResolvedLocalSkillCapability{
			Name:        name,
			RootPath:    rootPath,
			SkillMDPath: skillMDPath,
		})
	}

	sort.Slice(resolved, func(left, right int) bool {
		return resolved[left].RootPath < resolved[right].RootPath
	})

	return resolved, nil
}

func detectValidLocalSkillCapabilitiesWithLoader(
	trackedFiles []string,
	exportPaths []string,
	loadFrontmatter func(skillMDPath string) (skillFrontmatter, error),
) ([]ResolvedLocalSkillCapability, error) {
	trackedSet, err := normalizedPathSet(trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("normalize tracked files: %w", err)
	}
	exportSet, err := normalizedPathSet(exportPaths)
	if err != nil {
		return nil, fmt.Errorf("normalize export paths: %w", err)
	}

	rootSet := make(map[string]struct{})
	for exportPath := range exportSet {
		if !strings.HasSuffix(exportPath, "/SKILL.md") {
			continue
		}
		rootSet[path.Dir(exportPath)] = struct{}{}
	}

	roots := make([]string, 0, len(rootSet))
	for rootPath := range rootSet {
		roots = append(roots, rootPath)
	}
	sort.Strings(roots)

	resolved := make([]ResolvedLocalSkillCapability, 0, len(roots))
	for _, rootPath := range roots {
		skillMDPath := rootPath + "/SKILL.md"
		if _, ok := trackedSet[skillMDPath]; !ok {
			continue
		}
		if err := ensureCapabilityRootInsideExport(rootPath, trackedSet, exportSet); err != nil {
			continue
		}

		name := path.Base(rootPath)
		if err := ids.ValidateOrbitID(name); err != nil {
			continue
		}

		frontmatter, err := loadFrontmatter(skillMDPath)
		if err != nil {
			continue
		}
		if frontmatter.Name != name {
			continue
		}

		resolved = append(resolved, ResolvedLocalSkillCapability{
			Name:        name,
			RootPath:    rootPath,
			SkillMDPath: skillMDPath,
		})
	}

	sort.Slice(resolved, func(left, right int) bool {
		return resolved[left].RootPath < resolved[right].RootPath
	})

	return resolved, nil
}

// PreflightResolvedCapabilities validates that authored capability truth can resolve
// against the current tracked/export surface on disk.
func PreflightResolvedCapabilities(repoRoot string, spec OrbitSpec, trackedFiles []string, exportPaths []string) error {
	if _, err := ResolveCommandCapabilities(spec, trackedFiles, exportPaths); err != nil {
		return fmt.Errorf("resolve command capabilities: %w", err)
	}
	if _, err := ResolveLocalSkillCapabilities(repoRoot, spec, trackedFiles, exportPaths); err != nil {
		return fmt.Errorf("resolve local skill capabilities: %w", err)
	}
	if _, err := ResolveRemoteSkillCapabilities(spec); err != nil {
		return fmt.Errorf("resolve remote skill capabilities: %w", err)
	}
	if _, err := ResolveAgentAddonHooks(spec, trackedFiles, exportPaths); err != nil {
		return fmt.Errorf("resolve agent add-on hooks: %w", err)
	}

	return nil
}

// PreflightResolvedCapabilitiesFromFiles validates authored capability truth against
// a repo-relative file payload supplied by the caller.
func PreflightResolvedCapabilitiesFromFiles(
	spec OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
	files map[string][]byte,
) error {
	if _, err := ResolveCommandCapabilities(spec, trackedFiles, exportPaths); err != nil {
		return fmt.Errorf("resolve command capabilities: %w", err)
	}
	if _, err := ResolveLocalSkillCapabilitiesFromFiles(spec, trackedFiles, exportPaths, files); err != nil {
		return fmt.Errorf("resolve local skill capabilities: %w", err)
	}
	if _, err := ResolveRemoteSkillCapabilities(spec); err != nil {
		return fmt.Errorf("resolve remote skill capabilities: %w", err)
	}
	if _, err := ResolveAgentAddonHooks(spec, trackedFiles, exportPaths); err != nil {
		return fmt.Errorf("resolve agent add-on hooks: %w", err)
	}

	return nil
}

func resolveCapabilityOverlayPaths(spec OrbitSpec, trackedFiles []string) ([]string, error) {
	if spec.Capabilities == nil {
		return nil, nil
	}

	commandPaths, err := resolveCommandCapabilityOverlayPaths(spec.Capabilities.Commands, trackedFiles)
	if err != nil {
		return nil, err
	}

	localSkillPaths := []string{}
	if spec.Capabilities.Skills != nil {
		localSkillPaths, err = resolveLocalSkillCapabilityOverlayPaths(spec.Capabilities.Skills.Local, trackedFiles)
		if err != nil {
			return nil, err
		}
	}

	return mergeSortedUniquePaths(commandPaths, localSkillPaths), nil
}

func resolveCommandCapabilityOverlayPaths(commands *OrbitCommandCapabilityPaths, trackedFiles []string) ([]string, error) {
	if commands == nil {
		return nil, nil
	}

	matches, err := matchCapabilityFiles(commands.Paths, trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("resolve command paths: %w", err)
	}

	return matches, nil
}

func resolveLocalSkillCapabilityOverlayPaths(localSkills *OrbitLocalSkillCapabilityPaths, trackedFiles []string) ([]string, error) {
	if localSkills == nil {
		return nil, nil
	}

	roots, err := matchCapabilityDirectories(localSkills.Paths, trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("resolve local skill roots: %w", err)
	}

	overlaySet := make(map[string]struct{})
	for _, trackedFile := range trackedFiles {
		normalizedPath, err := ids.NormalizeRepoRelativePath(trackedFile)
		if err != nil {
			return nil, fmt.Errorf("normalize tracked path %q: %w", trackedFile, err)
		}
		for _, rootPath := range roots {
			if !strings.HasPrefix(normalizedPath, rootPath+"/") {
				continue
			}
			overlaySet[normalizedPath] = struct{}{}
		}
	}

	overlay := make([]string, 0, len(overlaySet))
	for path := range overlaySet {
		overlay = append(overlay, path)
	}
	sort.Strings(overlay)

	return overlay, nil
}

// ResolveRemoteSkillCapabilities derives remote skill capabilities from authored URI/dependency truth.
func ResolveRemoteSkillCapabilities(spec OrbitSpec) ([]ResolvedRemoteSkillCapability, error) {
	if spec.Capabilities == nil || spec.Capabilities.Skills == nil || spec.Capabilities.Skills.Remote == nil {
		return nil, nil
	}

	remote := spec.Capabilities.Skills.Remote
	if len(remote.URIs) > 0 && len(remote.Dependencies) > 0 {
		return nil, fmt.Errorf("capabilities.skills.remote must not define both uris and dependencies")
	}

	resolved := make([]ResolvedRemoteSkillCapability, 0, len(remote.URIs)+len(remote.Dependencies))
	seen := map[string]struct{}{}
	for index, rawURI := range remote.URIs {
		normalizedURI, err := normalizeRemoteSkillURI(rawURI)
		if err != nil {
			return nil, fmt.Errorf("capabilities.skills.remote.uris[%d]: %w", index, err)
		}
		if _, ok := seen[normalizedURI]; ok {
			return nil, fmt.Errorf("capabilities.skills.remote.uris[%d]: duplicate remote skill URI %q", index, normalizedURI)
		}
		seen[normalizedURI] = struct{}{}
		resolved = append(resolved, ResolvedRemoteSkillCapability{URI: normalizedURI})
	}
	for index, dependency := range remote.Dependencies {
		normalizedURI, err := normalizeRemoteSkillURI(dependency.URI)
		if err != nil {
			return nil, fmt.Errorf("capabilities.skills.remote.dependencies[%d].uri: %w", index, err)
		}
		if _, ok := seen[normalizedURI]; ok {
			return nil, fmt.Errorf("capabilities.skills.remote.dependencies[%d].uri: duplicate remote skill URI %q", index, normalizedURI)
		}
		seen[normalizedURI] = struct{}{}
		resolved = append(resolved, ResolvedRemoteSkillCapability{URI: normalizedURI, Required: dependency.Required})
	}

	return resolved, nil
}

func matchCapabilityFiles(paths OrbitMemberPaths, trackedFiles []string) ([]string, error) {
	return matchCapabilityCandidates(paths, trackedFiles)
}

func matchCapabilityDirectories(paths OrbitMemberPaths, trackedFiles []string) ([]string, error) {
	directories, err := trackedDirectories(trackedFiles)
	if err != nil {
		return nil, err
	}

	return matchCapabilityCandidates(paths, directories)
}

func matchCapabilityCandidates(paths OrbitMemberPaths, candidates []string) ([]string, error) {
	if len(paths.Include) == 0 {
		return nil, nil
	}

	includePatterns, err := normalizeMemberPatterns(paths.Include)
	if err != nil {
		return nil, fmt.Errorf("normalize include patterns: %w", err)
	}
	excludePatterns, err := normalizeMemberPatterns(paths.Exclude)
	if err != nil {
		return nil, fmt.Errorf("normalize exclude patterns: %w", err)
	}

	matchedSet := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		normalizedCandidate, err := ids.NormalizeRepoRelativePath(candidate)
		if err != nil {
			return nil, fmt.Errorf("normalize candidate path %q: %w", candidate, err)
		}

		included, err := matchMemberPatterns(includePatterns, normalizedCandidate)
		if err != nil {
			return nil, fmt.Errorf("match include patterns for %q: %w", normalizedCandidate, err)
		}
		if !included {
			continue
		}

		excluded, err := matchMemberPatterns(excludePatterns, normalizedCandidate)
		if err != nil {
			return nil, fmt.Errorf("match exclude patterns for %q: %w", normalizedCandidate, err)
		}
		if excluded {
			continue
		}

		matchedSet[normalizedCandidate] = struct{}{}
	}

	matches := make([]string, 0, len(matchedSet))
	for matchedPath := range matchedSet {
		matches = append(matches, matchedPath)
	}
	sort.Strings(matches)

	return matches, nil
}

func trackedDirectories(trackedFiles []string) ([]string, error) {
	directories := make(map[string]struct{}, len(trackedFiles))
	for _, trackedFile := range trackedFiles {
		normalizedPath, err := ids.NormalizeRepoRelativePath(trackedFile)
		if err != nil {
			return nil, fmt.Errorf("normalize tracked path %q: %w", trackedFile, err)
		}

		dir := path.Dir(normalizedPath)
		for dir != "." && dir != "/" {
			directories[dir] = struct{}{}
			dir = path.Dir(dir)
		}
	}

	values := make([]string, 0, len(directories))
	for directory := range directories {
		values = append(values, directory)
	}
	sort.Strings(values)

	return values, nil
}

func ensureCapabilityRootInsideExport(rootPath string, trackedSet map[string]struct{}, exportSet map[string]struct{}) error {
	skillMDPath := rootPath + "/SKILL.md"
	if _, ok := exportSet[skillMDPath]; !ok {
		return fmt.Errorf("SKILL.md must resolve inside the export surface")
	}

	prefix := rootPath + "/"
	for trackedPath := range trackedSet {
		if !strings.HasPrefix(trackedPath, prefix) {
			continue
		}
		if _, ok := exportSet[trackedPath]; !ok {
			return fmt.Errorf("entire skill root must resolve inside the export surface")
		}
	}

	return nil
}
