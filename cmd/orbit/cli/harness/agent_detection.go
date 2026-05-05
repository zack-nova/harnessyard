package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const agentDetectionSchemaVersion = "1.0"
const agentDetectionSignatureVersion = "2026-04-25.2"
const agentDetectionCommandTimeout = 10 * time.Second

// AgentDetectionStatus describes the classified local detection state for one agent component.
type AgentDetectionStatus string

const (
	AgentDetectionStatusNotFound            AgentDetectionStatus = "not_found"
	AgentDetectionStatusFootprintOnly       AgentDetectionStatus = "footprint_only"
	AgentDetectionStatusInstalledUnverified AgentDetectionStatus = "installed_unverified"
	AgentDetectionStatusInstalledCLI        AgentDetectionStatus = "installed_cli"
	AgentDetectionStatusInstalledDesktop    AgentDetectionStatus = "installed_desktop"
	AgentDetectionStatusConfigured          AgentDetectionStatus = "configured"
	AgentDetectionStatusRunning             AgentDetectionStatus = "running"
	AgentDetectionStatusStaleOrRemoved      AgentDetectionStatus = "stale_or_removed"
	AgentDetectionStatusAmbiguous           AgentDetectionStatus = "ambiguous"
)

// AgentDetectionInput captures one local agent detection request.
type AgentDetectionInput struct {
	RepoRoot string
	GitDir   string
	Deep     bool
	Refresh  bool
	NoCache  bool
}

// AgentDetectionHost describes the host environment that was inspected.
type AgentDetectionHost struct {
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	EnvironmentID string `json:"environment_id"`
}

// AgentDetectionReport is the top-level JSON contract for hyard agent detect.
type AgentDetectionReport struct {
	SchemaVersion         string                 `json:"schema_version"`
	Host                  AgentDetectionHost     `json:"host"`
	RuntimeRecommendation string                 `json:"runtime_recommendation,omitempty"`
	LocalSelection        string                 `json:"local_selection,omitempty"`
	Tools                 []AgentToolDetection   `json:"tools"`
	SuggestedActions      []AgentSuggestedAction `json:"suggested_actions,omitempty"`
	Warnings              []string               `json:"warnings,omitempty"`
}

// AgentToolDetection captures all detected components for one supported agent.
type AgentToolDetection struct {
	Agent      string                    `json:"agent"`
	Components []AgentComponentDetection `json:"components"`
	Summary    AgentDetectionSummary     `json:"summary"`
}

// AgentDetectionSummary is the per-agent rollup used by human output and suggestions.
type AgentDetectionSummary struct {
	Status     AgentDetectionStatus `json:"status"`
	Confidence float64              `json:"confidence"`
	Ready      bool                 `json:"ready"`
}

// AgentComponentDetection captures one detected component such as cli, state, or project footprint.
type AgentComponentDetection struct {
	Component  string                   `json:"component"`
	Status     AgentDetectionStatus     `json:"status"`
	Confidence float64                  `json:"confidence"`
	Version    string                   `json:"version,omitempty"`
	Evidence   []AgentDetectionEvidence `json:"evidence,omitempty"`
	Warnings   []string                 `json:"warnings,omitempty"`
}

