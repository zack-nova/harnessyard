package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// CheckFindingKind is one stable harness-check diagnostic kind.
type CheckFindingKind string

const (
	CheckFindingManifestSchemaInvalid CheckFindingKind = "manifest_schema_invalid"
	CheckFindingMissingDefinition     CheckFindingKind = "missing_definition"
	CheckFindingInstallMemberMismatch CheckFindingKind = "install_member_mismatch"
	CheckFindingInstallRecordInvalid  CheckFindingKind = "install_record_invalid"
	CheckFindingInstallPathMismatch   CheckFindingKind = "install_path_mismatch"
	CheckFindingUnresolvedBindings    CheckFindingKind = "unresolved_bindings"
	CheckFindingBundleMemberMismatch  CheckFindingKind = "bundle_member_mismatch"
	CheckFindingBundlePathMismatch    CheckFindingKind = "bundle_path_mismatch"
)

// CheckFinding captures one stable harness-check diagnostic.
type CheckFinding struct {
	Kind    CheckFindingKind `json:"kind"`
	OrbitID string           `json:"orbit_id,omitempty"`
	Path    string           `json:"path,omitempty"`
	Message string           `json:"message"`
}

// CheckBindingsSummary captures low-noise unresolved binding diagnostics.
type CheckBindingsSummary struct {
	UnresolvedInstallCount  int      `json:"unresolved_install_count"`
	UnresolvedVariableCount int      `json:"unresolved_variable_count"`
	OrbitIDs                []string `json:"orbit_ids,omitempty"`
	BundleIDs               []string `json:"bundle_ids,omitempty"`
}

// CheckResult captures the current harness-check result.
type CheckResult struct {
	HarnessID       string                `json:"harness_id,omitempty"`
	OK              bool                  `json:"ok"`
	FindingCount    int                   `json:"finding_count"`
	Findings        []CheckFinding        `json:"findings"`
	BindingsSummary *CheckBindingsSummary `json:"bindings_summary,omitempty"`
}

// CheckRuntime analyzes the current harness runtime and reports stable diagnostics.
func CheckRuntime(ctx context.Context, repoRoot string) (CheckResult, error) {
	return CheckRuntimeWithProgress(ctx, repoRoot, nil)
}

// CheckRuntimeWithProgress analyzes the current harness runtime and emits coarse progress stages when requested.
func CheckRuntimeWithProgress(ctx context.Context, repoRoot string, progress func(string) error) (CheckResult, error) {
	manifestData, readErr := os.ReadFile(ManifestPath(repoRoot))
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return CheckResult{}, fmt.Errorf("read %s: %w", ManifestPath(repoRoot), readErr)
		}
		return CheckResult{}, fmt.Errorf("read %s: %w", ManifestPath(repoRoot), readErr)
	}

	runtimeFile, schemaFinding := parseManifestFileForCheck(manifestData)
	if schemaFinding != nil {
		findings := []CheckFinding{*schemaFinding}
		return CheckResult{
			OK:           false,
			FindingCount: len(findings),
			Findings:     findings,
		}, nil
	}

	if err := checkRuntimeStage(progress, "scanning harness records"); err != nil {
		return CheckResult{}, err
	}
	findings, bindingsSummary, err := collectMembershipFindings(ctx, repoRoot, runtimeFile, progress)
	if err != nil {
		return CheckResult{}, err
	}
	sortCheckFindings(findings)

	result := CheckResult{
		HarnessID:       runtimeFile.Harness.ID,
		OK:              !hasBlockingFindings(findings),
		FindingCount:    len(findings),
		Findings:        findings,
		BindingsSummary: bindingsSummaryOrNil(bindingsSummary),
	}
	if err := checkRuntimeStage(progress, "check complete"); err != nil {
		return CheckResult{}, err
	}

	return result, nil
}

func hasBlockingFindings(findings []CheckFinding) bool {
	for _, finding := range findings {
		if checkFindingIsWarningOnly(finding.Kind) {
			continue
		}
		return true
	}

	return false
}

func checkFindingIsWarningOnly(kind CheckFindingKind) bool {
	switch kind {
	case CheckFindingUnresolvedBindings:
		return true
	default:
		return false
	}
}

