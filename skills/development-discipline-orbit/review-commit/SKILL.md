---
name: review-commit
description: Review current worktree changes, fix blockers, record non-blocking technical debt, and create a git commit. Use when asked to review and commit, prepare a commit, finalize local changes, or run a pre-commit review.
---

# Review Commit

The process below is the `review-commit` workflow under the active development-discipline rules.

Before running, apply any available orbit or repository discipline rules.

Use this skill when code changes are believed ready for commit and need one last local review pass. This is an explicitly invoked skill, not a Git pre-commit hook. It may create a commit, but it does not push, create PRs/MRs, create review artifacts, write Review Sweeps, change issue state, or replace human review decisions.

## Inputs

- Session or task context: what was meant to change and why.
- Git state: `git status`, `git diff`, `git diff --staged`, and untracked files.
- Repo conventions: `AGENTS.md`, `CLAUDE.md`, commit guidelines, validation commands, and formatting rules when present.
- Project sources from the discipline rules, including project memory, ADRs, issue body content, and debt sinks when present and relevant.

## Workflow

### 1. Establish Commit Scope

Default to all task-related uncommitted worktree changes, including staged and unstaged changes.

Audit the scope before staging:

- Confirm the intended task from session context or issue context.
- Inspect staged, unstaged, and untracked changes.
- Exclude unrelated edits, generated artifacts, logs, local scratch files, and accidental secrets.
- If unrelated changes are already staged, unstage or restage only the intended files when safe.
- If safe separation is unclear, stop and ask the user.

If there are no task-related changes, stop without committing.

### 2. Review The Diff

Keep the review lightweight but real. Check the changed behavior against the task and the repository's conventions.

Review for:

- Correctness: broken logic, edge cases, error paths, stale assumptions.
- Verification: missing or weak tests, validation not run, brittle assertions.
- Maintainability: avoidable complexity, unclear naming, dead code, poor boundaries.
- Safety: secrets, shell injection, auth/permission drift, data loss, unsafe migrations.
- Performance: obvious repeated work, expensive queries, avoidable synchronous blocking.
- Scope: work that belongs in another issue or commit.

Read outside the diff when the diff introduces a new enum value, state, type, public API, storage shape, security boundary, or cross-module contract. Do not claim something is safe, covered, or handled elsewhere unless you verified the relevant code or tests.

### 3. Classify Findings

Classify every meaningful finding:

- **Blocker**: must be fixed before commit. Examples: real defect, broken validation, obvious security/data risk, accidental unrelated file, or scope creep.
- **Debt**: non-blocking technical debt discovered during the review. It should be recorded, not pulled into the current commit unless fixing it is clearly inside scope.
- **Note**: useful context for the final report that does not require action.

Fix blockers first when the fix stays inside the current task scope. After fixing, rerun the relevant validation and re-review the touched diff. Stop instead of committing when a blocker needs product judgment, a behavior change, or a large follow-up change.

### 4. Record Debt

Look for the best available repository-declared debt sink. This is soft compatibility, not a dependency.

Prefer, in order:

1. An issue-scoped Debt Notes section declared by a readable tracker contract or issue body.
2. Another repository-declared technical debt note location or template.
3. The final completion report when no suitable writable place exists.

When using a tracker contract, read its machine-friendly mapping first and follow the declared Debt Notes storage and heading instead of guessing a backend convention.

When writing Debt Notes, use this shape:

```md
- **Observation:**
  - <what was found>
  **Impact:**
  - <why it matters>
  **Follow-up issue:** none
```

Do not create follow-up issues unless explicitly asked. If the debt blocks or changes the current delivery, treat it as a blocker or review evidence, not merely Debt Notes.

### 5. Validate

Run the narrowest validation that gives confidence in the commit:

- repository-required validation commands when documented,
- tests or checks tied to the changed behavior,
- formatting/lint/typecheck when relevant to the changed files.

If validation cannot run, record why. Do not commit over a failing validation unless the user explicitly accepts the risk.

### 6. Commit

Stage only the intended scope, including any in-scope fixes and debt-note updates. Re-check `git diff --staged` before committing.

Write a commit message that matches the staged diff:

- conventional type and optional scope when the repo uses them,
- imperative subject, 72 characters or fewer, no trailing period,
- body with summary, rationale, validation, and debt notes recorded or deferred,
- repository-required trailers.

Create exactly one commit for the reviewed scope. After committing, report the commit hash, review outcome, validation run, blockers fixed or remaining, and where debt was recorded.
