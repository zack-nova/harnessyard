# ADR Rules

ADRs record project decisions that are hard to reverse, non-obvious, and involve real tradeoffs.

## Locations

- System-level ADRs: `docs/adr/`
- Context-level ADRs: `<context>/docs/adr/`

Create the directory only after the first ADR is confirmed.

## Reading And Use Rules

Before exploring code, designing an approach, splitting work, debugging a complex problem, or proposing architecture changes, read ADRs relevant to the current task.

- In single-context repositories, read relevant ADRs under root `docs/adr/`.
- In multi-context repositories, read relevant system-level ADRs under root `docs/adr/` first, then read ADRs under the relevant contexts.
- If the ADR directory or relevant ADR is missing, continue silently and only state that no relevant record was found; do not interpret ADR absence as "there are no constraints."
- Do not relitigate tradeoffs already confirmed by accepted ADRs.
- If there is a real reason to challenge an old ADR, clearly state the conflict and ask a human to decide.

## File Names And Titles

File name:

```text
docs/adr/0001-short-slug.md
```

Title:

```md
# 0001 Short Decision Title
```

The number is one greater than the current maximum number in the target ADR directory.

## Format

```md
---
status: accepted
---

# 0001 Short Decision Title

1-3 sentences explaining the context, the decision, and the rationale.
```

Include optional sections only when they provide real value:

```md
## Considered Options

## Consequences

## Supersedes
```

## Status

Allowed statuses:

- `proposed`
- `accepted`
- `deprecated`
- `superseded`

The default status is `accepted`.

## When To Create An ADR

Suggest creating an ADR only when all three conditions hold:

- Hard to reverse: changing the decision later would have meaningful cost.
- Non-obvious: future readers will want to know why this path was chosen.
- Real tradeoff: reasonable alternatives existed and a choice was made.

If it is only a temporary preference, an obvious implementation choice, or an easily reversible detail, do not write an ADR.

## Conflict Handling

If a new suggestion or decision conflicts with an existing ADR:

- Clearly state the conflicting ADR number and title.
- Explain the conflict.
- Wait for a human to decide whether to keep the old ADR, update it, deprecate it, or add a superseding ADR.