func parseManifestFileForCheck(data []byte) (RuntimeFile, *CheckFinding) {
	manifestFile, err := ParseManifestFileData(data)
	if err == nil {
		runtimeFile, convertErr := RuntimeFileFromManifestFile(manifestFile)
		if convertErr == nil {
			return runtimeFile, nil
		}
		err = convertErr
	}

	return RuntimeFile{}, &CheckFinding{
		Kind:    CheckFindingManifestSchemaInvalid,
		Path:    ManifestRepoPath(),
		Message: err.Error(),
	}
}

func collectMembershipFindings(
	ctx context.Context,
	repoRoot string,
	runtimeFile RuntimeFile,
	progress func(string) error,
) ([]CheckFinding, CheckBindingsSummary, error) {
	findings := make([]CheckFinding, 0)
	validInstallRecords, installFindings, bindingsSummary, err := scanInstallRecords(repoRoot)
	if err != nil {
		return nil, CheckBindingsSummary{}, err
	}
	findings = append(findings, installFindings...)
	validBundleRecords, bundleFindings, bundleBindingsSummary, err := scanBundleRecords(repoRoot)
	if err != nil {
		return nil, CheckBindingsSummary{}, err
	}
	findings = append(findings, bundleFindings...)
	bindingsSummary = mergeCheckBindingsSummaries(bindingsSummary, bundleBindingsSummary)

	installMembers := make(map[string]struct{})
	bundleMembersByHarnessID := make(map[string]map[string]struct{})
	installCheckStageEmitted := false

	for _, member := range runtimeFile.Members {
		definitionPath, err := orbitpkg.HostedDefinitionRelativePath(member.OrbitID)
		if err != nil {
			return nil, CheckBindingsSummary{}, fmt.Errorf("build definition path for %q: %w", member.OrbitID, err)
		}

		if _, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, member.OrbitID); err != nil {
			findings = append(findings, CheckFinding{
				Kind:    CheckFindingMissingDefinition,
				OrbitID: member.OrbitID,
				Path:    definitionPath,
				Message: fmt.Sprintf("member %q points to a missing or invalid orbit definition", member.OrbitID),
			})
		}

		switch member.Source {
		case MemberSourceInstallOrbit:
			if !installCheckStageEmitted {
				if err := checkRuntimeStage(progress, "checking install-backed members"); err != nil {
					return nil, CheckBindingsSummary{}, err
				}
				installCheckStageEmitted = true
			}
			installMembers[member.OrbitID] = struct{}{}
			if _, ok := validInstallRecords[member.OrbitID]; !ok {
				installPath, pathErr := InstallRecordRepoPath(member.OrbitID)
				if pathErr != nil {
					return nil, CheckBindingsSummary{}, fmt.Errorf("build install record path for %q: %w", member.OrbitID, pathErr)
				}
				findings = append(findings, CheckFinding{
					Kind:    CheckFindingInstallMemberMismatch,
					OrbitID: member.OrbitID,
					Path:    installPath,
					Message: fmt.Sprintf("install-backed member %q has missing install record", member.OrbitID),
				})
				continue
			}

			driftFindings, err := orbittemplate.AnalyzeInstalledTemplateDrift(ctx, repoRoot, member.OrbitID)
			if err != nil {
				return nil, CheckBindingsSummary{}, fmt.Errorf("analyze installed template drift for %q: %w", member.OrbitID, err)
			}
			for _, driftFinding := range driftFindings {
				findings = append(findings, CheckFinding{
					Kind:    CheckFindingKind(driftFinding.Kind),
					OrbitID: member.OrbitID,
					Path:    driftFinding.Path,
					Message: driftMessageForFinding(member.OrbitID, driftFinding),
				})
			}
		case MemberSourceInstallBundle:
			record, ok := findBundleRecordForOwnedMember(validBundleRecords, member)
			if !ok {
				findings = append(findings, CheckFinding{
					Kind:    CheckFindingBundleMemberMismatch,
					OrbitID: member.OrbitID,
					Path:    BundleRecordsDirRepoPath(),
					Message: fmt.Sprintf(
						"bundle-backed member %q with owner_harness_id %q has no matching bundle record",
						member.OrbitID,
						member.OwnerHarnessID,
					),
				})
				continue
			}
			if _, ok := bundleMembersByHarnessID[record.Record.HarnessID]; !ok {
				bundleMembersByHarnessID[record.Record.HarnessID] = make(map[string]struct{})
			}
			bundleMembersByHarnessID[record.Record.HarnessID][member.OrbitID] = struct{}{}
		}
	}

	for orbitID, record := range validInstallRecords {
		if _, ok := installMembers[orbitID]; ok {
			continue
		}
		if record.Status == orbittemplate.InstallRecordStatusDetached {
			continue
		}
		findings = append(findings, CheckFinding{
			Kind:    CheckFindingInstallMemberMismatch,
			OrbitID: orbitID,
			Path:    record.Path,
			Message: fmt.Sprintf("install record %q has no matching install-backed member", orbitID),
		})
	}

	for harnessID, bundlePath := range validBundleRecords {
		if bundleRecordHasAnyMember(bundleMembersByHarnessID[harnessID], validBundleRecords[harnessID]) {
			continue
		}
		findings = append(findings, CheckFinding{
			Kind:    CheckFindingBundleMemberMismatch,
			OrbitID: harnessID,
			Path:    bundlePath.Path,
			Message: fmt.Sprintf("bundle record %q has no matching bundle-backed members", harnessID),
		})
	}

	return findings, bindingsSummary, nil
}

