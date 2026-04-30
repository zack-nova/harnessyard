package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkApplyOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.FrameworkApplyResult
	Readiness harnesspkg.ReadinessReport `json:"readiness"`
}

type frameworkApplyMultiOutput struct {
	HarnessRoot string                            `json:"harness_root"`
	HarnessID   string                            `json:"harness_id"`
	Frameworks  []string                          `json:"frameworks"`
	Results     []harnesspkg.FrameworkApplyResult `json:"results"`
	Readiness   harnesspkg.ReadinessReport        `json:"readiness"`
}

// NewFrameworkApplyCommand creates the harness framework apply command.
func NewFrameworkApplyCommand() *cobra.Command {
	var yes bool
	var projectOnly bool
	var global bool
	var allowGlobalFallback bool
	var hooks bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply framework-managed project and global side effects for the current runtime",
		Long: "Apply framework-managed project/global side effects for the current runtime and record\n" +
			"their ownership in the repo-local activation ledger.",
		Example: "" +
			"  harness framework apply\n" +
			"  harness framework apply --yes\n" +
			"  harness framework apply --hooks --yes\n" +
			"  harness framework apply --global\n" +
			"  harness framework apply --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			selectedFrameworks, err := selectedFrameworksForApply(resolved.Repo.GitDir)
			if err != nil {
				return err
			}
			routeChoice, err := frameworkApplyRouteChoiceFromCommand(cmd, frameworkApplyRouteChoiceInput{
				Yes:                 yes,
				ProjectOnly:         projectOnly,
				Global:              global,
				AllowGlobalFallback: allowGlobalFallback,
				JSON:                jsonOutput,
				RepoRoot:            resolved.Repo.Root,
				GitDir:              resolved.Repo.GitDir,
				HarnessID:           resolved.Manifest.Runtime.ID,
				Frameworks:          selectedFrameworks,
			})
			if err != nil {
				return err
			}
			if err := confirmFrameworkHookPreviewFromCommand(cmd, frameworkHookPreviewInput{
				Enabled:    hooks,
				Yes:        yes,
				JSON:       jsonOutput,
				RepoRoot:   resolved.Repo.Root,
				GitDir:     resolved.Repo.GitDir,
				HarnessID:  resolved.Manifest.Runtime.ID,
				Frameworks: selectedFrameworks,
			}); err != nil {
				return err
			}

			if len(selectedFrameworks) > 1 {
				results := make([]harnesspkg.FrameworkApplyResult, 0, len(selectedFrameworks))
				for _, frameworkID := range selectedFrameworks {
					result, err := harnesspkg.ApplyFramework(cmd.Context(), harnesspkg.FrameworkApplyInput{
						RepoRoot:            resolved.Repo.Root,
						GitDir:              resolved.Repo.GitDir,
						HarnessID:           resolved.Manifest.Runtime.ID,
						FrameworkOverride:   frameworkID,
						RouteChoice:         routeChoice,
						AllowGlobalFallback: allowGlobalFallback,
						EnableHooks:         hooks,
					})
					if err != nil {
						return fmt.Errorf("apply framework activation for %s: %w", frameworkID, err)
					}
					if err := emitFrameworkApplyWarnings(cmd, result.Warnings); err != nil {
						return err
					}
					results = append(results, result)
				}
				readiness, err := evaluateCommandReadiness(cmd.Context(), resolved.Repo.Root)
				if err != nil {
					return err
				}
				output := frameworkApplyMultiOutput{
					HarnessRoot: resolved.Repo.Root,
					HarnessID:   resolved.Manifest.Runtime.ID,
					Frameworks:  selectedFrameworks,
					Results:     results,
					Readiness:   readiness,
				}
				if jsonOutput {
					return emitJSON(cmd.OutOrStdout(), output)
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_root: %s\n", output.HarnessRoot); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", output.HarnessID); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				for _, result := range output.Results {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "framework: %s status=%s\n", result.Framework, result.Status); err != nil {
						return fmt.Errorf("write command output: %w", err)
					}
				}
				return emitPostActionReadinessText(cmd.OutOrStdout(), output.Readiness)
			}

			result, err := harnesspkg.ApplyFramework(cmd.Context(), harnesspkg.FrameworkApplyInput{
				RepoRoot:            resolved.Repo.Root,
				GitDir:              resolved.Repo.GitDir,
				HarnessID:           resolved.Manifest.Runtime.ID,
				RouteChoice:         routeChoice,
				AllowGlobalFallback: allowGlobalFallback,
				EnableHooks:         hooks,
			})
			if err != nil {
				return fmt.Errorf("apply framework activation: %w", err)
			}
			if err := emitFrameworkApplyWarnings(cmd, result.Warnings); err != nil {
				return err
			}
			readiness, err := evaluateCommandReadiness(cmd.Context(), resolved.Repo.Root)
			if err != nil {
				return err
			}

			output := frameworkApplyOutput{
				HarnessRoot:          resolved.Repo.Root,
				HarnessID:            resolved.Manifest.Runtime.ID,
				FrameworkApplyResult: result,
				Readiness:            readiness,
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_root: %s\n", output.HarnessRoot); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", output.HarnessID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "framework: %s\n", output.Framework); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "activation_path: %s\n", output.ActivationPath); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, warning := range output.Warnings {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warning); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if err := emitPostActionReadinessText(cmd.OutOrStdout(), output.Readiness); err != nil {
				return err
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)
	cmd.Flags().BoolVar(&yes, "yes", false, "Apply the recommended project route without prompting")
	cmd.Flags().BoolVar(&projectOnly, "project-only", false, "Apply only project-level activation routes")
	cmd.Flags().BoolVar(&global, "global", false, "Apply eligible command and skill artifacts through global registration")
	cmd.Flags().BoolVar(&allowGlobalFallback, "allow-global-fallback", false, "Allow failed project route artifacts to fall back to global registration")
	cmd.Flags().BoolVar(&hooks, "hooks", false, "Preview and apply framework hook activation")

	return cmd
}

