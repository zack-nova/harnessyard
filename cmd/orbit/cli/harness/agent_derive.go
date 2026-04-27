package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// AgentDeriveResult reports one completed runtime agent truth derive.
type AgentDeriveResult struct {
	RecommendedFramework string   `json:"recommended_framework,omitempty"`
	WrittenPaths         []string `json:"written_paths,omitempty"`
	PackageCount         int      `json:"package_count"`
	Warnings             []string `json:"warnings,omitempty"`
}

// DeriveAgentTruth materializes runtime root .harness/agents/* from active harness package truth.
func DeriveAgentTruth(_ context.Context, repoRoot string) (AgentDeriveResult, error) {
	records, warnings, err := loadActiveHarnessPackageRecords(repoRoot)
	if err != nil {
		return AgentDeriveResult{}, err
	}

	desiredFrameworks, hasDesiredFrameworks, err := deriveRecommendedFramework(records)
	if err != nil {
		return AgentDeriveResult{}, err
	}
	desiredAgentConfig, hasDesiredAgentConfig, err := deriveAgentConfig(records)
	if err != nil {
		return AgentDeriveResult{}, err
	}
	desiredOverlays, err := deriveAgentOverlays(records)
	if err != nil {
		return AgentDeriveResult{}, err
	}

	desiredFiles := make(map[string][]byte)
	if hasDesiredFrameworks || hasDesiredAgentConfig || len(desiredOverlays) > 0 {
		frameworksFile := FrameworksFile{SchemaVersion: frameworksSchemaVersion}
		if hasDesiredFrameworks {
			frameworksFile.RecommendedFramework = desiredFrameworks.RecommendedFramework
		}
		data, err := MarshalFrameworksFile(frameworksFile)
		if err != nil {
			return AgentDeriveResult{}, fmt.Errorf("marshal derived frameworks file: %w", err)
		}
		desiredFiles[FrameworksPath(repoRoot)] = data
	}
	if hasDesiredAgentConfig {
		data, err := MarshalAgentConfigFile(desiredAgentConfig)
		if err != nil {
			return AgentDeriveResult{}, fmt.Errorf("marshal derived agent config file: %w", err)
		}
		desiredFiles[AgentConfigPath(repoRoot)] = data
	}
	for agentID, overlay := range desiredOverlays {
		desiredFiles[AgentOverlayPath(repoRoot, agentID)] = append([]byte(nil), overlay.Content...)
	}

	existingPaths, err := listExistingAgentTruthPaths(repoRoot)
	if err != nil {
		return AgentDeriveResult{}, err
	}
	touchedPaths := unionSortedPaths(existingPaths, desiredFiles)
	if err := applyDerivedAgentTruth(touchedPaths, desiredFiles); err != nil {
		return AgentDeriveResult{}, err
	}

	writtenPaths := make([]string, 0, len(desiredFiles))
	for path := range desiredFiles {
		writtenPaths = append(writtenPaths, filepath.ToSlash(path[len(repoRoot)+1:]))
	}
	sort.Strings(writtenPaths)

	result := AgentDeriveResult{
		WrittenPaths: writtenPaths,
		PackageCount: len(records),
		Warnings:     warnings,
	}
	if hasDesiredFrameworks {
		result.RecommendedFramework = desiredFrameworks.RecommendedFramework
	}

	return result, nil
}

func deriveRecommendedFramework(records []BundleRecord) (FrameworksFile, bool, error) {
	unique := make(map[string]struct{})
	for _, record := range records {
		if record.RecommendedFramework == "" {
			continue
		}
		unique[record.RecommendedFramework] = struct{}{}
	}
	switch len(unique) {
	case 0:
		return FrameworksFile{}, false, nil
	case 1:
		for frameworkID := range unique {
			return FrameworksFile{
				SchemaVersion:        frameworksSchemaVersion,
				RecommendedFramework: frameworkID,
			}, true, nil
		}
	default:
		recommendations := make([]FrameworkPackageRecommendation, 0, len(records))
		for _, record := range records {
			if record.RecommendedFramework == "" {
				continue
			}
			recommendations = append(recommendations, FrameworkPackageRecommendation{
				HarnessID:            record.HarnessID,
				RecommendedFramework: record.RecommendedFramework,
			})
		}
		return FrameworksFile{}, false, fmt.Errorf("derive recommended framework conflict: %s", formatFrameworkRecommendationConflictWarning(recommendations))
	}

	return FrameworksFile{}, false, nil
}

