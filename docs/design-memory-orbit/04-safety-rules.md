# Safety Rules

## Lazy create

- Do not create empty `CONTEXT.md`.
- Do not create empty `CONTEXT-MAP.md`.
- Do not create empty `docs/adr/`.
- Create files or directories only when confirmed content needs to be recorded.

## Uncertainty

- Write uncertain terms under `Open Questions`; do not pretend they are definitions.
- When context ownership is uncertain, ask a human first.
- When an ADR conflict is uncertain, mark it as a possible conflict and list the evidence.

## Writing Boundaries

- Do not modify files unrelated to design memory.
- Do not write delivery workflow, issue state, or review artifact strategy into this orbit unless they are themselves project architecture decisions.
- Do not record implementation details, temporary plans, or short-term preferences as long-term memory.

## Conflicts

Stop writing and ask a human to decide when:

- A new term conflicts with an existing `CONTEXT.md` definition.
- A new boundary conflicts with existing `Boundaries`.
- A new ADR conflicts with an accepted ADR.
- `CONTEXT-MAP.md` points to multiple conflicting context ownership claims.