func emitFrameworkApplyWarnings(cmd *cobra.Command, warnings []string) error {
	for _, warning := range warnings {
		if !strings.Contains(warning, "global agent config") {
			continue
		}
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning); err != nil {
			return fmt.Errorf("write command warning: %w", err)
		}
	}

	return nil
}

func selectedFrameworksForApply(gitDir string) ([]string, error) {
	selection, err := harnesspkg.LoadFrameworkSelection(gitDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("load framework selection: %w", err)
	}
	frameworks := harnesspkg.FrameworkSelectionIDs(selection)
	if len(frameworks) <= 1 {
		return frameworks, nil
	}
	normalized := make([]string, 0, len(frameworks))
	seen := map[string]struct{}{}
	for _, frameworkID := range frameworks {
		adapter, ok := harnesspkg.LookupFrameworkAdapter(frameworkID)
		if !ok {
			return nil, fmt.Errorf("framework %q is not supported by this build", frameworkID)
		}
		if _, ok := seen[adapter.ID]; ok {
			continue
		}
		seen[adapter.ID] = struct{}{}
		normalized = append(normalized, adapter.ID)
	}

	return normalized, nil
}

type frameworkApplyRouteChoiceInput struct {
	Yes                 bool
	ProjectOnly         bool
	Global              bool
	AllowGlobalFallback bool
	JSON                bool
	RepoRoot            string
	GitDir              string
	HarnessID           string
	Frameworks          []string
}

type frameworkHookPreviewInput struct {
	Enabled    bool
	Yes        bool
	JSON       bool
	RepoRoot   string
	GitDir     string
	HarnessID  string
	Frameworks []string
}

func confirmFrameworkHookPreviewFromCommand(cmd *cobra.Command, input frameworkHookPreviewInput) error {
	if !input.Enabled {
		return nil
	}
	plans, err := frameworkApplyPlansForSelection(cmd.Context(), input.RepoRoot, input.GitDir, input.HarnessID, input.Frameworks)
	if err != nil {
		return fmt.Errorf("build hook activation preview: %w", err)
	}
	outputs := []harnesspkg.FrameworkRoutePlanOutput{}
	for _, plan := range plans {
		outputs = append(outputs, plan.HookPreview...)
	}
	if err := writeFrameworkHookPreview(cmd.ErrOrStderr(), outputs); err != nil {
		return err
	}
	if input.Yes {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Apply hook activation? accepted by --yes"); err != nil {
			return fmt.Errorf("write hook preview acceptance: %w", err)
		}
		return nil
	}
	if input.JSON {
		return fmt.Errorf("--hooks with --json requires --yes to accept the hook preview")
	}
	confirmed, err := confirmDefaultYes(cmd.InOrStdin(), cmd.ErrOrStderr(), "Apply hook activation? [Y/n] ")
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("hook activation declined")
	}

	return nil
}

