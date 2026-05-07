# Design Memory Model

Design memory is the long-lived memory layer for design discussions. It preserves project language, boundaries, and decisions that agents and humans need to reference repeatedly.

## Memory Types

- `CONTEXT.md`: official project domain language, context boundaries, and open questions.
- `CONTEXT-MAP.md`: context index for a multi-context repository.
- `docs/adr/*.md`: confirmed architecture, boundaries, constraints, and important tradeoffs.
- `<context>/CONTEXT.md`: context-level domain language.
- `<context>/docs/adr/*.md`: context-level decision records.

## Responsibilities

- Let agents read relevant terminology and historical decisions before working.
- Clarify ambiguous language during discussion and preserve official terms.
- Record hard-to-reverse, non-obvious decisions with real tradeoffs as ADRs.
- Clearly flag conflicts between new output and existing ADRs.

## Non-Responsibilities

- Does not manage issue trackers, metadata, review artifacts, or delivery state.
- Does not replace Issue Tracker Contract Orbit, manager orbits, or execution orbits.
- Does not wrap code implementation details as domain terms.
- Does not create empty memory files to appear complete.
