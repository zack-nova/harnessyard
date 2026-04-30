package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	repoPrepareBootstrapPath = "BOOTSTRAP.md"
	repoPrepareAgentsPath    = "AGENTS.md"

	repoPrepareBootstrapBegin = "<!-- hyard:repo-bootstrap:begin -->"
	repoPrepareBootstrapEnd   = "<!-- hyard:repo-bootstrap:end -->"
	repoPrepareAgentsBegin    = "<!-- hyard:repo-agents:begin -->"
	repoPrepareAgentsEnd      = "<!-- hyard:repo-agents:end -->"

	repoPrepareBootstrapBody = "Review and update these repository-level preparation notes before follow-up work starts.\nKeep this block focused on setup that belongs to the repository, not to any specific harness or orbit."
	repoPrepareAgentsBody    = "Review BOOTSTRAP.md before starting work in this repository.\nFollow the repository-level preparation notes in the hyard repo bootstrap block when they apply."
)

// RepoPrepareGuidancePlan captures the repo-level guidance work hyard prepare can perform.
type RepoPrepareGuidancePlan struct {
	BootstrapPresent bool                          `json:"bootstrap_present"`
	Files            []RepoPrepareGuidanceFilePlan `json:"files"`
}

// RepoPrepareGuidanceFilePlan captures one repo-level guidance file action.
type RepoPrepareGuidanceFilePlan struct {
	Path   string `json:"path"`
	Action string `json:"action"`

	absolutePath string
	content      []byte
	mode         os.FileMode
}

// PlanRepoPrepareGuidance previews repo-level guidance blocks without writing files.
func PlanRepoPrepareGuidance(repoRoot string) (RepoPrepareGuidancePlan, error) {
	bootstrapPath := filepath.Join(repoRoot, repoPrepareBootstrapPath)
	bootstrapData, err := os.ReadFile(bootstrapPath) //nolint:gosec // Path is repo-local and fixed by the preparation contract.
	if errors.Is(err, os.ErrNotExist) {
		return RepoPrepareGuidancePlan{Files: []RepoPrepareGuidanceFilePlan{}}, nil
	}
	if err != nil {
		return RepoPrepareGuidancePlan{}, fmt.Errorf("read %s: %w", repoPrepareBootstrapPath, err)
	}
	if strings.TrimSpace(string(bootstrapData)) == "" {
		return RepoPrepareGuidancePlan{Files: []RepoPrepareGuidanceFilePlan{}}, nil
	}

	bootstrapPlan, err := planRepoPrepareGuidanceFile(repoRoot, repoPrepareBootstrapPath, repoPrepareBootstrapBegin, repoPrepareBootstrapEnd, repoPrepareBootstrapBody)
	if err != nil {
		return RepoPrepareGuidancePlan{}, err
	}
	agentsPlan, err := planRepoPrepareGuidanceFile(repoRoot, repoPrepareAgentsPath, repoPrepareAgentsBegin, repoPrepareAgentsEnd, repoPrepareAgentsBody)
	if err != nil {
		return RepoPrepareGuidancePlan{}, err
	}

	return RepoPrepareGuidancePlan{
		BootstrapPresent: true,
		Files:            []RepoPrepareGuidanceFilePlan{bootstrapPlan, agentsPlan},
	}, nil
}

// ApplyRepoPrepareGuidance applies repo-level guidance blocks when root BOOTSTRAP.md is present and non-empty.
func ApplyRepoPrepareGuidance(repoRoot string) (RepoPrepareGuidancePlan, error) {
	plan, err := PlanRepoPrepareGuidance(repoRoot)
	if err != nil {
		return RepoPrepareGuidancePlan{}, err
	}
	for index := range plan.Files {
		filePlan := &plan.Files[index]
		if filePlan.Action != "create" && filePlan.Action != "update" {
			continue
		}
		if err := contractutil.AtomicWriteFileMode(filePlan.absolutePath, filePlan.content, filePlan.mode); err != nil {
			return RepoPrepareGuidancePlan{}, fmt.Errorf("write %s: %w", filePlan.Path, err)
		}
	}

	return plan, nil
}

func planRepoPrepareGuidanceFile(repoRoot, repoPath, beginMarker, endMarker, body string) (RepoPrepareGuidanceFilePlan, error) {
	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	data, mode, exists, err := readRepoPrepareGuidanceFile(absolutePath)
	if err != nil {
		return RepoPrepareGuidanceFilePlan{}, fmt.Errorf("read %s: %w", repoPath, err)
	}

	nextData, changed, err := ensureRepoPrepareGuidanceBlock(data, beginMarker, endMarker, body)
	if err != nil {
		return RepoPrepareGuidanceFilePlan{}, fmt.Errorf("%s: %w", repoPath, err)
	}
	action := "unchanged"
	if changed {
		action = "update"
		if !exists {
			action = "create"
		}
	}

	return RepoPrepareGuidanceFilePlan{
		Path:         repoPath,
		Action:       action,
		absolutePath: absolutePath,
		content:      nextData,
		mode:         mode,
	}, nil
}

func readRepoPrepareGuidanceFile(filename string) ([]byte, os.FileMode, bool, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // Caller provides a repo-local fixed path.
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0o644, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("read repo prepare guidance file %s: %w", filename, err)
	}
	stat, err := os.Stat(filename)
	if err != nil {
		return nil, 0, false, fmt.Errorf("stat repo prepare guidance file %s: %w", filename, err)
	}

	return data, stat.Mode().Perm(), true, nil
}

func ensureRepoPrepareGuidanceBlock(data []byte, beginMarker, endMarker, body string) ([]byte, bool, error) {
	content := string(data)
	beginCount := strings.Count(content, beginMarker)
	endCount := strings.Count(content, endMarker)
	if beginCount == 1 && endCount == 1 {
		beginIndex := strings.Index(content, beginMarker)
		endIndex := strings.Index(content, endMarker)
		if beginIndex > endIndex {
			return nil, false, fmt.Errorf("end marker appears before begin marker")
		}

		return data, false, nil
	}
	if beginCount != 0 || endCount != 0 {
		return nil, false, fmt.Errorf("malformed hyard repo-level guidance block markers")
	}

	block := repoPrepareGuidanceBlock(beginMarker, endMarker, body)
	if len(data) == 0 {
		return []byte(block + "\n"), true, nil
	}

	separator := "\n\n"
	if strings.HasSuffix(content, "\n\n") {
		separator = ""
	} else if strings.HasSuffix(content, "\n") {
		separator = "\n"
	}

	return []byte(content + separator + block + "\n"), true, nil
}

func repoPrepareGuidanceBlock(beginMarker, endMarker, body string) string {
	return beginMarker + "\n" + strings.TrimSpace(body) + "\n" + endMarker
}
