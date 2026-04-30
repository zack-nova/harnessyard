package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type frameworkDetectionMode string

const (
	frameworkDetectionModeLocalHint frameworkDetectionMode = "local_hint"
	frameworkDetectionModeProject   frameworkDetectionMode = "project_detection"
)

// FrameworkAdapter is one built-in framework planning adapter.
type FrameworkAdapter struct {
	ID                    string
	ProjectAliasPath      string
	LocalHintPaths        []string
	ProjectDetectionPaths []string
	CommandsGlobal        bool
	SkillsGlobal          bool
	RemoteSkillsSupported bool
	ExecutableNames       []string
	RequiredEnvVars       []string
}

// RegisteredFrameworkAdapters returns the current built-in framework adapters in stable order.
func RegisteredFrameworkAdapters() []FrameworkAdapter {
	adapters := []FrameworkAdapter{
		{
			ID:                    "claudecode",
			ProjectAliasPath:      "CLAUDE.md",
			ProjectDetectionPaths: []string{"CLAUDE.md"},
			CommandsGlobal:        true,
			SkillsGlobal:          true,
			ExecutableNames:       []string{"claude"},
		},
		{
			ID:              "codex",
			ExecutableNames: []string{"codex"},
		},
		{
			ID:              "gitagent",
			LocalHintPaths:  []string{".gitagent_adapter"},
			ExecutableNames: []string{"gitagent"},
		},
		{
			ID:              "openclaw",
			ExecutableNames: []string{"openclaw"},
		},
	}

	sort.Slice(adapters, func(left, right int) bool {
		return adapters[left].ID < adapters[right].ID
	})

	return adapters
}

// NormalizeFrameworkID maps supported user aliases to canonical framework ids.
func NormalizeFrameworkID(frameworkID string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(frameworkID)) {
	case "claude", "claude_code", "claude-code", "claudecode":
		return "claudecode", true
	case "codex":
		return "codex", true
	case "gitagent":
		return "gitagent", true
	case "openclaw":
		return "openclaw", true
	default:
		return "", false
	}
}

// LookupFrameworkAdapter returns one built-in adapter by id.
func LookupFrameworkAdapter(frameworkID string) (FrameworkAdapter, bool) {
	normalizedFrameworkID, ok := NormalizeFrameworkID(frameworkID)
	if !ok {
		return FrameworkAdapter{}, false
	}
	for _, adapter := range RegisteredFrameworkAdapters() {
		if adapter.ID == normalizedFrameworkID {
			return adapter, true
		}
	}

	return FrameworkAdapter{}, false
}

func detectFrameworkLevel(repoRoot string, adapters []FrameworkAdapter, mode frameworkDetectionMode) ([]string, error) {
	matches := make([]string, 0)
	for _, adapter := range adapters {
		var detected bool
		switch mode {
		case frameworkDetectionModeLocalHint:
			var err error
			detected, err = frameworkPathDetected(repoRoot, adapter.LocalHintPaths)
			if err != nil {
				return nil, fmt.Errorf("detect local hint for framework %q: %w", adapter.ID, err)
			}
		case frameworkDetectionModeProject:
			var err error
			detected, err = frameworkPathDetected(repoRoot, adapter.ProjectDetectionPaths)
			if err != nil {
				return nil, fmt.Errorf("detect project files for framework %q: %w", adapter.ID, err)
			}
		default:
			continue
		}
		if detected {
			matches = append(matches, adapter.ID)
		}
	}
	sort.Strings(matches)

	return matches, nil
}

func frameworkPathDetected(repoRoot string, repoPaths []string) (bool, error) {
	for _, repoPath := range repoPaths {
		if repoPath == "" {
			continue
		}
		filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
		_, err := os.Stat(filename)
		if err == nil {
			return true, nil
		}
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("stat %s: %w", repoPath, err)
		}
	}

	return false, nil
}