// AgentDetectionEvidence records one privacy-filtered signal.
type AgentDetectionEvidence struct {
	Kind         string            `json:"kind"`
	Source       string            `json:"source,omitempty"`
	Path         string            `json:"path,omitempty"`
	Version      string            `json:"version,omitempty"`
	Score        int               `json:"score,omitempty"`
	OwnedByOrbit bool              `json:"owned_by_orbit,omitempty"`
	ContentRead  bool              `json:"content_read"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// AgentSuggestedAction is a safe next command the user may run explicitly.
type AgentSuggestedAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

type agentSignature struct {
	ID                    string
	ExecutableNames       []string
	NativeExecutablePaths []string
	VersionArgSets        [][]string
	Packages              []agentPackageSignature
	DesktopAppNames       []string
	DesktopBundleIDHints  []string
	GatewaySystemdUnits   []string
	GatewayLaunchdLabels  []string
	GatewayScheduledTasks []string
	StateDirEnv           string
	StateDirDefault       string
	ProjectFootprintPaths []string
}

type agentPackageSignature struct {
	Manager string
	Name    string
}

type agentDetectionCacheFile struct {
	SchemaVersion string               `json:"schema_version"`
	CacheKey      string               `json:"cache_key"`
	Tools         []AgentToolDetection `json:"tools"`
}

// SupportedAgentIDs returns the canonical supported agent ids in stable order.
func SupportedAgentIDs() []string {
	return []string{"claudecode", "codex", "openclaw"}
}

// NormalizeAgentID maps supported aliases to canonical agent ids.
func NormalizeAgentID(agentID string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "claude", "claude_code", "claude-code", "claudecode":
		return "claudecode", true
	case "codex":
		return "codex", true
	case "openclaw":
		return "openclaw", true
	default:
		return "", false
	}
}

// DetectAgents inspects the current machine for supported agents without changing selection, guidance, or target-agent state.
func DetectAgents(ctx context.Context, input AgentDetectionInput) (AgentDetectionReport, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	report := AgentDetectionReport{
		SchemaVersion: agentDetectionSchemaVersion,
		Host: AgentDetectionHost{
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			EnvironmentID: "native",
		},
		Tools: []AgentToolDetection{},
	}

	frameworksFile, err := LoadOptionalFrameworksFile(input.RepoRoot)
	if err != nil {
		return AgentDetectionReport{}, fmt.Errorf("load agent recommendation truth: %w", err)
	}
	if frameworksFile.RecommendedFramework != "" {
		if normalized, ok := NormalizeAgentID(frameworksFile.RecommendedFramework); ok {
			report.RuntimeRecommendation = normalized
		} else {
			report.RuntimeRecommendation = frameworksFile.RecommendedFramework
			report.Warnings = append(report.Warnings, fmt.Sprintf(`legacy or unsupported recommended agent %q is ignored by detection`, frameworksFile.RecommendedFramework))
		}
	}

	if selection, err := LoadFrameworkSelection(input.GitDir); err == nil {
		if normalized, ok := NormalizeAgentID(selection.SelectedFramework); ok {
			report.LocalSelection = normalized
		} else {
			report.LocalSelection = selection.SelectedFramework
			report.Warnings = append(report.Warnings, fmt.Sprintf(`legacy or unsupported selected agent %q is ignored by detection`, selection.SelectedFramework))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return AgentDetectionReport{}, fmt.Errorf("load local agent selection: %w", err)
	}

	cacheKey := agentDetectionCacheKey(input, homeDir)
	if !input.Refresh && !input.NoCache {
		if cachedTools, ok := loadAgentDetectionCache(input.GitDir, cacheKey); ok {
			report.Tools = cachedTools
			finalizeAgentDetectionReport(&report)
			return report, nil
		}
	}

	for _, signature := range agentSignatures(homeDir) {
		tool := detectAgentTool(ctx, input.RepoRoot, homeDir, signature, input.Deep)
		report.Tools = append(report.Tools, tool)
	}
	finalizeAgentDetectionReport(&report)
	if !input.NoCache {
		storeAgentDetectionCache(input.GitDir, cacheKey, report.Tools)
	}

	return report, nil
}

func finalizeAgentDetectionReport(report *AgentDetectionReport) {
	sort.Slice(report.Tools, func(left, right int) bool {
		return report.Tools[left].Agent < report.Tools[right].Agent
	})

	readyAgents := make([]string, 0)
	for _, tool := range report.Tools {
		if tool.Summary.Ready {
			readyAgents = append(readyAgents, tool.Agent)
		}
	}
	sort.Strings(readyAgents)
	switch len(readyAgents) {
	case 0:
	case 1:
		report.SuggestedActions = append(report.SuggestedActions, AgentSuggestedAction{
			Command: "hyard agent use " + readyAgents[0],
			Reason:  readyAgents[0] + " is the only ready detected agent",
		})
	default:
		report.Warnings = append(report.Warnings, "multiple ready agents detected: "+strings.Join(readyAgents, ", "))
	}
	sort.Strings(report.Warnings)
}

func agentDetectionCacheKey(input AgentDetectionInput, homeDir string) string {
	cacheMaterial := strings.Join([]string{
		"signature=" + agentDetectionSignatureVersion,
		"schema=" + agentDetectionSchemaVersion,
		"os=" + runtime.GOOS,
		"arch=" + runtime.GOARCH,
		"env=native",
		"deep=" + fmt.Sprintf("%t", input.Deep),
		"path=" + os.Getenv("PATH"),
		"home=" + filepath.Clean(homeDir),
	}, "\n")
	sum := sha256.Sum256([]byte(cacheMaterial))

	return hex.EncodeToString(sum[:])
}

func loadAgentDetectionCache(gitDir string, cacheKey string) ([]AgentToolDetection, bool) {
	cachePath := agentDetectionCachePath(gitDir)
	if cachePath == "" {
		return nil, false
	}
	//nolint:gosec // Cache path is derived from the resolved repo git dir and fixed state suffix.
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}
	var cache agentDetectionCacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if cache.SchemaVersion != agentDetectionSchemaVersion || cache.CacheKey != cacheKey {
		return nil, false
	}

	return cache.Tools, true
}

func storeAgentDetectionCache(gitDir string, cacheKey string, tools []AgentToolDetection) {
	cachePath := agentDetectionCachePath(gitDir)
	if cachePath == "" {
		return
	}
	data, err := json.MarshalIndent(agentDetectionCacheFile{
		SchemaVersion: agentDetectionSchemaVersion,
		CacheKey:      cacheKey,
		Tools:         tools,
	}, "", "  ")
	if err != nil {
		return
	}
	if err := contractutil.AtomicWriteFile(cachePath, append(data, '\n')); err != nil {
		return
	}
}

func agentDetectionCachePath(gitDir string) string {
	if strings.TrimSpace(gitDir) == "" {
		return ""
	}

	return filepath.Join(gitDir, "orbit", "state", "agents", "detection-cache.json")
}

func agentSignatures(homeDir string) []agentSignature {
	return []agentSignature{
		{
			ID:                    "claudecode",
			ExecutableNames:       []string{"claude"},
			NativeExecutablePaths: []string{filepath.Join(homeDir, ".local", "bin", "claude")},
			VersionArgSets:        [][]string{{"-v"}, {"--version"}},
			Packages: []agentPackageSignature{
				{Manager: "npm", Name: "@anthropic-ai/claude-code"},
				{Manager: "homebrew", Name: "claude-code"},
				{Manager: "dpkg", Name: "claude-code"},
				{Manager: "rpm", Name: "claude-code"},
				{Manager: "apk", Name: "claude-code"},
			},
			DesktopAppNames:       []string{"Claude.app"},
			DesktopBundleIDHints:  []string{"com.anthropic.Claude"},
			StateDirEnv:           "CLAUDE_CONFIG_DIR",
			StateDirDefault:       filepath.Join(homeDir, ".claude"),
			ProjectFootprintPaths: []string{"CLAUDE.md", "CLAUDE.local.md", ".claude/settings.json", ".claude/commands", ".claude/skills"},
		},
		{
			ID:              "codex",
			ExecutableNames: []string{"codex"},
			VersionArgSets:  [][]string{{"--version"}},
			Packages: []agentPackageSignature{
				{Manager: "npm", Name: "@openai/codex"},
				{Manager: "homebrew", Name: "codex"},
			},
			DesktopAppNames:      []string{"Codex.app"},
			DesktopBundleIDHints: []string{"com.openai.codex"},
			StateDirEnv:          "CODEX_HOME",
			StateDirDefault:      filepath.Join(homeDir, ".codex"),
		},
		{
			ID:              "openclaw",
			ExecutableNames: []string{"openclaw"},
			NativeExecutablePaths: []string{
				filepath.Join(homeDir, ".openclaw", "bin", "openclaw"),
			},
			VersionArgSets: [][]string{{"--version"}, {"-v"}},
			Packages: []agentPackageSignature{
				{Manager: "npm", Name: "openclaw"},
				{Manager: "pnpm", Name: "openclaw"},
				{Manager: "bun", Name: "openclaw"},
				{Manager: "homebrew", Name: "openclaw"},
				{Manager: "dpkg", Name: "openclaw"},
				{Manager: "rpm", Name: "openclaw"},
				{Manager: "apk", Name: "openclaw"},
			},
			DesktopAppNames:       []string{"OpenClaw.app"},
			GatewaySystemdUnits:   []string{"openclaw-gateway.service"},
			GatewayLaunchdLabels:  []string{"ai.openclaw.gateway"},
			GatewayScheduledTasks: []string{"OpenClaw Gateway"},
			StateDirEnv:           "OPENCLAW_STATE_DIR",
			StateDirDefault:       filepath.Join(homeDir, ".openclaw"),
		},
	}
}

func detectAgentTool(ctx context.Context, repoRoot string, homeDir string, signature agentSignature, deep bool) AgentToolDetection {
	components := []AgentComponentDetection{
		detectAgentCLI(ctx, homeDir, signature),
	}
	if deep {
		components = append(components, detectAgentPackage(ctx, signature))
		components = append(components, detectAgentDesktop(homeDir, signature))
		components = append(components, detectAgentGateway(ctx, signature))
	}
	components = append(components,
		detectAgentStateDir(homeDir, signature),
		detectAgentProjectFootprint(repoRoot, signature),
	)

	return AgentToolDetection{
		Agent:      signature.ID,
		Components: components,
		Summary:    summarizeAgentComponents(components),
	}
}

func detectAgentPackage(ctx context.Context, signature agentSignature) AgentComponentDetection {
	if len(signature.Packages) == 0 {
		return AgentComponentDetection{Component: "package", Status: AgentDetectionStatusNotFound}
	}

	for _, packageSignature := range signature.Packages {
		version, source, err := detectPackageManagerVersion(ctx, packageSignature)
		if err != nil {
			continue
		}

		return AgentComponentDetection{
			Component:  "package",
			Status:     AgentDetectionStatusInstalledUnverified,
			Confidence: 0.60,
			Version:    version,
			Evidence: []AgentDetectionEvidence{
				{
					Kind:        "package",
					Source:      source,
					Version:     version,
					Score:       35,
					ContentRead: false,
					Metadata: map[string]string{
						"manager": packageSignature.Manager,
						"package": packageSignature.Name,
					},
				},
			},
		}
	}

	return AgentComponentDetection{Component: "package", Status: AgentDetectionStatusNotFound}
}

func detectAgentDesktop(homeDir string, signature agentSignature) AgentComponentDetection {
	if len(signature.DesktopAppNames) == 0 {
		return AgentComponentDetection{Component: "desktop", Status: AgentDetectionStatusNotFound}
	}

	for _, root := range desktopAppRoots(homeDir) {
		for _, appName := range signature.DesktopAppNames {
			appPath := filepath.Join(root, appName)
			info, err := os.Stat(appPath)
			if err != nil || !info.IsDir() {
				continue
			}

			metadata := map[string]string{}
			infoPath := filepath.Join(appPath, "Contents", "Info.plist")
			if bundleID, err := readPlistString(infoPath, "CFBundleIdentifier"); err == nil && bundleID != "" {
				metadata["bundle_id"] = bundleID
			}
			version := ""
			if parsedVersion, err := readPlistString(infoPath, "CFBundleShortVersionString"); err == nil {
				version = parsedVersion
			}

			confidence := 0.65
			if bundleIDMatches(metadata["bundle_id"], signature.DesktopBundleIDHints) {
				confidence = 0.80
			}
			return AgentComponentDetection{
				Component:  "desktop",
				Status:     AgentDetectionStatusInstalledDesktop,
				Confidence: confidence,
				Version:    version,
				Evidence: []AgentDetectionEvidence{
					{
						Kind:        "app",
						Source:      "app_bundle",
						Path:        redactHome(homeDir, appPath),
						Version:     version,
						Score:       45,
						ContentRead: false,
						Metadata:    metadata,
					},
				},
			}
		}
	}

	return AgentComponentDetection{Component: "desktop", Status: AgentDetectionStatusNotFound}
}

func desktopAppRoots(homeDir string) []string {
	if strings.TrimSpace(homeDir) == "" {
		return []string{}
	}

	return []string{filepath.Join(homeDir, "Applications")}
}

func bundleIDMatches(bundleID string, hints []string) bool {
	if bundleID == "" {
		return false
	}
	for _, hint := range hints {
		if strings.EqualFold(bundleID, hint) {
			return true
		}
	}

	return false
}

func detectAgentGateway(ctx context.Context, signature agentSignature) AgentComponentDetection {
	for _, unit := range signature.GatewaySystemdUnits {
		if gatewayServiceActive(ctx, "systemctl", []string{"--user", "is-active", unit}, "active") {
			return AgentComponentDetection{
				Component:  "gateway",
				Status:     AgentDetectionStatusRunning,
				Confidence: 0.70,
				Evidence: []AgentDetectionEvidence{
					{
						Kind:        "service",
						Source:      "systemd",
						Score:       35,
						ContentRead: false,
						Metadata: map[string]string{
							"unit": unit,
						},
					},
				},
			}
		}
	}
	for _, label := range signature.GatewayLaunchdLabels {
		if gatewayServiceActive(ctx, "launchctl", []string{"print", "gui/" + fmt.Sprintf("%d", os.Getuid()) + "/" + label}, "") {
			return AgentComponentDetection{
				Component:  "gateway",
				Status:     AgentDetectionStatusRunning,
				Confidence: 0.70,
				Evidence: []AgentDetectionEvidence{
					{
						Kind:        "service",
						Source:      "launchd",
						Score:       35,
						ContentRead: false,
						Metadata: map[string]string{
							"label": label,
						},
					},
				},
			}
		}
	}
	for _, task := range signature.GatewayScheduledTasks {
		if gatewayServiceActive(ctx, "schtasks", []string{"/Query", "/TN", task, "/FO", "LIST", "/V"}, "") {
			return AgentComponentDetection{
				Component:  "gateway",
				Status:     AgentDetectionStatusRunning,
				Confidence: 0.70,
				Evidence: []AgentDetectionEvidence{
					{
						Kind:        "service",
						Source:      "scheduled_task",
						Score:       35,
						ContentRead: false,
						Metadata: map[string]string{
							"task": task,
						},
					},
				},
			}
		}
	}

	return AgentComponentDetection{Component: "gateway", Status: AgentDetectionStatusNotFound}
}

func gatewayServiceActive(ctx context.Context, executableName string, args []string, expectedOutput string) bool {
	candidates := findExecutableCandidates([]string{executableName})
	if len(candidates) == 0 {
		return false
	}
	output, err := runAgentDetectionCommand(ctx, candidates[0], args)
	if err != nil {
		return false
	}
	if expectedOutput == "" {
		return true
	}

	return strings.EqualFold(firstNonEmptyLine(output), expectedOutput)
}

func detectAgentCLI(ctx context.Context, homeDir string, signature agentSignature) AgentComponentDetection {
	candidates := findAgentExecutableCandidates(signature)
	if len(candidates) == 0 {
		return AgentComponentDetection{
			Component:  "cli",
			Status:     AgentDetectionStatusNotFound,
			Confidence: 0,
		}
	}

	evidence := make([]AgentDetectionEvidence, 0, len(candidates)+1)
	for _, candidate := range candidates {
		evidence = append(evidence, AgentDetectionEvidence{
			Kind:        "executable",
			Source:      "path",
			Path:        redactHome(homeDir, candidate),
			Score:       25,
			ContentRead: false,
		})
	}

	version, versionErr := verifyAgentVersion(ctx, candidates[0], signature.VersionArgSets)
	if versionErr != nil {
		return AgentComponentDetection{
			Component:  "cli",
			Status:     AgentDetectionStatusInstalledUnverified,
			Confidence: 0.45,
			Evidence:   evidence,
			Warnings:   []string{versionErr.Error()},
		}
	}
	evidence = append(evidence, AgentDetectionEvidence{
		Kind:        "version",
		Source:      "version_command",
		Path:        redactHome(homeDir, candidates[0]),
		Version:     version,
		Score:       25,
		ContentRead: false,
	})
	if !agentVersionMatches(signature.ID, version) {
		return AgentComponentDetection{
			Component:  "cli",
			Status:     AgentDetectionStatusAmbiguous,
			Confidence: 0.45,
			Version:    version,
			Evidence:   evidence,
			Warnings:   []string{fmt.Sprintf("version output for %s did not match expected agent identity", signature.ID)},
		}
	}

	return AgentComponentDetection{
		Component:  "cli",
		Status:     AgentDetectionStatusInstalledCLI,
		Confidence: 0.70,
		Version:    version,
		Evidence:   evidence,
	}
}

func findAgentExecutableCandidates(signature agentSignature) []string {
	seen := map[string]struct{}{}
	candidates := []string{}
	for _, candidate := range findExecutableCandidates(signature.ExecutableNames) {
		clean := filepath.Clean(candidate)
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
	}
	for _, nativePath := range signature.NativeExecutablePaths {
		if !isExecutableFile(nativePath) {
			continue
		}
		clean := filepath.Clean(nativePath)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
	}
	sort.Strings(candidates)

	return candidates
}

func detectAgentStateDir(homeDir string, signature agentSignature) AgentComponentDetection {
	stateDir := strings.TrimSpace(os.Getenv(signature.StateDirEnv))
	if stateDir == "" {
		stateDir = signature.StateDirDefault
	}
	if stateDir == "" {
		return AgentComponentDetection{Component: "state", Status: AgentDetectionStatusNotFound}
	}
	//nolint:gosec // State directory comes from documented agent env/default paths and only metadata is inspected.
	info, err := os.Stat(stateDir)
	if err != nil {
		return AgentComponentDetection{Component: "state", Status: AgentDetectionStatusNotFound}
	}
	if !info.IsDir() {
		return AgentComponentDetection{Component: "state", Status: AgentDetectionStatusAmbiguous, Confidence: 0.20}
	}

	return AgentComponentDetection{
		Component:  "state",
		Status:     AgentDetectionStatusConfigured,
		Confidence: 0.35,
		Evidence: []AgentDetectionEvidence{
			{
				Kind:        "state_dir",
				Source:      "filesystem",
				Path:        redactHome(homeDir, stateDir),
				Score:       10,
				ContentRead: false,
			},
		},
	}
}

func detectAgentProjectFootprint(repoRoot string, signature agentSignature) AgentComponentDetection {
	if len(signature.ProjectFootprintPaths) == 0 {
		return AgentComponentDetection{Component: "project_footprint", Status: AgentDetectionStatusNotFound}
	}

	evidence := []AgentDetectionEvidence{}
	for _, repoPath := range signature.ProjectFootprintPaths {
		absolutePath := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
		if _, err := os.Lstat(absolutePath); err != nil {
			continue
		}
		evidence = append(evidence, AgentDetectionEvidence{
			Kind:         "project_footprint",
			Source:       "filesystem",
			Path:         filepath.ToSlash(repoPath),
			Score:        5,
			OwnedByOrbit: orbitOwnedAgentFootprint(repoRoot, absolutePath),
			ContentRead:  false,
		})
	}
	if len(evidence) == 0 {
		return AgentComponentDetection{Component: "project_footprint", Status: AgentDetectionStatusNotFound}
	}

	return AgentComponentDetection{
		Component:  "project_footprint",
		Status:     AgentDetectionStatusFootprintOnly,
		Confidence: 0.20,
		Evidence:   evidence,
	}
}

func summarizeAgentComponents(components []AgentComponentDetection) AgentDetectionSummary {
	summary := AgentDetectionSummary{Status: AgentDetectionStatusNotFound}
	for _, component := range components {
		if component.Confidence > summary.Confidence {
			summary.Confidence = component.Confidence
		}
		switch component.Status {
		case AgentDetectionStatusAmbiguous:
			return AgentDetectionSummary{Status: AgentDetectionStatusAmbiguous, Confidence: component.Confidence}
		case AgentDetectionStatusInstalledCLI,
			AgentDetectionStatusInstalledDesktop,
			AgentDetectionStatusRunning:
			return AgentDetectionSummary{Status: component.Status, Confidence: component.Confidence, Ready: true}
		case AgentDetectionStatusInstalledUnverified:
			if summary.Status == AgentDetectionStatusNotFound ||
				summary.Status == AgentDetectionStatusFootprintOnly {
				summary.Status = AgentDetectionStatusInstalledUnverified
			}
		case AgentDetectionStatusConfigured,
			AgentDetectionStatusFootprintOnly,
			AgentDetectionStatusStaleOrRemoved:
			if summary.Status == AgentDetectionStatusNotFound {
				summary.Status = AgentDetectionStatusFootprintOnly
			}
		}
	}

	return summary
}

func findExecutableCandidates(names []string) []string {
	pathValue := os.Getenv("PATH")
	if pathValue == "" {
		return []string{}
	}
	seen := map[string]struct{}{}
	candidates := []string{}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			continue
		}
		for _, name := range names {
			for _, candidate := range executableNameCandidates(name) {
				absolutePath := filepath.Join(dir, candidate)
				if !isExecutableFile(absolutePath) {
					continue
				}
				clean := filepath.Clean(absolutePath)
				if _, ok := seen[clean]; ok {
					continue
				}
				seen[clean] = struct{}{}
				candidates = append(candidates, clean)
			}
		}
	}
	sort.Strings(candidates)

	return candidates
}

func executableNameCandidates(name string) []string {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		return []string{name + ".exe", name + ".cmd", name + ".bat", name}
	}

	return []string{name}
}

func isExecutableFile(path string) bool {
	//nolint:gosec // Candidate executable paths come from PATH/native signatures and only metadata is inspected.
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}

	return info.Mode().Perm()&0o111 != 0
}

func verifyAgentVersion(ctx context.Context, executablePath string, argSets [][]string) (string, error) {
	var lastErr error
	for _, args := range argSets {
		version, err := runAgentVersionCommand(ctx, executablePath, args)
		if err == nil {
			return version, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no version arguments configured")
	}

	return "", lastErr
}

func runAgentVersionCommand(ctx context.Context, executablePath string, args []string) (string, error) {
	output, err := runAgentDetectionCommand(ctx, executablePath, args)
	if err != nil {
		return "", err
	}
	version := firstNonEmptyLine(output)
	if version == "" {
		return "", fmt.Errorf("version check returned empty output for %s", filepath.Base(executablePath))
	}

	return version, nil
}

func detectPackageManagerVersion(ctx context.Context, packageSignature agentPackageSignature) (string, string, error) {
	switch packageSignature.Manager {
	case "npm":
		version, err := detectNPMGlobalPackageVersion(ctx, packageSignature.Name)
		return version, "npm_global", err
	case "pnpm":
		version, err := detectPNPMGlobalPackageVersion(ctx, packageSignature.Name)
		return version, "pnpm_global", err
	case "bun":
		version, err := detectBunGlobalPackageVersion(ctx, packageSignature.Name)
		return version, "bun_global", err
	case "homebrew":
		version, err := detectHomebrewPackageVersion(ctx, packageSignature.Name)
		return version, "homebrew", err
	case "dpkg":
		version, err := detectDPKGPackageVersion(ctx, packageSignature.Name)
		return version, "dpkg", err
	case "rpm":
		version, err := detectRPMPackageVersion(ctx, packageSignature.Name)
		return version, "rpm", err
	case "apk":
		version, err := detectAPKPackageVersion(ctx, packageSignature.Name)
		return version, "apk", err
	default:
		return "", "", os.ErrNotExist
	}
}

func detectNPMGlobalPackageVersion(ctx context.Context, packageName string) (string, error) {
	candidates := findExecutableCandidates([]string{"npm"})
	if len(candidates) == 0 {
		return "", os.ErrNotExist
	}
	output, err := runAgentDetectionCommand(ctx, candidates[0], []string{"list", "-g", "--depth=0", "--json", packageName})
	if err != nil {
		return "", err
	}

	version, ok, err := packageVersionFromJSON(output, packageName)
	if err != nil {
		return "", fmt.Errorf("decode npm package inventory: %w", err)
	}
	if !ok {
		return "", os.ErrNotExist
	}

	return version, nil
}

func detectPNPMGlobalPackageVersion(ctx context.Context, packageName string) (string, error) {
	candidates := findExecutableCandidates([]string{"pnpm"})
	if len(candidates) == 0 {
		return "", os.ErrNotExist
	}
	output, err := runAgentDetectionCommand(ctx, candidates[0], []string{"list", "-g", "--depth=0", "--json"})
	if err != nil {
		return "", err
	}

	version, ok, err := packageVersionFromJSON(output, packageName)
	if err != nil {
		return "", fmt.Errorf("decode pnpm package inventory: %w", err)
	}
	if !ok {
		return "", os.ErrNotExist
	}

	return version, nil
}

func detectBunGlobalPackageVersion(ctx context.Context, packageName string) (string, error) {
	candidates := findExecutableCandidates([]string{"bun"})
	if len(candidates) == 0 {
		return "", os.ErrNotExist
	}
	output, err := runAgentDetectionCommand(ctx, candidates[0], []string{"pm", "ls", "-g"})
	if err != nil {
		return "", err
	}
	if version, ok := packageVersionFromText(output, packageName); ok {
		return version, nil
	}

	return "", os.ErrNotExist
}

func detectHomebrewPackageVersion(ctx context.Context, packageName string) (string, error) {
	candidates := findExecutableCandidates([]string{"brew"})
	if len(candidates) == 0 {
		return "", os.ErrNotExist
	}
	argSets := [][]string{
		{"list", "--versions", packageName},
		{"list", "--formula", "--versions", packageName},
		{"list", "--cask", "--versions", packageName},
	}
	for _, args := range argSets {
		output, err := runAgentDetectionCommand(ctx, candidates[0], args)
		if err != nil {
			continue
		}
		if version, ok := homebrewPackageVersionFromText(output, packageName); ok {
			return version, nil
		}
	}

	return "", os.ErrNotExist
}

func detectDPKGPackageVersion(ctx context.Context, packageName string) (string, error) {
	candidates := findExecutableCandidates([]string{"dpkg-query"})
	if len(candidates) == 0 {
		return "", os.ErrNotExist
	}
	output, err := runAgentDetectionCommand(ctx, candidates[0], []string{"-W", packageName})
	if err != nil {
		return "", err
	}
	if version, ok := packageVersionFromTabularText(output, packageName); ok {
		return version, nil
	}

	return "", os.ErrNotExist
}

func detectRPMPackageVersion(ctx context.Context, packageName string) (string, error) {
	candidates := findExecutableCandidates([]string{"rpm"})
	if len(candidates) == 0 {
		return "", os.ErrNotExist
	}
	output, err := runAgentDetectionCommand(ctx, candidates[0], []string{"-q", "--qf", "%{NAME}\t%{VERSION}\n", packageName})
	if err != nil {
		return "", err
	}
	if version, ok := packageVersionFromTabularText(output, packageName); ok {
		return version, nil
	}

	return "", os.ErrNotExist
}

func detectAPKPackageVersion(ctx context.Context, packageName string) (string, error) {
	candidates := findExecutableCandidates([]string{"apk"})
	if len(candidates) == 0 {
		return "", os.ErrNotExist
	}
	output, err := runAgentDetectionCommand(ctx, candidates[0], []string{"info", "-v", packageName})
	if err != nil {
		return "", err
	}
	if version, ok := apkPackageVersionFromText(output, packageName); ok {
		return version, nil
	}

	return "", os.ErrNotExist
}

func packageVersionFromJSON(output string, packageName string) (string, bool, error) {
	var payload any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return "", false, fmt.Errorf("decode package inventory json: %w", err)
	}

	return findPackageVersionInJSON(payload, packageName)
}

func findPackageVersionInJSON(value any, packageName string) (string, bool, error) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if version, ok, err := findPackageVersionInJSON(item, packageName); ok || err != nil {
				return version, ok, err
			}
		}
	case map[string]any:
		if nameValue, ok := typed["name"].(string); ok && nameValue == packageName {
			if version, ok := typed["version"].(string); ok && strings.TrimSpace(version) != "" {
				return strings.TrimSpace(version), true, nil
			}
		}
		if dependencies, ok := typed["dependencies"].(map[string]any); ok {
			if dependency, ok := dependencies[packageName]; ok {
				if dependencyMap, ok := dependency.(map[string]any); ok {
					if version, ok := dependencyMap["version"].(string); ok && strings.TrimSpace(version) != "" {
						return strings.TrimSpace(version), true, nil
					}
				}
			}
			for _, dependency := range dependencies {
				if version, ok, err := findPackageVersionInJSON(dependency, packageName); ok || err != nil {
					return version, ok, err
				}
			}
		}
		for key, child := range typed {
			if key == "dependencies" {
				continue
			}
			if version, ok, err := findPackageVersionInJSON(child, packageName); ok || err != nil {
				return version, ok, err
			}
		}
	}

	return "", false, nil
}

func packageVersionFromText(output string, packageName string) (string, bool) {
	for _, field := range strings.Fields(output) {
		if version, ok := packageVersionFromToken(field, packageName); ok {
			return version, true
		}
	}

	return "", false
}

func packageVersionFromToken(token string, packageName string) (string, bool) {
	trimmed := strings.Trim(token, " ,;()[]{}")
	if !strings.HasPrefix(trimmed, packageName+"@") {
		return "", false
	}
	version := strings.TrimSpace(strings.TrimPrefix(trimmed, packageName+"@"))
	if version == "" {
		return "", false
	}

	return version, true
}

func homebrewPackageVersionFromText(output string, packageName string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != packageName {
			continue
		}
		return fields[1], true
	}

	return "", false
}

func packageVersionFromTabularText(output string, packageName string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != packageName {
			continue
		}
		return fields[1], true
	}

	return "", false
}

func apkPackageVersionFromText(output string, packageName string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, packageName+"-") {
			continue
		}
		version := strings.TrimPrefix(line, packageName+"-")
		if version == "" {
			continue
		}
		return version, true
	}

	return "", false
}

func runAgentDetectionCommand(ctx context.Context, executablePath string, args []string) (string, error) {
	versionCtx, cancel := context.WithTimeout(ctx, agentDetectionCommandTimeout)
	defer cancel()

	//nolint:gosec // Detection runs a candidate executable with fixed, non-mutating version arguments only.
	command := exec.CommandContext(versionCtx, executablePath, args...)
	command.Stdin = strings.NewReader("")
	output, err := command.CombinedOutput()
	if versionCtx.Err() != nil {
		return "", fmt.Errorf("detection command timed out for %s", filepath.Base(executablePath))
	}
	if err != nil {
		return "", fmt.Errorf("detection command failed for %s", filepath.Base(executablePath))
	}

	return string(output), nil
}

func readPlistString(filename string, key string) (string, error) {
	//nolint:gosec // App bundle metadata path is built from a detected .app bundle and reads Info.plist only.
	file, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("open plist metadata %s: %w", filename, err)
	}
	defer file.Close()

	decoder := xml.NewDecoder(file)
	var currentKey string
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", os.ErrNotExist
			}
			return "", fmt.Errorf("decode plist metadata %s: %w", filename, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return "", fmt.Errorf("decode plist key %s: %w", filename, err)
			}
			currentKey = strings.TrimSpace(value)
		case "string":
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return "", fmt.Errorf("decode plist string %s: %w", filename, err)
			}
			if currentKey == key {
				return strings.TrimSpace(value), nil
			}
			currentKey = ""
		}
	}
}

func firstNonEmptyLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func agentVersionMatches(agentID string, version string) bool {
	normalized := strings.ToLower(strings.TrimSpace(version))
	if normalized == "" {
		return false
	}
	if startsWithVersionNumber(normalized) {
		return true
	}
	switch agentID {
	case "codex":
		return strings.Contains(normalized, "codex")
	case "claudecode":
		return strings.Contains(normalized, "claude")
	case "openclaw":
		return strings.Contains(normalized, "openclaw")
	default:
		return false
	}
}

func startsWithVersionNumber(value string) bool {
	if value == "" || value[0] < '0' || value[0] > '9' {
		return false
	}
	for _, char := range value {
		if char >= '0' && char <= '9' {
			continue
		}
		return char == '.'
	}

	return true
}

func orbitOwnedAgentFootprint(repoRoot string, absolutePath string) bool {
	info, err := os.Lstat(absolutePath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	target, err := os.Readlink(absolutePath)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(absolutePath), target)
	}

	return filepath.Clean(target) == filepath.Clean(filepath.Join(repoRoot, "AGENTS.md"))
}

func redactHome(homeDir string, path string) string {
	if homeDir == "" || path == "" {
		return filepath.ToSlash(path)
	}
	cleanHome := filepath.Clean(homeDir)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(filepath.Separator)
	if strings.HasPrefix(cleanPath, prefix) {
		return filepath.ToSlash(filepath.Join("~", strings.TrimPrefix(cleanPath, prefix)))
	}

	return filepath.ToSlash(cleanPath)
}