func checkRuntimeStage(progress func(string) error, stage string) error {
	if progress == nil {
		return nil
	}

	return progress(stage)
}

type validBundleRecord struct {
	Path   string
	Record BundleRecord
}

type validInstallRecord struct {
	Path   string
	Status string
}

func scanInstallRecords(repoRoot string) (map[string]validInstallRecord, []CheckFinding, CheckBindingsSummary, error) {
	entries, err := os.ReadDir(InstallRecordsDirPath(repoRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]validInstallRecord{}, []CheckFinding{}, CheckBindingsSummary{}, nil
		}
		return nil, nil, CheckBindingsSummary{}, fmt.Errorf("read harness install records directory: %w", err)
	}

	validRecords := make(map[string]validInstallRecord)
	findings := make([]CheckFinding, 0)
	bindingsSummary := CheckBindingsSummary{}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".yaml" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		absolutePath := filepath.Join(InstallRecordsDirPath(repoRoot), name)
		repoPath := pathpkg.Join(InstallRecordsDirRepoPath(), name)

		//nolint:gosec // Paths come from the fixed repo-local .harness/installs directory.
		data, err := os.ReadFile(absolutePath)
		if err != nil {
			return nil, nil, CheckBindingsSummary{}, fmt.Errorf("read %s: %w", absolutePath, err)
		}

		record, err := orbittemplate.ParseInstallRecordData(data)
		if err != nil {
			findings = append(findings, CheckFinding{
				Kind:    CheckFindingInstallRecordInvalid,
				Path:    repoPath,
				Message: fmt.Sprintf("install record is invalid: %v", err),
			})
			continue
		}

		expectedName := record.OrbitID + ".yaml"
		if name != expectedName {
			findings = append(findings, CheckFinding{
				Kind:    CheckFindingInstallPathMismatch,
				OrbitID: record.OrbitID,
				Path:    repoPath,
				Message: fmt.Sprintf("install record path %q does not match orbit_id %q", repoPath, record.OrbitID),
			})
			continue
		}
		if record.Variables != nil && len(record.Variables.UnresolvedAtApply) > 0 {
			bindingsSummary.UnresolvedInstallCount++
			bindingsSummary.UnresolvedVariableCount += len(record.Variables.UnresolvedAtApply)
			bindingsSummary.OrbitIDs = append(bindingsSummary.OrbitIDs, record.OrbitID)
		}

		validRecords[record.OrbitID] = validInstallRecord{
			Path:   repoPath,
			Status: orbittemplate.EffectiveInstallRecordStatus(record),
		}
	}

	return validRecords, findings, bindingsSummary, nil
}

func bindingsSummaryOrNil(summary CheckBindingsSummary) *CheckBindingsSummary {
	if summary.UnresolvedInstallCount == 0 && summary.UnresolvedVariableCount == 0 {
		return nil
	}
	sort.Strings(summary.OrbitIDs)
	sort.Strings(summary.BundleIDs)
	return &summary
}

