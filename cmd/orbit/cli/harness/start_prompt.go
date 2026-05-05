package harness

import (
	"fmt"
	"strings"
)

// StartPromptInput describes the stable prompt handoff for Harness Start.
type StartPromptInput struct {
	RepoRoot string
}

// BuildStartPrompt returns the shared initial prompt used by Harness Start.
func BuildStartPrompt(input StartPromptInput) string {
	var builder strings.Builder

	builder.WriteString("# Harness Start\n\n")
	builder.WriteString("Start Prompt\n\n")
	if strings.TrimSpace(input.RepoRoot) != "" {
		fmt.Fprintf(&builder, "You are starting work in a Harness Runtime at `%s`.\n\n", input.RepoRoot)
	} else {
		builder.WriteString("You are starting work in a Harness Runtime.\n\n")
	}
	builder.WriteString("First handle any pending Harness Runtime bootstrap work.\n\n")
	builder.WriteString("1. From the runtime root, run `hyard guide render --target bootstrap --json`.\n")
	builder.WriteString("2. If no bootstrap guidance is rendered and `BOOTSTRAP.md` is absent, continue to the harness introduction.\n")
	builder.WriteString("3. If `BOOTSTRAP.md` exists, read it and perform the initialization work it describes.\n")
	builder.WriteString("4. Run `hyard bootstrap complete --check --json` and inspect the proposed removals.\n")
	builder.WriteString("5. Only when the preview removes expected bootstrap-lane runtime artifacts or root `BOOTSTRAP.md`, run `hyard bootstrap complete --yes`.\n\n")
	builder.WriteString("Then introduce this Harness Runtime in the same session.\n\n")
	builder.WriteString("Explain what Harness Yard found, name the available harness workflows when useful, and ask what the user wants to do next.\n")

	return builder.String()
}