func deriveAgentConfig(records []BundleRecord) (AgentConfigFile, bool, error) {
	var (
		desired AgentConfigFile
		ok      bool
	)
	for _, record := range records {
		if record.AgentConfig == nil {
			continue
		}
		if !ok {
			desired = *record.AgentConfig
			ok = true
			continue
		}
		if desired.SchemaVersion != record.AgentConfig.SchemaVersion {
			return AgentConfigFile{}, false, fmt.Errorf("derive agent config conflict between package %q and another active package", record.HarnessID)
		}
	}

	return desired, ok, nil
}

func deriveAgentOverlays(records []BundleRecord) (map[string]AgentOverlayFile, error) {
	derived := make(map[string]AgentOverlayFile)
	for _, record := range records {
		for agentID, content := range record.AgentOverlays {
			file, err := ParseAgentOverlayFileData([]byte(content))
			if err != nil {
				return nil, fmt.Errorf("parse overlay %q from package %q: %w", agentID, record.HarnessID, err)
			}
			existing, ok := derived[agentID]
			if !ok {
				derived[agentID] = file
				continue
			}
			if string(existing.Content) != string(file.Content) {
				return nil, fmt.Errorf("derive overlay conflict for %q across active packages", agentID)
			}
		}
	}

	return derived, nil
}

func listExistingAgentTruthPaths(repoRoot string) ([]string, error) {
	paths := make([]string, 0, 4)
	if _, err := os.Stat(FrameworksPath(repoRoot)); err == nil {
		paths = append(paths, FrameworksPath(repoRoot))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat frameworks file: %w", err)
	}
	if _, err := os.Stat(legacyFrameworksPath(repoRoot)); err == nil {
		paths = append(paths, legacyFrameworksPath(repoRoot))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat legacy frameworks file: %w", err)
	}
	if _, err := os.Stat(AgentConfigPath(repoRoot)); err == nil {
		paths = append(paths, AgentConfigPath(repoRoot))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat agent config file: %w", err)
	}

	overlayIDs, err := ListAgentOverlayIDs(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, agentID := range overlayIDs {
		paths = append(paths, AgentOverlayPath(repoRoot, agentID))
	}
	sort.Strings(paths)

	return paths, nil
}

func unionSortedPaths(existingPaths []string, desiredFiles map[string][]byte) []string {
	set := make(map[string]struct{}, len(existingPaths)+len(desiredFiles))
	for _, path := range existingPaths {
		set[path] = struct{}{}
	}
	for path := range desiredFiles {
		set[path] = struct{}{}
	}

	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return paths
}

func applyDerivedAgentTruth(touchedPaths []string, desiredFiles map[string][]byte) error {
	backups := make(map[string][]byte, len(touchedPaths))
	previouslyPresent := make(map[string]bool, len(touchedPaths))
	for _, path := range touchedPaths {
		data, err := os.ReadFile(path) //nolint:gosec // Paths are repo-local agent truth hosts resolved under the current repo root.
		if err == nil {
			backups[path] = data
			previouslyPresent[path] = true
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read existing derived agent truth %s: %w", path, err)
		}
	}

	applied := make([]string, 0, len(touchedPaths))
	for _, path := range touchedPaths {
		data, ok := desiredFiles[path]
		if ok {
			if err := contractutil.AtomicWriteFile(path, data); err != nil {
				rollbackDerivedAgentTruth(applied, backups, previouslyPresent)
				return fmt.Errorf("write derived agent truth %s: %w", path, err)
			}
			applied = append(applied, path)
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackDerivedAgentTruth(applied, backups, previouslyPresent)
			return fmt.Errorf("remove stale derived agent truth %s: %w", path, err)
		}
		applied = append(applied, path)
	}

	return nil
}

func rollbackDerivedAgentTruth(applied []string, backups map[string][]byte, previouslyPresent map[string]bool) {
	for index := len(applied) - 1; index >= 0; index-- {
		path := applied[index]
		if previouslyPresent[path] {
			if err := contractutil.AtomicWriteFile(path, backups[path]); err != nil {
				_ = os.Remove(path)
			}
			continue
		}
		_ = os.Remove(path)
	}
}
