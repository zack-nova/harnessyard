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

	repoPrepareBootstrapBody = "Complete the repository bootstrap tasks before normal work starts.\nRun `hyard bootstrap complete --check --json` to preview the closeout.\nRun `hyard bootstrap complete --yes` after confirming the preview only removes bootstrap guidance and bootstrap-lane runtime files."
	repoPrepareAgentsBody    = "Before starting normal work, read `BOOTSTRAP.md` if it exists.\nIf it contains hyard bootstrap instructions, complete that initialization flow first."
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

// RepoBootstrapCloseoutPlan captures repo-level bootstrap guidance cleanup.
type RepoBootstrapCloseoutPlan struct {
	RemovedBlocks        []string `json:"removed_blocks,omitempty"`
	RemovedPaths         []string `json:"removed_paths,omitempty"`
	DeletedBootstrapFile bool     `json:"deleted_bootstrap_file"`
	DeletedAgentsFile    bool     `json:"deleted_agents_file"`
}

// RepoBootstrapReopenPlan captures repo-level bootstrap guidance restoration.
type RepoBootstrapReopenPlan struct {
	RestoredBlocks       []string `json:"restored_blocks,omitempty"`
	RestoredPaths        []string `json:"restored_paths,omitempty"`
	CreatedBootstrapFile bool     `json:"created_bootstrap_file"`
	CreatedAgentsFile    bool     `json:"created_agents_file"`
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

// PlanRepoBootstrapCloseout previews cleanup of repo-level bootstrap guidance.
func PlanRepoBootstrapCloseout(repoRoot string) (RepoBootstrapCloseoutPlan, error) {
	var plan RepoBootstrapCloseoutPlan

	bootstrapPath := filepath.Join(repoRoot, repoPrepareBootstrapPath)
	bootstrapData, err := os.ReadFile(bootstrapPath) //nolint:gosec // Path is repo-local and fixed by the preparation contract.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return RepoBootstrapCloseoutPlan{}, fmt.Errorf("read %s: %w", repoPrepareBootstrapPath, err)
	}
	if err == nil {
		if hasRepoPrepareGuidanceBlock(bootstrapData, repoPrepareBootstrapBegin, repoPrepareBootstrapEnd) {
			plan.RemovedBlocks = append(plan.RemovedBlocks, "bootstrap")
		}
		plan.RemovedPaths = append(plan.RemovedPaths, repoPrepareBootstrapPath)
		plan.DeletedBootstrapFile = true
	}

	agentsPath := filepath.Join(repoRoot, repoPrepareAgentsPath)
	agentsData, err := os.ReadFile(agentsPath) //nolint:gosec // Path is repo-local and fixed by the preparation contract.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return RepoBootstrapCloseoutPlan{}, fmt.Errorf("read %s: %w", repoPrepareAgentsPath, err)
	}
	if err == nil {
		nextData, removed, err := removeRepoPrepareGuidanceBlock(agentsData, repoPrepareAgentsBegin, repoPrepareAgentsEnd)
		if err != nil {
			return RepoBootstrapCloseoutPlan{}, fmt.Errorf("%s: %w", repoPrepareAgentsPath, err)
		}
		if removed {
			plan.RemovedBlocks = append(plan.RemovedBlocks, "agents")
			plan.RemovedPaths = append(plan.RemovedPaths, repoPrepareAgentsPath)
			plan.DeletedAgentsFile = strings.TrimSpace(string(nextData)) == ""
		}
	}

	return plan, nil
}

// ApplyRepoBootstrapCloseout removes repo-level bootstrap guidance according to
// the repository bootstrap lifecycle contract.
func ApplyRepoBootstrapCloseout(repoRoot string) (RepoBootstrapCloseoutPlan, error) {
	plan, err := PlanRepoBootstrapCloseout(repoRoot)
	if err != nil {
		return RepoBootstrapCloseoutPlan{}, err
	}

	if plan.DeletedBootstrapFile {
		bootstrapPath := filepath.Join(repoRoot, repoPrepareBootstrapPath)
		if err := os.Remove(bootstrapPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return RepoBootstrapCloseoutPlan{}, fmt.Errorf("delete %s: %w", repoPrepareBootstrapPath, err)
		}
	}

	agentsPath := filepath.Join(repoRoot, repoPrepareAgentsPath)
	agentsData, mode, exists, err := readRepoPrepareGuidanceFile(agentsPath)
	if err != nil {
		return RepoBootstrapCloseoutPlan{}, fmt.Errorf("read %s: %w", repoPrepareAgentsPath, err)
	}
	if exists {
		nextData, removed, err := removeRepoPrepareGuidanceBlock(agentsData, repoPrepareAgentsBegin, repoPrepareAgentsEnd)
		if err != nil {
			return RepoBootstrapCloseoutPlan{}, fmt.Errorf("%s: %w", repoPrepareAgentsPath, err)
		}
		if removed {
			if strings.TrimSpace(string(nextData)) == "" {
				if err := os.Remove(agentsPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return RepoBootstrapCloseoutPlan{}, fmt.Errorf("delete %s: %w", repoPrepareAgentsPath, err)
				}
			} else if err := contractutil.AtomicWriteFileMode(agentsPath, nextData, mode); err != nil {
				return RepoBootstrapCloseoutPlan{}, fmt.Errorf("write %s: %w", repoPrepareAgentsPath, err)
			}
		}
	}

	return plan, nil
}

