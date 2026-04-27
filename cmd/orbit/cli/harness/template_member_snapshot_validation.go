package harness

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type templateMemberSnapshotValidation struct {
	pathContributors map[string][]string
}

func validateTemplateMemberSnapshots(source LocalTemplateInstallSource) (templateMemberSnapshotValidation, error) {
	result := templateMemberSnapshotValidation{
		pathContributors: map[string][]string{},
	}

	if len(source.MemberSnapshots) == 0 {
		return result, nil
	}

	memberSet := make(map[string]struct{}, len(source.Manifest.Members))
	for _, member := range source.Manifest.Members {
		memberSet[member.OrbitID] = struct{}{}
		if _, ok := source.MemberSnapshots[member.OrbitID]; !ok {
			return templateMemberSnapshotValidation{}, fmt.Errorf("template member snapshot for %q is required", member.OrbitID)
		}
	}
	for memberID := range source.MemberSnapshots {
		if _, ok := memberSet[memberID]; !ok {
			return templateMemberSnapshotValidation{}, fmt.Errorf("template member snapshot for %q is not declared in template members", memberID)
		}
	}

	payloadDigests := make(map[string]string, len(source.Files))
	payloadFilesByPath := make(map[string]orbittemplate.CandidateFile, len(source.Files))
	for _, file := range source.Files {
		payloadDigests[file.Path] = contentDigest(file.Content)
		payloadFilesByPath[file.Path] = cloneCandidateFile(file)
	}

	sharedDigests := make(map[string]string)
	for _, member := range source.Manifest.Members {
		snapshot := source.MemberSnapshots[member.OrbitID]
		memberFiles := make([]orbittemplate.CandidateFile, 0, len(snapshot.Snapshot.ExportedPaths))
		for _, path := range snapshot.Snapshot.ExportedPaths {
			actualDigest, ok := payloadDigests[path]
			if !ok {
				return templateMemberSnapshotValidation{}, fmt.Errorf("snapshot path %q is missing from template payload", path)
			}

			expectedDigest := snapshot.Snapshot.FileDigests[path]
			if sharedDigest, ok := sharedDigests[path]; ok && sharedDigest != expectedDigest {
				return templateMemberSnapshotValidation{}, fmt.Errorf("shared snapshot path %q has inconsistent digests", path)
			}
			if actualDigest != expectedDigest {
				return templateMemberSnapshotValidation{}, fmt.Errorf("snapshot path %q digest does not match template payload", path)
			}

			sharedDigests[path] = expectedDigest
			result.pathContributors[path] = append(result.pathContributors[path], member.OrbitID)
			memberFiles = append(memberFiles, cloneCandidateFile(payloadFilesByPath[path]))
		}

		scanResult := orbittemplate.ScanVariables(memberFiles, templateManifestVariableSpecs(source.Manifest.Variables))
		if len(scanResult.Undeclared) > 0 {
			return templateMemberSnapshotValidation{}, fmt.Errorf(
				"template member snapshot for %q references undeclared variables: %s",
				member.OrbitID,
				strings.Join(scanResult.Undeclared, ", "),
			)
		}

		snapshotVariableNames := sortedTemplateSnapshotVariableNames(snapshot.Snapshot.Variables)
		if !slices.Equal(scanResult.Referenced, snapshotVariableNames) {
			return templateMemberSnapshotValidation{}, fmt.Errorf("template member snapshot for %q has variable summary drift", member.OrbitID)
		}
		for _, name := range snapshotVariableNames {
			manifestSpec, ok := source.Manifest.Variables[name]
			if !ok {
				return templateMemberSnapshotValidation{}, fmt.Errorf(
					"template member snapshot for %q references variable %q missing from template manifest",
					member.OrbitID,
					name,
				)
			}
			if snapshot.Snapshot.Variables[name] != manifestSpec {
				return templateMemberSnapshotValidation{}, fmt.Errorf(
					"template member snapshot for %q variable %q does not match template manifest",
					member.OrbitID,
					name,
				)
			}
		}
	}

	unownedPaths := make([]string, 0)
	for path := range payloadDigests {
		if path == rootAgentsPath {
			continue
		}
		if _, ok := result.pathContributors[path]; ok {
			continue
		}
		unownedPaths = append(unownedPaths, path)
	}
	if len(unownedPaths) > 0 {
		sort.Strings(unownedPaths)
		return templateMemberSnapshotValidation{}, fmt.Errorf(
			"template payload paths are not owned by any member snapshot: %s",
			strings.Join(unownedPaths, ", "),
		)
	}

	return result, nil
}

func sortedTemplateSnapshotVariableNames(values map[string]TemplateVariableSpec) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
