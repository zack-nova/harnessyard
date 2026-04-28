package orbittemplate

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

// BindingPrompter fills missing required bindings during interactive apply flows.
type BindingPrompter interface {
	PromptBindings(ctx context.Context, unresolved []bindings.UnresolvedBinding) (map[string]bindings.VariableBinding, error)
}

// ConfirmPrompter confirms one interactive yes/no action during template author flows.
type ConfirmPrompter interface {
	Confirm(ctx context.Context, prompt string) (bool, error)
}

// SourceBranchPushDecision captures how an interactive publish should handle source branch push.
type SourceBranchPushDecision string

const (
	SourceBranchPushDecisionPush  SourceBranchPushDecision = "push"
	SourceBranchPushDecisionAbort SourceBranchPushDecision = "abort"
)

// SourceBranchStatus describes the source branch relationship to the selected remote.
type SourceBranchStatus string

const (
	SourceBranchStatusMissing  SourceBranchStatus = "missing"
	SourceBranchStatusAhead    SourceBranchStatus = "ahead"
	SourceBranchStatusEqual    SourceBranchStatus = "equal"
	SourceBranchStatusBehind   SourceBranchStatus = "behind"
	SourceBranchStatusDiverged SourceBranchStatus = "diverged"
)

// SourceBranchPushPrompt describes a source branch push confirmation request.
type SourceBranchPushPrompt struct {
	Remote       string
	SourceBranch string
	Status       SourceBranchStatus
}

// SourceBranchPushPrompter confirms whether publish should push the source branch first.
type SourceBranchPushPrompter interface {
	PromptSourceBranchPush(ctx context.Context, prompt SourceBranchPushPrompt) (SourceBranchPushDecision, error)
}

// LineBindingPrompter prompts for one binding per line via stdin/stderr style streams.
type LineBindingPrompter struct {
	Reader io.Reader
	Writer io.Writer
}

// LineConfirmPrompter prompts for one yes/no confirmation via stdin/stderr style streams.
type LineConfirmPrompter struct {
	Reader io.Reader
	Writer io.Writer
}

// LineSourceBranchPushPrompter prompts for pushing a source branch before package publish.
type LineSourceBranchPushPrompter struct {
	Reader io.Reader
	Writer io.Writer
}

// PromptBindings prompts for each unresolved binding and returns the filled values.
func (prompter LineBindingPrompter) PromptBindings(ctx context.Context, unresolved []bindings.UnresolvedBinding) (map[string]bindings.VariableBinding, error) {
	if prompter.Reader == nil {
		return nil, fmt.Errorf("interactive input reader must be configured")
	}
	if prompter.Writer == nil {
		return nil, fmt.Errorf("interactive prompt writer must be configured")
	}

	reader := bufio.NewReader(prompter.Reader)
	values := make(map[string]bindings.VariableBinding, len(unresolved))
	for _, missing := range unresolved {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("prompt context canceled: %w", err)
		}

		if _, err := fmt.Fprint(prompter.Writer, promptForBinding(missing)); err != nil {
			return nil, fmt.Errorf("write prompt for %s: %w", missing.Name, err)
		}

		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read interactive value for %s: %w", missing.Name, err)
		}

		value := strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("interactive value for %s must not be empty", missing.Name)
		}

		values[missing.Name] = bindings.VariableBinding{
			Value:       value,
			Description: missing.Description,
		}
	}

	return values, nil
}

func promptForBinding(binding bindings.UnresolvedBinding) string {
	if binding.Description != "" {
		return fmt.Sprintf("%s (%s): ", binding.Name, binding.Description)
	}

	return fmt.Sprintf("%s: ", binding.Name)
}

// Confirm prompts once and accepts yes/y or no/n/empty as the answer.
func (prompter LineConfirmPrompter) Confirm(ctx context.Context, prompt string) (bool, error) {
	if prompter.Reader == nil {
		return false, fmt.Errorf("interactive input reader must be configured")
	}
	if prompter.Writer == nil {
		return false, fmt.Errorf("interactive prompt writer must be configured")
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("prompt context canceled: %w", err)
	}
	if _, err := fmt.Fprint(prompter.Writer, prompt); err != nil {
		return false, fmt.Errorf("write confirmation prompt: %w", err)
	}

	reader := bufio.NewReader(prompter.Reader)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("read interactive confirmation: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	case "", "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("interactive confirmation must be yes or no")
	}
}

// PromptSourceBranchPush prompts once; enter/y/yes pushes, q/n/no aborts.
func (prompter LineSourceBranchPushPrompter) PromptSourceBranchPush(ctx context.Context, prompt SourceBranchPushPrompt) (SourceBranchPushDecision, error) {
	if prompter.Reader == nil {
		return "", fmt.Errorf("interactive input reader must be configured")
	}
	if prompter.Writer == nil {
		return "", fmt.Errorf("interactive prompt writer must be configured")
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("prompt context canceled: %w", err)
	}
	if _, err := fmt.Fprint(prompter.Writer, formatSourceBranchPushPrompt(prompt)); err != nil {
		return "", fmt.Errorf("write source branch push prompt: %w", err)
	}

	reader := bufio.NewReader(prompter.Reader)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read source branch push choice: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return SourceBranchPushDecisionPush, nil
	case "q", "n", "no", "abort":
		return SourceBranchPushDecisionAbort, nil
	default:
		return "", fmt.Errorf("source branch push choice must be enter, yes, or q")
	}
}

func formatSourceBranchPushPrompt(prompt SourceBranchPushPrompt) string {
	sourceRef := prompt.SourceBranch
	if prompt.Remote != "" {
		sourceRef = prompt.Remote + "/" + prompt.SourceBranch
	}

	lines := make([]string, 0, 8)
	switch prompt.Status {
	case SourceBranchStatusMissing:
		lines = append(lines, fmt.Sprintf("%s does not exist.", sourceRef))
	case SourceBranchStatusAhead:
		lines = append(lines, fmt.Sprintf("%s has commits not on %s.", prompt.SourceBranch, sourceRef))
	default:
		lines = append(lines, fmt.Sprintf("%s is not ready on %s.", prompt.SourceBranch, prompt.Remote))
	}
	lines = append(lines,
		"",
		"Publish can push the source branch before the package branch.",
		"",
		"Continue?",
		fmt.Sprintf("  [Enter] push %s, then publish package", prompt.SourceBranch),
		"  q       abort",
		"> ",
	)
	return strings.Join(lines, "\n")
}
