---
name: to-prd
description: Turn the current conversation context into a PRD and publish it to the project issue tracker when repository publishing rules are clear.
---

# To PRD

This is the `to-prd` workflow. Before running it, read the repository's issue discovery rules, for example `docs/issue-discovery-orbit/discovery-rules.md` when present.

This skill takes the current conversation context and codebase understanding and produces a PRD. Do NOT interview the user - just synthesize what you already know.

If repository publishing rules are clear, publish the PRD to the issue tracker. If they are missing or conflicting, output a PRD candidate instead and list what must be confirmed before publishing.

## Process

1. Explore the repo to understand the current state of the codebase, if you haven't already. Use the project's domain glossary vocabulary throughout the PRD, and respect any ADRs in the area you're touching.

2. Sketch out the major modules you will need to build or modify to complete the implementation. Actively look for opportunities to extract deep modules that can be tested in isolation.

A deep module is one which encapsulates a lot of functionality in a simple, testable interface which rarely changes.

Check with the user that these modules match their expectations. Check with the user which modules they want tests written for.

3. Write the PRD using the template below, then publish it to the issue tracker if repository publishing rules are clear. Apply the repository's state that best matches the next needed step.

If child implementation issues are already known, include them in the PRD issue body under `## Blocked by`, record the intended resume state, and use the repository's blocked state instead of the entry state while those children remain unresolved. Do not publish a PRD with both the blocked state and the entry state. If child issues will be published later, keep the section with the placeholder text and treat updating it as part of the issue-slicing handoff.

If publishing rules are missing or conflicting, do not publish. Present the PRD as a candidate and list the missing or conflicting rules.

<prd-template>

## Problem Statement

The problem that the user is facing, from the user's perspective.

## Solution

The solution to the problem, from the user's perspective.

## User Stories

A LONG, numbered list of user stories. Each user story should be in the format of:

1. As an <actor>, I want a <feature>, so that <benefit>

<user-story-example>
1. As a mobile bank customer, I want to see balance on my accounts, so that I can make better informed decisions about my spending
</user-story-example>

This list of user stories should be extremely extensive and cover all aspects of the feature.

## Implementation Decisions

A list of implementation decisions that were made. This can include:

- The modules that will be built/modified
- The interfaces of those modules that will be modified
- Technical clarifications from the developer
- Architectural decisions
- Schema changes
- API contracts
- Specific interactions

Do NOT include specific file paths or code snippets. They may end up being outdated very quickly.

## Testing Decisions

A list of testing decisions that were made. Include:

- A description of what makes a good test
- Which modules will be tested
- Prior art for the tests

## Out of Scope

A description of the things that are out of scope for this PRD.

## Further Notes

Any further notes about the feature.

## Blocked by

References to child issues that must complete before the PRD can advance, if any.
If child issues have not been published yet, write "None - child issues not published yet".
When blockers are listed, also record the intended resume state.

</prd-template>
