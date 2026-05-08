# HUMANS.md - Issue Tracker Contract Orbit

This guide is for day-to-day use by project maintainers. It is not an installation checklist or an internal orbit maintenance manual.

## What You Need To Know

- The issue workflow rules for this repository live in `docs/issue-tracker-orbit/tracker-contract.md`.
- Issues may live in GitHub, GitLab, local markdown, or another tracker; the backend contract decides the concrete location.
- **Contract Consumer** is the runtime participant identity that must read and follow the tracker contract first. Any runtime participant that joins the issue workflow must first act as a **Contract Consumer**.
- **Contract Consumer** identity does not itself grant permission to triage, develop, create review artifacts, rework, land, or merge; concrete authority comes from external orbits, runtime participants, tools, or human process.
- Human maintainers clarify requirements, approve waivers, resolve conflicts, and write final review decisions.

## When You Need To Participate

### Triage

When an issue lacks enough information, a **Contract Consumer** with triage responsibility keeps it in `needs-info` and records Triage Notes. You need to provide missing facts or decide whether to cancel, split, or downgrade it.

### Ready for Dev

Before an issue enters `ready-for-dev`, it must have a complete Dev Brief. You need to confirm:

- the requirement is clear,
- acceptance criteria are verifiable,
- the validation plan is realistically executable,
- out-of-scope boundaries are explicit,
- delivery mode is either absent, `afk`, or `hitl`,
- `hitl` has a rationale explaining why human interaction is required,
- the issue is not in `blocked`.

### Needs Split

When an issue is too large, move it to `needs-split` and record the split reason and intended decomposition. After child issues exist, record their references before moving the parent to `blocked` or another valid state.

### Blocked

Blocked is a state. It means the current issue is paused by a dependency, external factor, or human decision. Record the concrete blocker and intended resume state in issue text; after the block is resolved, move the issue out of `blocked` only when the target state's gates are satisfied.

### Cancelled And Out Of Scope

`cancelled` is the terminal non-delivery state. Use `resolution:duplicate` only with a superseding issue reference. Use `resolution:wontfix` for ordinary feature requests rejected as out of scope, and reference the matching Out-of-Scope Catalog entry.

### Debt Notes

When development or review finds technical debt that does not block the current delivery, create `## Debt Notes` as needed. It is not a hidden backlog and not a merge blocker. If the debt is later promoted to an independent issue, update the original entry's `Follow-up issue`; if the debt affects the current delivery, it must enter review evidence and be decided by Human Review Decision.

### Review

After an issue enters `in-review`, a review artifact exists. It is usually a PR/MR; the local markdown backend uses a local review artifact file to model the review gate. Review Sweep records observations and may identify objective AFK rework, objective AFK merge eligibility, or the need for `human-review`.

Review Sweep is observation only, not a decision.

### Human Review Decision

When an issue is `hitl` or enters `human-review`, a human maintainer needs to write Human Review Decision:

```text
Decision: hold
Decision: rework
Decision: merge
```

- `hold`: keep the issue in `human-review`.
- `rework`: allow the issue to enter `to-rework`, where a **Contract Consumer** with rework responsibility takes over.
- `merge`: allow the issue to enter `to-merge`, where a **Contract Consumer** with land responsibility performs Land; after Land succeeds, the issue enters `merged`.

Do not let review output replace Human Review Decision.

`Human Review Decision` is authorization evidence, not the operation trigger. Operations are triggered by the issue's canonical state role. Contract Consumers need their own claim/running/retry mechanism to avoid duplicate pickup; the tracker contract does not record these runtime occupancy facts.

After `to-rework` is completed, the issue must return to `in-review` and produce Review Sweep again. If `to-merge` execution fails, return a `hitl` issue or a human-dependent failure to `human-review`; return objective AFK failures to `to-rework`.

## Do Not Bypass Manually

- Do not develop directly on the default branch.
- Do not locally merge the default branch.
- Do not bypass the tracker contract when changing state.
- Do not treat external runtime sessions or worktree notes as fact sources.
- Do not ask any runtime environment to merge a `hitl` or `human-review` issue without Human Review Decision.

# HUMANS.md - Issue Discovery Orbit

This orbit is for people who want to turn ideas, PRDs, or plans into issue tracker work items.

Available skills:

- `to-prd`
- `to-issues`

Before publishing, confirm the target repository's Repository Publishing Rules: issue tracker, templates, labels, state entry points, and permissions.
Publish when rules are clear; when rules are missing or conflicting, generate candidates only for confirmation.

# HUMANS.md - Design Memory Orbit

This orbit is for code repositories. It helps agents remember project language, context boundaries, and architecture decisions confirmed during human-AI design discussions.

Common skills:

- `grill-with-docs`: use when you want to drive a design discussion toward clear terminology, boundaries, or ADR-worthy decisions.
- `grill-me`: use when you only want to be questioned about a plan and do not want project memory written.
- `zoom-out`: use when you want the agent to explain an unfamiliar code area one level higher.
- `improve-codebase-architecture`: use when you want to find opportunities for deeper modules, testability, or AI navigability.
- `caveman`: use when you want short, high-density communication.

Decisions where you need to participate:

- Whether a term is the project's official language.
- Whether a context owns a concept or data.
- Whether an architecture choice is worth recording as an ADR.
- Whether a new decision should overturn, deprecate, or replace an older ADR.

`CONTEXT.md`, `CONTEXT-MAP.md`, and `docs/adr/` are created only when there is confirmed content; empty files have no value.

# HUMANS.md - Development Discipline Orbit

This orbit is for people who want to keep small-step, verifiable development discipline in a code repository.

Available skills:

- `diagnose`
- `review-commit`
- `tdd`

Use `diagnose` for bugs, failures, performance regressions, or problems with unclear root cause.
Use `tdd` when developing new behavior, changing existing behavior, or following red-green-refactor.
Use `review-commit` after local changes are complete and you want to review changes, record non-blocking technical debt, and create a commit.
