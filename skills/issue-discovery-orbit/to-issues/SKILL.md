---
name: to-issues
description: Turn a plan, spec, or PRD into one or more independently-grabbable issues, and publish them when repository publishing rules are clear.
---

# To Issues

This is the `to-issues` workflow. Before running it, read the repository's issue discovery rules, for example `docs/issue-discovery-orbit/discovery-rules.md` when present.

Turn a plan into one or more independently-grabbable issues using vertical slices (tracer bullets).

If repository publishing rules are clear, publish the approved issue or slices to the issue tracker. If they are missing or conflicting, output Issue candidates instead and list what must be confirmed before publishing.

## Process

### 1. Gather context

Work from whatever is already in the conversation context. If the user passes an issue reference, URL, or path as an argument, fetch it from the issue tracker and read its full body and comments when the tracker is accessible.

### 2. Explore the codebase

If you have not already explored the codebase, do so to understand the current state of the code. Issue titles and descriptions should use the project's domain glossary vocabulary, and respect ADRs in the area you're touching.

### 3. Draft vertical slices

Turn the plan into one or more **tracer bullet** issues. Each issue is a thin vertical slice that cuts through ALL necessary integration layers end-to-end, NOT a horizontal slice of one layer.

Slices may be HITL or AFK. HITL slices require human interaction, such as an architectural decision or a design review. AFK slices can be implemented and merged without human interaction. Prefer AFK over HITL where possible.

<vertical-slice-rules>
- Each slice delivers a narrow but COMPLETE path through every required layer.
- A completed slice is demoable or verifiable on its own.
- Choose issue boundaries so every resulting work item is independently useful and easy to verify.
</vertical-slice-rules>

### 4. Quiz the user

Present the proposed breakdown as a numbered list. For each slice, show:

- **Title**: short descriptive name
- **Type**: HITL / AFK
- **Blocked by**: which other slices must complete first, if any
- **User stories covered**: which user stories this addresses, if the source material has them

Ask the user:

- Does the granularity feel right?
- Are the dependency relationships correct?
- Should any slices be merged or split further?
- Are the correct slices marked as HITL and AFK?

Iterate until the user approves the breakdown.

### 5. Publish the issues to the issue tracker

For each approved slice, or the single approved issue, publish a new issue to the issue tracker only if Repository Publishing Rules are clear. Use the repository's issue body template and entry labels or states.

Publish issues in dependency order so you can reference real issue identifiers in the "Blocked by" field.

If publishing rules are missing or conflicting, do not publish. Present the approved breakdown as Issue candidates and list the missing or conflicting rules.

<issue-template>

## Parent

A reference to the parent issue on the issue tracker if the source was an existing issue. Otherwise omit this section.

## What to build

A concise description of this vertical slice. Describe the end-to-end behavior, not layer-by-layer implementation.

## Acceptance criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Blocked by

- A reference to the blocking issue, if any

Or "None - can start immediately" if no blockers.

</issue-template>

Do NOT close or modify any parent issue unless repository rules and the user both explicitly allow it.
