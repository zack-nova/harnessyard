package harness

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func buildTemplateMemberSnapshotFiles(
	candidates []TemplateMemberCandidate,
	finalFiles []orbittemplate.CandidateFile,
	runtimeBindings map[string]bindings.VariableBinding,
) ([]orbittemplate.CandidateFile, error) {
	if err := validateTemplateMemberSnapshotOwnership(candidates, finalFiles); err != nil {
		return nil, err
	}

	finalFilesByPath := make(map[string]orbittemplate.CandidateFile, len(finalFiles))
	for _, file := range finalFiles {
		finalFilesByPath[file.Path] = cloneCandidateFile(file)
	}

	snapshotFiles := make([]orbittemplate.CandidateFile, 0, len(candidates))
	for _, candidate := range sortedTemplateMemberCandidates(candidates) {
		snapshot, err := buildTemplateMemberSnapshot(candidate, finalFilesByPath, runtimeBindings)
		if err != nil {
			return nil, err
		}
		data, err := MarshalTemplateMemberSnapshot(snapshot)
		if err != nil {
			return nil, fmt.Errorf("marshal template member snapshot for %q: %w", candidate.OrbitID, err)
		}
		path, err := TemplateMemberSnapshotRepoPath(candidate.OrbitID)
		if err != nil {
			return nil, err
		}
		snapshotFiles = append(snapshotFiles, orbittemplate.CandidateFile{
			Path:    path,
			Content: data,
			Mode:    gitpkg.FileModeRegular,
		})
	}

	return snapshotFiles, nil
}

func validateTemplateMemberSnapshotOwnership(
	candidates []TemplateMemberCandidate,
	finalFiles []orbittemplate.CandidateFile,
) error {
	ownedPayloadPaths := make(map[string]struct{})
	for _, candidate := range candidates {
		hostedDefinitionPath, err := OrbitSpecRepoPath(candidate.OrbitID)
		if err != nil {
			return err
		}

		for _, file := range candidate.Files {
			switch {
			case file.Path == hostedDefinitionPath:
				continue
			case strings.HasPrefix(file.Path, ".harness/"):
				continue
			}

			ownedPayloadPaths[file.Path] = struct{}{}
		}
	}

	unownedPaths := make([]string, 0)
	for _, file := range finalFiles {
		switch {
		case file.Path == rootAgentsPath:
			continue
		case strings.HasPrefix(file.Path, ".harness/"):
			continue
		}

		if _, ok := ownedPayloadPaths[file.Path]; ok {
			continue
		}
		unownedPaths = append(unownedPaths, file.Path)
	}

	if len(unownedPaths) == 0 {
		return nil
	}

	sort.Strings(unownedPaths)
	return fmt.Errorf(
		"template member snapshots cannot attribute unowned payload paths after edit: %s",
		strings.Join(unownedPaths, ", "),
	)
}

func buildTemplateMemberSnapshot(
	candidate TemplateMemberCandidate,
	finalFilesByPath map[string]orbittemplate.CandidateFile,
	runtimeBindings map[string]bindings.VariableBinding,
) (TemplateMemberSnapshot, error) {
	memberFiles, err := finalTemplateMemberFiles(candidate, finalFilesByPath)
	if err != nil {
		return TemplateMemberSnapshot{}, err
	}

	variableSpecs, err := buildTemplateCandidateVariableSpecs(candidate.OrbitID, memberFiles, runtimeBindings)
	if err != nil {
		return TemplateMemberSnapshot{}, fmt.Errorf("build template member snapshot variables for %q: %w", candidate.OrbitID, err)
	}

	exportedPaths := make([]string, 0, len(memberFiles))
	fileDigests := make(map[string]string, len(memberFiles))
	for _, file := range memberFiles {
		exportedPaths = append(exportedPaths, file.Path)
		fileDigests[file.Path] = contentDigest(file.Content)
	}
	sort.Strings(exportedPaths)

	return TemplateMemberSnapshot{
		SchemaVersion: templateMemberSnapshotSchemaVersion,
		Kind:          TemplateMemberSnapshotKind,
		OrbitID:       candidate.OrbitID,
		MemberSource:  candidate.Source,
		Snapshot: TemplateMemberSnapshotData{
			ExportedPaths: exportedPaths,
			FileDigests:   fileDigests,
			Variables:     variableSpecs,
		},
	}, nil
}

func finalTemplateMemberFiles(
	candidate TemplateMemberCandidate,
	finalFilesByPath map[string]orbittemplate.CandidateFile,
) ([]orbittemplate.CandidateFile, error) {
	hostedDefinitionPath, err := OrbitSpecRepoPath(candidate.OrbitID)
	if err != nil {
		return nil, err
	}

	files := make([]orbittemplate.CandidateFile, 0, len(candidate.Files))
	seenPaths := make(map[string]struct{}, len(candidate.Files))
	for _, file := range candidate.Files {
		if file.Path == hostedDefinitionPath {
			continue
		}
		if _, ok := seenPaths[file.Path]; ok {
			continue
		}
		seenPaths[file.Path] = struct{}{}

		finalFile, ok := finalFilesByPath[file.Path]
		if !ok {
			continue
		}
		files = append(files, cloneCandidateFile(finalFile))
	}

	sort.Slice(files, func(left, right int) bool {
		return files[left].Path < files[right].Path
	})

	return files, nil
}

func sortedTemplateMemberCandidates(candidates []TemplateMemberCandidate) []TemplateMemberCandidate {
	sorted := append([]TemplateMemberCandidate(nil), candidates...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].OrbitID < sorted[right].OrbitID
	})
	return sorted
}
