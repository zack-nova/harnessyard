# Discovery Rules

Issue Discovery Orbit turns discussions, PRDs, plans, or repository understanding into issue tracker work items.

## Purpose

- `to-prd`: generate a PRD from the current context and repository understanding.
- `to-issues`: split a plan or PRD into vertical-slice issues.
- Publish when publishing rules are clear; output candidates only when rules are missing or conflicting.

## Outputs

**PRD**:
A product requirements document that can be published as a tracker issue.

**Slice plan**:
A human confirmation plan before issues are published.

**Issue candidate**:
A candidate work item that has not yet been published to the issue tracker; it is not an Issue and not a fact source.

**Issue**:
A work item returned by the tracker; this is an issue tracker fact.

## Before Publishing

Read the available Repository Publishing Rules in the target repository compatibly; these are candidate sources, not a required directory structure:

- General agent entry points or repository rule documents, such as `AGENTS.md`.
- Tracker contract documentation, such as Issue Tracker Contract conventions or repository-specific equivalents.
- Issue template configuration, such as GitHub `.github/ISSUE_TEMPLATE/` or other tracker templates.
- Other documents that clearly define the issue tracker, templates, labels, state entry points, or publishing permissions.

Also read available project memory compatibly; it is not publishing rules and not a required directory structure:

- Design Memory conventions, such as `CONTEXT.md`, `CONTEXT-MAP.md`, or `docs/adr/`.
- Repository-declared domain language, context maps, or design decision records.
- Relevant context already present in the conversation, issue body, or task description.

When publishing, use only the repository-defined issue templates, states, labels, assignees, milestones, and project fields. For `to-issues`, publish slices with no blockers in the repository's normal new-issue state. Publish slices with unresolved `Blocked by` dependencies in `blocked`, replacing any template default state rather than adding a second state label, and record both the blocker and intended resume state in the issue text.

## Candidate Fallback

Output candidates only, and list missing or conflicting rules, when:

- No issue tracker can be found.
- The required template or label rules cannot be found.
- Repository documents conflict with each other.
- Tracker permissions are unavailable.
- The user's requested publishing mode conflicts with repository rules.
