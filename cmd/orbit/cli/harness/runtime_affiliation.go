package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// AssignRuntimeMemberResult captures one successful runtime affiliation assign.
type AssignRuntimeMemberResult struct {
	ManifestPath           string
	Runtime                RuntimeFile
	HarnessID              string
	Source                 string
	PreviousOwnerHarnessID string
	Changed                bool
}

// UnassignRuntimeMemberResult captures one successful runtime affiliation unassign.
type UnassignRuntimeMemberResult struct {
	ManifestPath           string
	Runtime                RuntimeFile
	SourceBefore           string
	SourceAfter            string
	PreviousOwnerHarnessID string
	Changed                bool
	RemovedPaths           []string
	RemovedAgentsBlock     bool
	DeletedBundleRecord    bool
}

// AssignRuntimeMember assigns one active runtime member to an existing harness id.
func AssignRuntimeMember(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	harnessID string,
	now time.Time,
) (AssignRuntimeMemberResult, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return AssignRuntimeMemberResult{}, fmt.Errorf("load harness runtime: %w", err)
	}

	targetIndex, targetMember, found := findRuntimeMemberIndex(runtimeFile, orbitID)
	if !found {
		return AssignRuntimeMemberResult{}, fmt.Errorf("member %q not found", orbitID)
	}

	trimmedHarnessID := strings.TrimSpace(harnessID)
	if trimmedHarnessID == "" {
		return AssignRuntimeMemberResult{}, fmt.Errorf("harness id must not be empty")
	}
	if err := ids.ValidateOrbitID(trimmedHarnessID); err != nil {
		return AssignRuntimeMemberResult{}, fmt.Errorf("harness id: %w", err)
	}

	if targetMember.OwnerHarnessID != "" {
		if targetMember.OwnerHarnessID == trimmedHarnessID {
			return AssignRuntimeMemberResult{
				Runtime:                runtimeFile,
				HarnessID:              trimmedHarnessID,
				Source:                 targetMember.Source,
				PreviousOwnerHarnessID: targetMember.OwnerHarnessID,
				Changed:                false,
			}, nil
		}
		return AssignRuntimeMemberResult{}, fmt.Errorf(
			"orbit %q is currently assigned to harness %q; unassign it before assigning to harness %q",
			orbitID,
			targetMember.OwnerHarnessID,
			trimmedHarnessID,
		)
	}

	if err := ensureRuntimeHarnessExists(ctx, repoRoot, runtimeFile, trimmedHarnessID); err != nil {
		return AssignRuntimeMemberResult{}, err
	}

	runtimeFile.Members[targetIndex].OwnerHarnessID = trimmedHarnessID
	runtimeFile.Harness.UpdatedAt = resolveMutationTime(now)

	manifestPath, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return AssignRuntimeMemberResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return AssignRuntimeMemberResult{
		ManifestPath:           manifestPath,
		Runtime:                runtimeFile,
		HarnessID:              trimmedHarnessID,
		Source:                 targetMember.Source,
		PreviousOwnerHarnessID: "",
		Changed:                true,
	}, nil
}

// UnassignRuntimeMember removes the current harness affiliation from one active runtime member.
func UnassignRuntimeMember(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	now time.Time,
) (UnassignRuntimeMemberResult, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return UnassignRuntimeMemberResult{}, fmt.Errorf("load harness runtime: %w", err)
	}

	targetIndex, targetMember, found := findRuntimeMemberIndex(runtimeFile, orbitID)
	if !found {
		return UnassignRuntimeMemberResult{}, fmt.Errorf("member %q not found", orbitID)
	}
	if targetMember.OwnerHarnessID == "" {
		return UnassignRuntimeMemberResult{}, fmt.Errorf("orbit %q is already standalone", orbitID)
	}

	if targetMember.Source == MemberSourceInstallBundle {
		result, err := ExtractRuntimeMemberDetached(ctx, repoRoot, orbitID, now)
		if err != nil {
			return UnassignRuntimeMemberResult{}, err
		}
		nextMember, found := findRuntimeMember(result.Runtime, orbitID)
		if !found {
			return UnassignRuntimeMemberResult{}, fmt.Errorf("extracted runtime member %q is missing from runtime", orbitID)
		}

		return UnassignRuntimeMemberResult{
			ManifestPath:           result.ManifestPath,
			Runtime:                result.Runtime,
			SourceBefore:           targetMember.Source,
			SourceAfter:            nextMember.Source,
			PreviousOwnerHarnessID: targetMember.OwnerHarnessID,
			Changed:                true,
			RemovedPaths:           result.RemovedPaths,
			RemovedAgentsBlock:     result.RemovedAgentsBlock,
			DeletedBundleRecord:    result.DeletedBundleRecord,
		}, nil
	}

	runtimeFile.Members[targetIndex].OwnerHarnessID = ""
	runtimeFile.Harness.UpdatedAt = resolveMutationTime(now)

	manifestPath, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return UnassignRuntimeMemberResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return UnassignRuntimeMemberResult{
		ManifestPath:           manifestPath,
		Runtime:                runtimeFile,
		SourceBefore:           targetMember.Source,
		SourceAfter:            runtimeFile.Members[targetIndex].Source,
		PreviousOwnerHarnessID: targetMember.OwnerHarnessID,
		Changed:                true,
	}, nil
}

func ensureRuntimeHarnessExists(ctx context.Context, repoRoot string, runtimeFile RuntimeFile, harnessID string) error {
	if runtimeFile.Harness.ID == harnessID {
		return nil
	}

	for _, member := range runtimeFile.Members {
		if member.OwnerHarnessID == harnessID {
			return nil
		}
	}

	record, err := LoadBundleRecord(repoRoot, harnessID)
	if err == nil {
		if record.HarnessID == harnessID {
			return nil
		}
		return fmt.Errorf("bundle record %q does not match requested harness id", harnessID)
	}
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"harness %q is not active in the current runtime and has no bundle record to bind against",
			harnessID,
		)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("check command context: %w", ctx.Err())
	}
	return fmt.Errorf("load bundle record for %q: %w", harnessID, err)
}

func findRuntimeMemberIndex(runtimeFile RuntimeFile, orbitID string) (int, RuntimeMember, bool) {
	for index, member := range runtimeFile.Members {
		if member.OrbitID == orbitID {
			return index, member, true
		}
	}

	return -1, RuntimeMember{}, false
}
