package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// FrameworkActivationOutput stores one framework-managed side effect owned by the current runtime.
type FrameworkActivationOutput struct {
	Path           string   `json:"path"`
	AbsolutePath   string   `json:"absolute_path"`
	Kind           string   `json:"kind"`
	Action         string   `json:"action"`
	Target         string   `json:"target"`
	OrbitID        string   `json:"orbit_id,omitempty"`
	Package        string   `json:"package,omitempty"`
	AddonID        string   `json:"addon_id,omitempty"`
	Artifact       string   `json:"artifact,omitempty"`
	ArtifactType   string   `json:"artifact_type,omitempty"`
	Source         string   `json:"source,omitempty"`
	Sidecar        string   `json:"sidecar,omitempty"`
	SourceFiles    []string `json:"source_files,omitempty"`
	Route          string   `json:"route,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	EffectiveScope string   `json:"effective_scope,omitempty"`
	Invocation     []string `json:"invocation,omitempty"`
	GeneratedKeys  []string `json:"generated_keys,omitempty"`
	PatchOwnedKeys []string `json:"patch_owned_keys,omitempty"`
	BackupPath     string   `json:"backup_path,omitempty"`
	HandlerDigest  string   `json:"handler_digest,omitempty"`
}

// FrameworkActivationPackageHook stores one package hook add-on applied through native hook activation.
type FrameworkActivationPackageHook struct {
	OrbitID        string `json:"orbit_id"`
	Package        string `json:"package"`
	AddonID        string `json:"addon_id"`
	DisplayID      string `json:"display_id"`
	Required       bool   `json:"required,omitempty"`
	Source         string `json:"source"`
	EventKind      string `json:"event_kind"`
	NativeEvent    string `json:"native_event"`
	HandlerPath    string `json:"handler_path"`
	HandlerDigest  string `json:"handler_digest"`
	HookApplyMode  string `json:"hook_apply_mode"`
	EffectiveScope string `json:"effective_scope"`
}

// FrameworkActivation stores one repo-local framework activation ledger entry.
type FrameworkActivation struct {
	Framework             string                           `json:"framework"`
	ResolutionSource      FrameworkSelectionSource         `json:"resolution_source"`
	RepoRoot              string                           `json:"repo_root"`
	AppliedAt             time.Time                        `json:"applied_at"`
	GuidanceHash          string                           `json:"guidance_hash"`
	CapabilitiesHash      string                           `json:"capabilities_hash"`
	SelectionHash         string                           `json:"selection_hash"`
	RuntimeAgentTruthHash string                           `json:"runtime_agent_truth_hash,omitempty"`
	ProjectOutputs        []FrameworkActivationOutput      `json:"project_outputs,omitempty"`
	GlobalOutputs         []FrameworkActivationOutput      `json:"global_outputs,omitempty"`
	PackageHooks          []FrameworkActivationPackageHook `json:"package_hooks,omitempty"`
}

// FrameworkActivationPath returns the absolute activation ledger path for one framework.
func FrameworkActivationPath(gitDir string, frameworkID string) string {
	return filepath.Join(gitDir, "orbit", "state", "agents", "activations", frameworkID+".json")
}

func legacyFrameworkActivationPath(gitDir string, frameworkID string) string {
	return filepath.Join(gitDir, "orbit", "state", "frameworks", "activations", frameworkID+".json")
}

func frameworkActivationDir(gitDir string) string {
	return filepath.Join(gitDir, "orbit", "state", "agents", "activations")
}

func legacyFrameworkActivationDir(gitDir string) string {
	return filepath.Join(gitDir, "orbit", "state", "frameworks", "activations")
}

// LoadFrameworkActivation reads and validates one repo-local framework activation ledger entry.
func LoadFrameworkActivation(gitDir string, frameworkID string) (FrameworkActivation, error) {
	filename := FrameworkActivationPath(gitDir, frameworkID)
	activation, err := loadFrameworkActivationAtPath(filename)
	if err == nil {
		return activation, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return FrameworkActivation{}, err
	}

	return loadFrameworkActivationAtPath(legacyFrameworkActivationPath(gitDir, frameworkID))
}

func loadFrameworkActivationAtPath(filename string) (FrameworkActivation, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // The path is repo-local and built from the fixed framework activation contract path.
	if err != nil {
		return FrameworkActivation{}, fmt.Errorf("read %s: %w", filename, err)
	}

	var activation FrameworkActivation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&activation); err != nil {
		return FrameworkActivation{}, fmt.Errorf("decode %s: %w", filename, err)
	}
	if err := ValidateFrameworkActivation(activation); err != nil {
		return FrameworkActivation{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return activation, nil
}

// ValidateFrameworkActivation validates one repo-local framework activation ledger entry.
func ValidateFrameworkActivation(activation FrameworkActivation) error {
	if err := ids.ValidateOrbitID(activation.Framework); err != nil {
		return fmt.Errorf("framework: %w", err)
	}
	switch activation.ResolutionSource {
	case FrameworkSelectionSourceExplicitLocal,
		FrameworkSelectionSourceLocalHint,
		FrameworkSelectionSourceProjectDetection,
		FrameworkSelectionSourceRecommendedDefault,
		FrameworkSelectionSourcePackageRecommendation:
	default:
		return fmt.Errorf("resolution_source must be one of %q, %q, %q, %q, or %q",
			FrameworkSelectionSourceExplicitLocal,
			FrameworkSelectionSourceLocalHint,
			FrameworkSelectionSourceProjectDetection,
			FrameworkSelectionSourceRecommendedDefault,
			FrameworkSelectionSourcePackageRecommendation,
		)
	}
	if activation.RepoRoot == "" {
		return fmt.Errorf("repo_root must not be empty")
	}
	if activation.AppliedAt.IsZero() {
		return fmt.Errorf("applied_at must be set")
	}
	if activation.GuidanceHash == "" {
		return fmt.Errorf("guidance_hash must not be empty")
	}
	if activation.CapabilitiesHash == "" {
		return fmt.Errorf("capabilities_hash must not be empty")
	}
	if activation.SelectionHash == "" {
		return fmt.Errorf("selection_hash must not be empty")
	}
	if err := validateFrameworkActivationOutputs("project_outputs", activation.ProjectOutputs); err != nil {
		return err
	}
	if err := validateFrameworkActivationOutputs("global_outputs", activation.GlobalOutputs); err != nil {
		return err
	}
	if err := validateFrameworkActivationPackageHooks(activation.PackageHooks); err != nil {
		return err
	}

	return nil
}

func validateFrameworkActivationOutputs(field string, outputs []FrameworkActivationOutput) error {
	for index, output := range outputs {
		if output.Path == "" {
			return fmt.Errorf("%s[%d].path must not be empty", field, index)
		}
		if output.AbsolutePath == "" {
			return fmt.Errorf("%s[%d].absolute_path must not be empty", field, index)
		}
		if output.Kind == "" {
			return fmt.Errorf("%s[%d].kind must not be empty", field, index)
		}
		if output.Action == "" {
			return fmt.Errorf("%s[%d].action must not be empty", field, index)
		}
		if output.Target == "" {
			return fmt.Errorf("%s[%d].target must not be empty", field, index)
		}
	}

	return nil
}

func validateFrameworkActivationPackageHooks(hooks []FrameworkActivationPackageHook) error {
	seen := map[string]struct{}{}
	for index, hook := range hooks {
		prefix := fmt.Sprintf("package_hooks[%d]", index)
		if err := ids.ValidateOrbitID(hook.OrbitID); err != nil {
			return fmt.Errorf("%s.orbit_id: %w", prefix, err)
		}
		if err := ids.ValidateOrbitID(hook.Package); err != nil {
			return fmt.Errorf("%s.package: %w", prefix, err)
		}
		if err := ids.ValidateOrbitID(hook.AddonID); err != nil {
			return fmt.Errorf("%s.addon_id: %w", prefix, err)
		}
		if hook.DisplayID == "" {
			return fmt.Errorf("%s.display_id must not be empty", prefix)
		}
		if _, ok := seen[hook.DisplayID]; ok {
			return fmt.Errorf("%s.display_id %q is duplicated", prefix, hook.DisplayID)
		}
		seen[hook.DisplayID] = struct{}{}
		if hook.Source == "" {
			return fmt.Errorf("%s.source must not be empty", prefix)
		}
		if hook.EventKind == "" {
			return fmt.Errorf("%s.event_kind must not be empty", prefix)
		}
		if hook.NativeEvent == "" {
			return fmt.Errorf("%s.native_event must not be empty", prefix)
		}
		if hook.HandlerPath == "" {
			return fmt.Errorf("%s.handler_path must not be empty", prefix)
		}
		if hook.HandlerDigest == "" {
			return fmt.Errorf("%s.handler_digest must not be empty", prefix)
		}
		if hook.HookApplyMode == "" {
			return fmt.Errorf("%s.hook_apply_mode must not be empty", prefix)
		}
		if hook.EffectiveScope == "" {
			return fmt.Errorf("%s.effective_scope must not be empty", prefix)
		}
	}

	return nil
}

// WriteFrameworkActivation validates and writes one repo-local framework activation ledger entry.
func WriteFrameworkActivation(gitDir string, activation FrameworkActivation) (string, error) {
	if err := ValidateFrameworkActivation(activation); err != nil {
		return "", fmt.Errorf("validate framework activation: %w", err)
	}

	filename := FrameworkActivationPath(gitDir, activation.Framework)
	data, err := json.MarshalIndent(activation, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode framework activation: %w", err)
	}
	if err := contractutil.AtomicWriteFile(filename, append(data, '\n')); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}
	if err := os.Remove(legacyFrameworkActivationPath(gitDir, activation.Framework)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove legacy framework activation: %w", err)
	}

	return filename, nil
}

// ListFrameworkActivationIDs lists all repo-local framework activation ledger ids in stable order.
func ListFrameworkActivationIDs(gitDir string) ([]string, error) {
	idsList, err := listFrameworkActivationIDsAtPath(frameworkActivationDir(gitDir))
	if err == nil {
		return idsList, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	return listFrameworkActivationIDsAtPath(legacyFrameworkActivationDir(gitDir))
}

func listFrameworkActivationIDsAtPath(dirname string) ([]string, error) {
	entries, err := os.ReadDir(dirname)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", dirname, err)
	}

	idsList := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if filepath.Ext(filename) != ".json" {
			continue
		}
		frameworkID := filename[:len(filename)-len(filepath.Ext(filename))]
		if err := ids.ValidateOrbitID(frameworkID); err != nil {
			return nil, fmt.Errorf("activation filename %q: %w", filename, err)
		}
		idsList = append(idsList, frameworkID)
	}
	sort.Strings(idsList)

	return idsList, nil
}

// RemoveFrameworkActivation removes one repo-local framework activation ledger entry when present.
func RemoveFrameworkActivation(gitDir string, frameworkID string) error {
	filename := FrameworkActivationPath(gitDir, frameworkID)
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", filename, err)
	}
	legacyFilename := legacyFrameworkActivationPath(gitDir, frameworkID)
	if err := os.Remove(legacyFilename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", legacyFilename, err)
	}

	return nil
}
