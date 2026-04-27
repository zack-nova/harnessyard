package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// FrameworkSelectionSource describes how one current framework was chosen.
type FrameworkSelectionSource string

const (
	FrameworkSelectionSourceExplicitLocal         FrameworkSelectionSource = "explicit_local"
	FrameworkSelectionSourceLocalHint             FrameworkSelectionSource = "local_hint"
	FrameworkSelectionSourceProjectDetection      FrameworkSelectionSource = "project_detection"
	FrameworkSelectionSourceRecommendedDefault    FrameworkSelectionSource = "recommended_default"
	FrameworkSelectionSourcePackageRecommendation FrameworkSelectionSource = "package_recommendation"
	FrameworkSelectionSourceUnresolvedConflict    FrameworkSelectionSource = "unresolved_conflict"
	FrameworkSelectionSourceUnresolved            FrameworkSelectionSource = "unresolved"
)

// FrameworkSelection stores repo-local selected framework state.
type FrameworkSelection struct {
	SelectedFramework string                   `json:"selected_framework"`
	SelectionSource   FrameworkSelectionSource `json:"selection_source"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

// FrameworkResolutionInput captures one resolution request.
type FrameworkResolutionInput struct {
	RepoRoot string
	GitDir   string
}

// FrameworkPackageRecommendation captures one active installed harness package recommendation.
type FrameworkPackageRecommendation struct {
	HarnessID            string `json:"harness_id"`
	RecommendedFramework string `json:"recommended_framework"`
}

// FrameworkResolution captures one resolved framework plus provenance and warnings.
type FrameworkResolution struct {
	Framework              string                           `json:"framework"`
	Source                 FrameworkSelectionSource         `json:"source"`
	RecommendedFramework   string                           `json:"recommended_framework,omitempty"`
	PackageRecommendations []FrameworkPackageRecommendation `json:"package_recommendations,omitempty"`
	SupportedFrameworks    []string                         `json:"supported_frameworks,omitempty"`
	Warnings               []string                         `json:"warnings,omitempty"`
}

// FrameworkSelectionPath returns the repo-local selection file path.
func FrameworkSelectionPath(gitDir string) string {
	return filepath.Join(gitDir, "orbit", "state", "agents", "selection.json")
}

func legacyFrameworkSelectionPath(gitDir string) string {
	return filepath.Join(gitDir, "orbit", "state", "frameworks", "selection.json")
}

// LoadFrameworkSelection reads and validates the current repo-local framework selection.
func LoadFrameworkSelection(gitDir string) (FrameworkSelection, error) {
	filename := FrameworkSelectionPath(gitDir)
	selection, err := loadFrameworkSelectionAtPath(filename)
	if err == nil {
		return selection, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return FrameworkSelection{}, err
	}

	return loadFrameworkSelectionAtPath(legacyFrameworkSelectionPath(gitDir))
}

func loadFrameworkSelectionAtPath(filename string) (FrameworkSelection, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // The path is repo-local and built from the fixed framework selection contract path.
	if err != nil {
		return FrameworkSelection{}, fmt.Errorf("read %s: %w", filename, err)
	}

	var selection FrameworkSelection
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&selection); err != nil {
		return FrameworkSelection{}, fmt.Errorf("decode %s: %w", filename, err)
	}
	if err := ValidateFrameworkSelection(selection); err != nil {
		return FrameworkSelection{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return selection, nil
}

// ValidateFrameworkSelection validates one repo-local framework selection payload.
func ValidateFrameworkSelection(selection FrameworkSelection) error {
	if err := ids.ValidateOrbitID(selection.SelectedFramework); err != nil {
		return fmt.Errorf("selected_framework: %w", err)
	}
	switch selection.SelectionSource {
	case FrameworkSelectionSourceExplicitLocal,
		FrameworkSelectionSourceLocalHint,
		FrameworkSelectionSourceProjectDetection,
		FrameworkSelectionSourceRecommendedDefault,
		FrameworkSelectionSourcePackageRecommendation:
	default:
		return fmt.Errorf("selection_source must be one of %q, %q, %q, %q, or %q",
			FrameworkSelectionSourceExplicitLocal,
			FrameworkSelectionSourceLocalHint,
			FrameworkSelectionSourceProjectDetection,
			FrameworkSelectionSourceRecommendedDefault,
			FrameworkSelectionSourcePackageRecommendation,
		)
	}
	if selection.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at must be set")
	}

	return nil
}

// WriteFrameworkSelection validates and writes one repo-local framework selection file.
func WriteFrameworkSelection(gitDir string, selection FrameworkSelection) (string, error) {
	if err := ValidateFrameworkSelection(selection); err != nil {
		return "", fmt.Errorf("validate framework selection: %w", err)
	}

	filename := FrameworkSelectionPath(gitDir)
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode framework selection: %w", err)
	}
	if err := contractutil.AtomicWriteFile(filename, append(data, '\n')); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}
	if err := os.Remove(legacyFrameworkSelectionPath(gitDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove legacy framework selection: %w", err)
	}

	return filename, nil
}

// ResolveFramework resolves the current framework using repo-local selection, local hints, project detection, and recommendations.
func ResolveFramework(ctx context.Context, input FrameworkResolutionInput) (FrameworkResolution, error) {
	_ = ctx

	adapters := RegisteredFrameworkAdapters()
	supportedFrameworks := make([]string, 0, len(adapters))
	supportedSet := make(map[string]struct{}, len(adapters))
	for _, adapter := range adapters {
		supportedFrameworks = append(supportedFrameworks, adapter.ID)
		supportedSet[adapter.ID] = struct{}{}
	}
	sort.Strings(supportedFrameworks)

	frameworksFile, err := LoadOptionalFrameworksFile(input.RepoRoot)
	if err != nil {
		return FrameworkResolution{}, fmt.Errorf("load harness frameworks file: %w", err)
	}

	resolution := FrameworkResolution{
		Source:               FrameworkSelectionSourceUnresolved,
		RecommendedFramework: frameworksFile.RecommendedFramework,
		SupportedFrameworks:  supportedFrameworks,
	}

	if selection, err := LoadFrameworkSelection(input.GitDir); err == nil {
		if _, ok := supportedSet[selection.SelectedFramework]; ok {
			resolution.Framework = selection.SelectedFramework
			resolution.Source = FrameworkSelectionSourceExplicitLocal
			return resolution, nil
		}
		resolution.Warnings = append(resolution.Warnings, fmt.Sprintf(`ignore unsupported explicit local framework selection %q`, selection.SelectedFramework))
	} else if !errors.Is(err, os.ErrNotExist) {
		return FrameworkResolution{}, fmt.Errorf("load framework selection: %w", err)
	}

	localHintMatches, err := detectFrameworkLevel(input.RepoRoot, adapters, frameworkDetectionModeLocalHint)
	if err != nil {
		return FrameworkResolution{}, err
	}
	if len(localHintMatches) == 1 {
		resolution.Framework = localHintMatches[0]
		resolution.Source = FrameworkSelectionSourceLocalHint
		return resolution, nil
	}
	if len(localHintMatches) > 1 {
		resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("multiple framework local hints detected: %v", localHintMatches))
		return resolution, nil
	}

	projectMatches, err := detectFrameworkLevel(input.RepoRoot, adapters, frameworkDetectionModeProject)
	if err != nil {
		return FrameworkResolution{}, err
	}
	if len(projectMatches) == 1 {
		resolution.Framework = projectMatches[0]
		resolution.Source = FrameworkSelectionSourceProjectDetection
		return resolution, nil
	}
	if len(projectMatches) > 1 {
		resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("multiple framework project detections detected: %v", projectMatches))
		return resolution, nil
	}

	if frameworksFile.RecommendedFramework != "" {
		if _, ok := supportedSet[frameworksFile.RecommendedFramework]; !ok {
			resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("ignore unsupported recommended framework %q", frameworksFile.RecommendedFramework))
		} else {
			resolution.Framework = frameworksFile.RecommendedFramework
			resolution.Source = FrameworkSelectionSourceRecommendedDefault
			return resolution, nil
		}
	}

	packageRecommendations, packageWarnings, err := loadFrameworkPackageRecommendations(input.RepoRoot)
	if err != nil {
		return FrameworkResolution{}, err
	}
	resolution.PackageRecommendations = packageRecommendations
	resolution.Warnings = append(resolution.Warnings, packageWarnings...)

	uniqueRecommendations := make(map[string]struct{}, len(packageRecommendations))
	for _, recommendation := range packageRecommendations {
		if _, ok := supportedSet[recommendation.RecommendedFramework]; !ok {
			resolution.Warnings = append(resolution.Warnings, fmt.Sprintf(
				`ignore unsupported package framework recommendation %q from harness %q`,
				recommendation.RecommendedFramework,
				recommendation.HarnessID,
			))
			continue
		}
		uniqueRecommendations[recommendation.RecommendedFramework] = struct{}{}
	}
	switch len(uniqueRecommendations) {
	case 0:
		return resolution, nil
	case 1:
		for frameworkID := range uniqueRecommendations {
			resolution.Framework = frameworkID
		}
		resolution.Source = FrameworkSelectionSourcePackageRecommendation
		return resolution, nil
	default:
		resolution.Source = FrameworkSelectionSourceUnresolvedConflict
		resolution.Warnings = append(resolution.Warnings, formatFrameworkRecommendationConflictWarning(packageRecommendations))
		return resolution, nil
	}
}

func loadFrameworkPackageRecommendations(repoRoot string) ([]FrameworkPackageRecommendation, []string, error) {
	records, warnings, err := loadActiveHarnessPackageRecords(repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("load package framework recommendation sources: %w", err)
	}

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

	return recommendations, warnings, nil
}

func loadActiveHarnessPackageRecords(repoRoot string) ([]BundleRecord, []string, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []BundleRecord{}, []string{}, nil
		}
		return nil, nil, fmt.Errorf("load runtime file: %w", err)
	}

	activeHarnessIDs := make(map[string]struct{}, len(runtimeFile.Members))
	for _, member := range runtimeFile.Members {
		if member.OwnerHarnessID == "" {
			continue
		}
		activeHarnessIDs[member.OwnerHarnessID] = struct{}{}
	}

	harnessIDs := make([]string, 0, len(activeHarnessIDs))
	for harnessID := range activeHarnessIDs {
		harnessIDs = append(harnessIDs, harnessID)
	}
	sort.Strings(harnessIDs)

	records := make([]BundleRecord, 0, len(harnessIDs))
	warnings := make([]string, 0)
	for _, harnessID := range harnessIDs {
		record, err := LoadBundleRecord(repoRoot, harnessID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				warnings = append(warnings, fmt.Sprintf(`ignore missing active harness package truth source %q`, harnessID))
				continue
			}
			return nil, nil, fmt.Errorf("load bundle record %q: %w", harnessID, err)
		}
		records = append(records, record)
	}

	return records, warnings, nil
}

func formatFrameworkRecommendationConflictWarning(recommendations []FrameworkPackageRecommendation) string {
	parts := make([]string, 0, len(recommendations))
	for _, recommendation := range recommendations {
		parts = append(parts, fmt.Sprintf("%s=%s", recommendation.HarnessID, recommendation.RecommendedFramework))
	}
	sort.Strings(parts)

	return fmt.Sprintf("conflicting package framework recommendations detected: %s", joinCommaSeparated(parts))
}

func joinCommaSeparated(values []string) string {
	return strings.Join(values, ", ")
}
