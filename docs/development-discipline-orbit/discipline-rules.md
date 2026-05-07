# Discipline Rules

Development Discipline Orbit provides three development feedback disciplines for the target codebase.

## Purpose

- `diagnose`: for bugs, broken behavior, failures, performance regressions, and problems with unclear root cause.
- `tdd`: for new behavior, behavior changes, test-driven development, and red-green-refactor.
- `review-commit`: for reviewing current worktree changes, fixing blockers, recording non-blocking technical debt, and creating a commit.

## Skill Choice

- Choose `diagnose` when the current goal is to confirm the real failure mode, find the root cause, or improve reproducibility.
- Choose `tdd` when the current goal is to deliver an observable behavior change.
- Choose `review-commit` when the current goal is to review and commit already-completed local code changes.
- When one task includes both failure investigation and behavior change, choose `diagnose` first, then `tdd`.
- When one task needs a commit after implementation is complete, choose `review-commit` last.

## Project Sources

Discover and read existing target-repository sources that are relevant to the current task as needed. These are candidate sources, not a required directory structure:

- project memory, such as `CONTEXT.md`, context files pointed to by `CONTEXT-MAP.md`, or other repository-declared domain language files.
- design decisions, such as existing ADR directories, architecture decision documents, or other repository-declared decision records.
- task context, such as provided issue body, task description, acceptance criteria, human clarifications, or conversation context.
- repository conventions, such as `AGENTS.md`, `CLAUDE.md`, commit conventions, validation commands, or formatting rules.
- debt sinks, such as Debt Notes in an issue body, section mappings declared by the tracker contract, or repository-declared technical debt recording locations.

Missing any candidate source is not a failure; continue using readable code, tests, and task context.

## Feedback Evidence

When complete, report:

- The feedback loop used.
- How the failure signal was observed.
- How the passing signal was verified.
- For `review-commit`, state the reviewed commit scope, validation, commit hash, and technical debt recording location; if no suitable location exists, list the debt in the completion report.
- Evidence still missing or context still needed from a human.
