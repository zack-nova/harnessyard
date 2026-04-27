package orbit

import (
	"fmt"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

func validateCommandCapabilityExportSurface(
	commands *OrbitCommandCapabilityPaths,
	trackedFiles []string,
	exportPaths []string,
) error {
	if commands == nil {
		return nil
	}

	trackedSet, err := normalizedPathSet(trackedFiles)
	if err != nil {
		return fmt.Errorf("normalize tracked files: %w", err)
	}
	exportSet, err := normalizedPathSet(exportPaths)
	if err != nil {
		return fmt.Errorf("normalize export paths: %w", err)
	}

	matches, err := matchCapabilityFiles(commands.Paths, trackedFiles)
	if err != nil {
		return fmt.Errorf("resolve command capability paths: %w", err)
	}
	for _, matchedPath := range matches {
		if _, ok := trackedSet[matchedPath]; !ok {
			return fmt.Errorf("command path %q must reference one tracked file inside the export surface", matchedPath)
		}
		if _, ok := exportSet[matchedPath]; !ok {
			return fmt.Errorf("command path %q must resolve inside the export surface", matchedPath)
		}
	}

	return nil
}

func validateLocalSkillCapabilityExportSurface(
	localSkills *OrbitLocalSkillCapabilityPaths,
	trackedFiles []string,
	exportPaths []string,
) error {
	if localSkills == nil {
		return nil
	}

	trackedSet, err := normalizedPathSet(trackedFiles)
	if err != nil {
		return fmt.Errorf("normalize tracked files: %w", err)
	}
	exportSet, err := normalizedPathSet(exportPaths)
	if err != nil {
		return fmt.Errorf("normalize export paths: %w", err)
	}

	roots, err := matchCapabilityDirectories(localSkills.Paths, trackedFiles)
	if err != nil {
		return fmt.Errorf("resolve local skill capability paths: %w", err)
	}
	for _, rootPath := range roots {
		skillMDPath := rootPath + "/SKILL.md"
		if _, ok := trackedSet[skillMDPath]; !ok {
			return fmt.Errorf(`local skill root %q: SKILL.md must exist and be tracked`, rootPath)
		}
		if err := ensureCapabilityRootInsideExport(rootPath, trackedSet, exportSet); err != nil {
			return fmt.Errorf("local skill root %q: %w", rootPath, err)
		}
	}

	return nil
}

func normalizedPathSet(paths []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(paths))
	for _, rawPath := range paths {
		normalizedPath, err := ids.NormalizeRepoRelativePath(rawPath)
		if err != nil {
			return nil, fmt.Errorf("normalize path %q: %w", rawPath, err)
		}
		set[normalizedPath] = struct{}{}
	}

	return set, nil
}