// PlanRepoBootstrapReopen previews restoration of repo-level bootstrap guidance blocks.
func PlanRepoBootstrapReopen(repoRoot string) (RepoBootstrapReopenPlan, error) {
	_, result, err := planRepoBootstrapReopenFiles(repoRoot)
	return result, err
}

// ApplyRepoBootstrapReopen restores repo-level bootstrap guidance blocks.
func ApplyRepoBootstrapReopen(repoRoot string) (RepoBootstrapReopenPlan, error) {
	filePlans, result, err := planRepoBootstrapReopenFiles(repoRoot)
	if err != nil {
		return RepoBootstrapReopenPlan{}, err
	}

	for _, filePlan := range filePlans {
		if filePlan.Action != "create" && filePlan.Action != "update" {
			continue
		}
		if err := contractutil.AtomicWriteFileMode(filePlan.absolutePath, filePlan.content, filePlan.mode); err != nil {
			return RepoBootstrapReopenPlan{}, fmt.Errorf("write %s: %w", filePlan.Path, err)
		}
	}

	return result, nil
}

func planRepoBootstrapReopenFiles(repoRoot string) ([]RepoPrepareGuidanceFilePlan, RepoBootstrapReopenPlan, error) {
	bootstrapPlan, err := planRepoPrepareGuidanceFile(repoRoot, repoPrepareBootstrapPath, repoPrepareBootstrapBegin, repoPrepareBootstrapEnd, repoPrepareBootstrapBody)
	if err != nil {
		return nil, RepoBootstrapReopenPlan{}, err
	}
	agentsPlan, err := planRepoPrepareGuidanceFile(repoRoot, repoPrepareAgentsPath, repoPrepareAgentsBegin, repoPrepareAgentsEnd, repoPrepareAgentsBody)
	if err != nil {
		return nil, RepoBootstrapReopenPlan{}, err
	}

	filePlans := []RepoPrepareGuidanceFilePlan{bootstrapPlan, agentsPlan}
	result := RepoBootstrapReopenPlan{}
	for _, filePlan := range filePlans {
		if filePlan.Action != "create" && filePlan.Action != "update" {
			continue
		}
		result.RestoredPaths = append(result.RestoredPaths, filePlan.Path)
		switch filePlan.Path {
		case repoPrepareBootstrapPath:
			result.RestoredBlocks = append(result.RestoredBlocks, "bootstrap")
			result.CreatedBootstrapFile = filePlan.Action == "create"
		case repoPrepareAgentsPath:
			result.RestoredBlocks = append(result.RestoredBlocks, "agents")
			result.CreatedAgentsFile = filePlan.Action == "create"
		}
	}

	return filePlans, result, nil
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
		return nil, 0, false, fmt.Errorf("read file: %w", err)
	}
	stat, err := os.Stat(filename)
	if err != nil {
		return nil, 0, false, fmt.Errorf("stat file: %w", err)
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

func hasRepoPrepareGuidanceBlock(data []byte, beginMarker, endMarker string) bool {
	content := string(data)
	return strings.Count(content, beginMarker) == 1 && strings.Count(content, endMarker) == 1
}

func removeRepoPrepareGuidanceBlock(data []byte, beginMarker, endMarker string) ([]byte, bool, error) {
	content := string(data)
	beginCount := strings.Count(content, beginMarker)
	endCount := strings.Count(content, endMarker)
	if beginCount == 0 && endCount == 0 {
		return data, false, nil
	}
	if beginCount != 1 || endCount != 1 {
		return nil, false, fmt.Errorf("malformed hyard repo-level guidance block markers")
	}

	beginIndex := strings.Index(content, beginMarker)
	endIndex := strings.Index(content, endMarker)
	if beginIndex > endIndex {
		return nil, false, fmt.Errorf("end marker appears before begin marker")
	}
	endIndex += len(endMarker)

	before := strings.TrimRight(content[:beginIndex], "\n")
	after := strings.TrimLeft(content[endIndex:], "\n")
	switch {
	case before == "":
		if after == "" {
			return []byte{}, true, nil
		}
		return []byte(after), true, nil
	case after == "":
		return []byte(before + "\n"), true, nil
	default:
		return []byte(before + "\n\n" + after), true, nil
	}
}
