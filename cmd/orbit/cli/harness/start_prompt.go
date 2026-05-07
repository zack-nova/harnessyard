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
	builder.WriteString("1. Check whether `BOOTSTRAP.md` exists in the runtime root.\n")
	builder.WriteString("2. If `BOOTSTRAP.md` does not exist, do not render guidance automatically. Say that no bootstrap guide was found; initialization may be unnecessary, or an authoring or recovery render step may be needed before bootstrap can continue.\n")
	builder.WriteString("3. If `BOOTSTRAP.md` exists, read it and perform the initialization work it describes.\n")
	builder.WriteString("4. Only after executing `BOOTSTRAP.md`, run `hyard bootstrap complete --check --json` and inspect the proposed removals.\n")
	builder.WriteString("5. Run `hyard bootstrap complete --yes` only when the preview removes expected bootstrap-lane runtime artifacts, expected root `BOOTSTRAP.md`, or expected bootstrap blocks. A retained plain `BOOTSTRAP.md` is not a failure.\n\n")
	builder.WriteString("Then introduce this Harness Runtime in the same session.\n\n")
	builder.WriteString("Explain what Harness Yard found, name the available harness workflows when useful, and ask what the user wants to do next.\n")

	return builder.String()
}