func mergeCheckBindingsSummaries(summaries ...CheckBindingsSummary) CheckBindingsSummary {
	merged := CheckBindingsSummary{}
	for _, summary := range summaries {
		merged.UnresolvedInstallCount += summary.UnresolvedInstallCount
		merged.UnresolvedVariableCount += summary.UnresolvedVariableCount
		merged.OrbitIDs = append(merged.OrbitIDs, summary.OrbitIDs...)
		merged.BundleIDs = append(merged.BundleIDs, summary.BundleIDs...)
	}
	return merged
}

func scanBundleRecords(repoRoot string) (map[string]validBundleRecord, []CheckFinding, CheckBindingsSummary, error) {
	entries, err := os.ReadDir(BundleRecordsDirPath(repoRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]validBundleRecord{}, []CheckFinding{}, CheckBindingsSummary{}, nil
		}
		return nil, nil, CheckBindingsSummary{}, fmt.Errorf("read harness bundle records directory: %w", err)
	}

	validRecords := make(map[string]validBundleRecord)
	findings := make([]CheckFinding, 0)
	bindingsSummary := CheckBindingsSummary{}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".yaml" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		absolutePath := filepath.Join(BundleRecordsDirPath(repoRoot), name)
		repoPath := pathpkg.Join(BundleRecordsDirRepoPath(), name)

		//nolint:gosec // Paths come from the fixed repo-local .harness/bundles directory.
		data, err := os.ReadFile(absolutePath)
		if err != nil {
			return nil, nil, CheckBindingsSummary{}, fmt.Errorf("read %s: %w", absolutePath, err)
		}

		record, err := ParseBundleRecordData(data)
		if err != nil {
			findings = append(findings, CheckFinding{
				Kind:    CheckFindingBundlePathMismatch,
				Path:    repoPath,
				Message: fmt.Sprintf("bundle record is invalid: %v", err),
			})
			continue
		}

		expectedName := record.HarnessID + ".yaml"
		if name != expectedName {
			findings = append(findings, CheckFinding{
				Kind:    CheckFindingBundlePathMismatch,
				OrbitID: record.HarnessID,
				Path:    repoPath,
				Message: fmt.Sprintf("bundle record path %q does not match harness_id %q", repoPath, record.HarnessID),
			})
			continue
		}

		validRecords[record.HarnessID] = validBundleRecord{
			Path:   repoPath,
			Record: record,
		}
		if record.Variables != nil && len(record.Variables.UnresolvedAtApply) > 0 {
			bindingsSummary.UnresolvedInstallCount++
			bindingsSummary.UnresolvedVariableCount += len(record.Variables.UnresolvedAtApply)
			bindingsSummary.BundleIDs = append(bindingsSummary.BundleIDs, record.HarnessID)
		}
	}

	return validRecords, findings, bindingsSummary, nil
}

func findBundleRecordForOwnedMember(records map[string]validBundleRecord, member RuntimeMember) (validBundleRecord, bool) {
	record, ok := records[member.OwnerHarnessID]
	if !ok {
		return validBundleRecord{}, false
	}

	for _, memberID := range record.Record.MemberIDs {
		if memberID == member.OrbitID {
			return record, true
		}
	}

	return validBundleRecord{}, false
}

func bundleRecordHasAnyMember(bundleMembers map[string]struct{}, record validBundleRecord) bool {
	for _, memberID := range record.Record.MemberIDs {
		if _, ok := bundleMembers[memberID]; ok {
			return true
		}
	}

	return false
}

func sortCheckFindings(findings []CheckFinding) {
	sort.Slice(findings, func(left, right int) bool {
		if findings[left].Path == findings[right].Path {
			if findings[left].OrbitID == findings[right].OrbitID {
				return findings[left].Kind < findings[right].Kind
			}
			return findings[left].OrbitID < findings[right].OrbitID
		}
		return findings[left].Path < findings[right].Path
	})
}

func driftMessageForFinding(orbitID string, finding orbittemplate.InstallDriftFinding) string {
	switch finding.Kind {
	case orbittemplate.DriftKindDefinition:
		return fmt.Sprintf("install-backed member %q definition drift detected", orbitID)
	case orbittemplate.DriftKindRuntimeFile:
		return fmt.Sprintf("install-backed member %q runtime file drift detected", orbitID)
	case orbittemplate.DriftKindProvenanceUnresolvable:
		return fmt.Sprintf("install-backed member %q provenance could not be reconstructed", orbitID)
	default:
		return fmt.Sprintf("install-backed member %q drift detected", orbitID)
	}
}