func writeFrameworkHookPreview(writer io.Writer, outputs []harnesspkg.FrameworkRoutePlanOutput) error {
	if writer == nil {
		return fmt.Errorf("interactive prompt writer must be configured")
	}
	if _, err := fmt.Fprintln(writer, "Hook activation may execute project scripts when the agent runs."); err != nil {
		return fmt.Errorf("write hook preview: %w", err)
	}
	if len(outputs) == 0 {
		if _, err := fmt.Fprintln(writer, "No supported hook outputs are planned."); err != nil {
			return fmt.Errorf("write hook preview: %w", err)
		}
		return nil
	}
	hasHybrid := false
	for _, output := range outputs {
		if output.EffectiveScope == "hybrid" || output.Route == "hybrid_hook_activation" {
			hasHybrid = true
			break
		}
	}
	if hasHybrid {
		if _, err := fmt.Fprintln(writer, "OpenClaw hook activation requires:"); err != nil {
			return fmt.Errorf("write hook preview: %w", err)
		}
		for _, output := range outputs {
			if output.Mode == "execute-later" || output.Route == "unsupported_event" {
				continue
			}
			prefix := "  project/workspace files:"
			if strings.HasPrefix(output.Path, "~/") {
				prefix = "  user config patch:"
			}
			if _, err := fmt.Fprintf(writer, "%s %s\n", prefix, output.Path); err != nil {
				return fmt.Errorf("write hook preview: %w", err)
			}
		}
	} else {
		if _, err := fmt.Fprintln(writer, "Will write:"); err != nil {
			return fmt.Errorf("write hook preview: %w", err)
		}
		for _, output := range outputs {
			if output.Mode == "execute-later" || output.Route == "unsupported_event" {
				continue
			}
			scope := "project"
			if strings.HasPrefix(output.Path, "~/") {
				scope = "user"
			}
			if _, err := fmt.Fprintf(writer, "  %s %s\n", scope, output.Path); err != nil {
				return fmt.Errorf("write hook preview: %w", err)
			}
		}
	}
	if _, err := fmt.Fprintln(writer, "Will execute later:"); err != nil {
		return fmt.Errorf("write hook preview: %w", err)
	}
	for _, output := range outputs {
		if output.Mode != "execute-later" {
			continue
		}
		if _, err := fmt.Fprintf(writer, "  %s\n", output.Path); err != nil {
			return fmt.Errorf("write hook preview: %w", err)
		}
	}

	return nil
}

func frameworkApplyRouteChoiceFromCommand(cmd *cobra.Command, input frameworkApplyRouteChoiceInput) (harnesspkg.FrameworkApplyRouteChoice, error) {
	projectSelected := input.Yes || input.ProjectOnly
	if input.Global && projectSelected {
		return harnesspkg.FrameworkApplyRouteAuto, fmt.Errorf("--global cannot be combined with --yes or --project-only")
	}
	if input.Global {
		return harnesspkg.FrameworkApplyRouteGlobal, nil
	}
	if projectSelected || input.JSON {
		return harnesspkg.FrameworkApplyRouteProject, nil
	}

	plans, err := frameworkApplyPlansForSelection(cmd.Context(), input.RepoRoot, input.GitDir, input.HarnessID, input.Frameworks)
	if err != nil {
		return harnesspkg.FrameworkApplyRouteAuto, fmt.Errorf("build framework activation prompt plan: %w", err)
	}
	hasRouteChoice := false
	for _, plan := range plans {
		if len(plan.RecommendedProjectOutputs) > 0 && len(plan.OptionalGlobalOutputs) > 0 {
			hasRouteChoice = true
			break
		}
	}
	if !hasRouteChoice {
		return harnesspkg.FrameworkApplyRouteProject, nil
	}

	confirmed, err := confirmDefaultYes(cmd.InOrStdin(), cmd.ErrOrStderr(), "Apply command and skill artifacts as project skills? [Y/n] ")
	if err != nil {
		return harnesspkg.FrameworkApplyRouteAuto, err
	}
	if confirmed {
		return harnesspkg.FrameworkApplyRouteProject, nil
	}

	return harnesspkg.FrameworkApplyRouteGlobal, nil
}

func frameworkApplyPlansForSelection(ctx context.Context, repoRoot string, gitDir string, harnessID string, frameworks []string) ([]harnesspkg.FrameworkPlan, error) {
	if len(frameworks) == 0 {
		plan, err := harnesspkg.BuildFrameworkPlan(ctx, repoRoot, gitDir, harnessID)
		if err != nil {
			return nil, fmt.Errorf("build framework activation prompt plan: %w", err)
		}
		return []harnesspkg.FrameworkPlan{plan}, nil
	}

	plans := make([]harnesspkg.FrameworkPlan, 0, len(frameworks))
	for _, frameworkID := range frameworks {
		plan, err := harnesspkg.BuildFrameworkPlanForFramework(ctx, repoRoot, gitDir, harnessID, frameworkID)
		if err != nil {
			return nil, fmt.Errorf("build framework activation prompt plan for %s: %w", frameworkID, err)
		}
		plans = append(plans, plan)
	}

	return plans, nil
}

func confirmDefaultYes(reader io.Reader, writer io.Writer, prompt string) (bool, error) {
	if reader == nil {
		return false, fmt.Errorf("interactive input reader must be configured")
	}
	if writer == nil {
		return false, fmt.Errorf("interactive prompt writer must be configured")
	}
	if _, err := fmt.Fprint(writer, prompt); err != nil {
		return false, fmt.Errorf("write confirmation prompt: %w", err)
	}
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("read interactive confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("interactive confirmation must be yes or no")
	}
}
