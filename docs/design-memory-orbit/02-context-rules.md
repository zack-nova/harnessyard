# Context Rules

`CONTEXT.md` records project language and context boundaries. It is written for domain experts, not for concrete code structure.

## Default Layout

Single-context repositories use this default layout:

```text
CONTEXT.md
docs/adr/
```

Multi-context repositories declare contexts through root `CONTEXT-MAP.md`, for example:

```text
CONTEXT-MAP.md
docs/adr/
src/ordering/CONTEXT.md
src/ordering/docs/adr/
src/billing/CONTEXT.md
src/billing/docs/adr/
```

Root `docs/adr/` records system-level decisions. `docs/adr/` inside a context directory records context-level decisions.

## Reading Rules

Before exploring code, designing an approach, splitting work, debugging a complex problem, or proposing architecture changes, read the project context relevant to the current task.

- In single-context repositories, read root `CONTEXT.md` if it exists.
- In multi-context repositories, read root `CONTEXT-MAP.md` first, determine which contexts the current work touches, then read the relevant context `CONTEXT.md` files.
- If `CONTEXT-MAP.md` points to a missing context file, report the missing file and continue with the readable parts.
- If `CONTEXT.md` or `CONTEXT-MAP.md` is missing, continue silently and do not treat the absence as an error.

## Language Use

- Domain concepts in output should use terms already defined in `CONTEXT.md`.
- If a user's wording conflicts with `CONTEXT.md`, point it out immediately and ask a human to confirm.
- If a needed new concept is not in `CONTEXT.md`, clarify it in conversation before deciding whether to write it.

## CONTEXT.md format

Use the full context structure:

```md
# {Context Name}

{1-2 sentences explaining what this context is and why it exists.}

## Language

**Term**:
A one-sentence definition for domain experts. Define what it is, not implementation details.
_Avoid_: synonym, overloaded word

## Relationships

- **Term A** produces one or more **Term B**.
- **Term B** belongs to exactly one **Term C**.

## Boundaries

- **Context A** owns ...
- **Context B** references ...

## Example Dialogue

> **Dev:** "When does **Term A** create **Term B**?"
> **Domain expert:** "Only after **Term C** is confirmed."

## Flagged Ambiguities

- "account" was used to mean both **Customer** and **User**. Resolution: these are distinct concepts.

## Open Questions

- ...
```

## Writing Rules

- After a term is confirmed by a human, write it to `Language` immediately.
- Write synonyms, old terms, and easily misused words under `_Avoid_` for the corresponding term.
- Write relationships, dependencies, and cardinality between domain concepts under `Relationships`.
- Write context ownership, references, and responsibility splits under `Boundaries`.
- Write short dialogue that shows how terms naturally work together under `Example Dialogue`.
- Write clarified ambiguity and conflicting wording under `Flagged Ambiguities`.
- Write unconfirmed, ambiguous, or still-pending content under `Open Questions`.
- Do not treat class names, function names, file names, or database table names as domain terms unless they are truly language used by domain experts.
- Keep definitions compact, preferably one sentence.
- Record only concepts specific to this context; general programming concepts do not belong in `CONTEXT.md`.

## Creation Timing

- When `CONTEXT.md` does not exist, do not proactively create it.
- Create `CONTEXT.md` only when the first term, relationship, boundary, example dialogue, ambiguity, or open question is confirmed as needing a record.
- When `CONTEXT-MAP.md` does not exist, treat the repository as single-context.
- Create or update `CONTEXT-MAP.md` only when a human explicitly decides to split multiple contexts.
