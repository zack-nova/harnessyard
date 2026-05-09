# Content And Workflows

This guide explains how maintained content files should be written, sized, split,
and organized into Orbit Workflows.

Use [Configuration Reference](../reference/configuration.md) for
program-readable files, editing policy, conformance checks, and validation
commands. This guide is about what the content is for and how much of it to
write.

## Content Vocabulary

An Orbit Workflow is one atomic, closed-loop workflow. It gives an agent a
bounded purpose, operating rules, expected feedback, and a controlled way to
return results or ask for human input.

A composed harness is either a Harness Runtime when it runs inside a Git
repository or a Harness Package when it is published for reuse. A Harness
Runtime or Harness Package may combine multiple Orbit Workflows into one agent
work system.

Maintained content is classified by responsibility:

| Content | Purpose | Typical location |
| --- | --- | --- |
| Guidance files | Rules, boundaries, workflow instructions, and handoff guidance. | Root guidance artifacts or linked docs. |
| Subject files | Product, domain, repository, or documentation content an orbit operates on or interprets. | Project docs or package content paths. |
| Process files | Checklists, templates, workpads, review notes, or other execution material. | Package process paths. |
| Local skill assets | Local agent skills and supporting resources owned by an Orbit Package. | `skills/<orbit-id>/*` |
| Prompt command assets | Prompt command templates owned by an Orbit Package. | `commands/<orbit-id>/**/*.md` |
| Bootstrap content | Initialization-only guidance or process content. | `BOOTSTRAP.md` or package bootstrap content. |

Typical locations are recommended positions, not semantic definitions. The
responsibility of the content decides how it should be written and reviewed.

## Authored Contract

Before writing files for an Orbit Workflow, answer these questions:

| Contract item | Question |
| --- | --- |
| `objective` | What class of work does this workflow complete? |
| `scope boundary` | What does it inspect, modify, and leave alone? |
| `rules` | Which decisions must remain stable? |
| `done probe` | What external signal proves the work is complete? |
| `failure condition` | When should the workflow clearly report failure? |
| `abnormal exit hint` | When should it stop spending effort and hand back to a human? |
| `record target` | Where are results, state, or evidence recorded? |
| `record minimum` | What minimum information lets the next consumer continue? |

These questions do not need to appear as literal headings in every file. They
are a check that the workflow has a complete runtime contract.

## Root Guidance Artifacts

Root guidance artifacts are maintained presentation files at the repository
root. They are user-visible guidance containers, not canonical package schema.

| File | Requiredness | Purpose | Recommendations |
| --- | --- | --- | --- |
| `AGENTS.md` | Required for a Harness Runtime. | Agent work entry point. | Prefer read order, hard boundaries, and run-state rules; keep it under 200 lines; link to deeper maintained content instead of embedding full package docs. |
| `BOOTSTRAP.md` | Conditional. | Pending initialization entry point. | Keep bootstrap-only guidance here; avoid turning it into a steady-state rule index. |
| `HUMANS.md` | Optional. | Human orientation for the current harness or template. | Summarize purpose, installed workflows, and human maintenance entry points. |

Root `AGENTS.md` and the maintained guidance it links to should assume the
runtime is already in its normal working state. Mention initialization only when
missing initialization would block or make unsafe the current run-state work.

An Orbit Workflow contributes a root guidance block to `AGENTS.md`; it does not
create a separate root `AGENTS.md` for that orbit. Each orbit block should stay
under 30 lines and link to deeper maintained content when it needs more detail.

## File Responsibility Guide

| File or directory | Primary reader | Owns | Suggested size | Should not carry |
| --- | --- | --- | --- | --- |
| `AGENTS.md` | Agent | Entry rules, read order, hard boundaries, fallback instructions | 5-20 lines for one orbit block; root file under 200 lines | Full manuals, skill workflows, history |
| `HUMANS.md` | Human | Orientation, when to use the harness or workflow, what humans must decide | 5-25 lines | Duplicated agent workflow |
| `README.md` | Package or repo reader | Product/package summary and documentation routing | 5-40 lines for package source | Runtime rules source |
| `BOOTSTRAP.md` | Installer or first-run agent/human | Initialization-only steps | Shortest executable setup | Normal steady-state workflow |
| `docs/<orbit>/...` | Agent and maintainer | Durable workflow rules, contracts, state machines, decision standards | 40-160 lines for a tiny workflow rule doc | Step-by-step skill procedure or source-only history |
| `docs/<orbit>/README.md` | Agent and maintainer | Initial rule directory entry point, especially from `--with-spec` authoring bootstrap | 5-30 lines | Member hint metadata, copied OrbitSpec fields |
| `docs/<orbit>/INDEX.md` | Agent and maintainer | Navigation when multiple rule documents are needed | 10-40 lines | Pass-through file list |
| `skills/<orbit-id>/*/SKILL.md` | Skill runtime | Trigger conditions, workflow steps, tool use, completion report | As complex as the skill requires | Overall product model or other skills' details |
| `commands/<orbit-id>/**/*.md` | Prompt command runtime | Callable prompt command templates | As command needs | Duplicated docs content |
| `docs/<orbit>/templates/**` | Bootstrap or runtime consumer | Templates that are installed, copied, merged, or filled | One consumable shape per template | Examples that no consumer reads |

## Runtime Surface

The runtime surface is the content installed into a target repository and read,
executed, or consumed there. It is not every file in a source authoring
repository.

For a Harness Runtime, expected content includes:

| Content | Requiredness | Purpose |
| --- | --- | --- |
| `AGENTS.md` | Required | Agent entry point for work in the runtime. |
| Orbit guidance blocks | Conditional | Introduce each installed Orbit Workflow's role, feedback loop, and control boundaries. |
| Harness guidance block | Conditional | Describe harness-level composition and cross-orbit boundaries. |
| `BOOTSTRAP.md` | Conditional | Initialize the runtime when pending bootstrap work exists. |
| `HUMANS.md` | Optional | Explain the runtime to a human user or maintainer. |
| Supporting guidance, subject, and process files | Conditional | Hold detailed rules, process material, or subject content that installed workflows need. |
| Local skill and prompt command assets | Conditional | Provide package-owned agent capabilities. |

Run View is the default runtime-user presentation. Author View is the expected
mode when a Harness author or Orbit author edits marked guidance, content hints,
or package-authored truth inside a runtime.

## Source And Template Content

An Orbit Template Revision installs one Orbit Package into a Harness Runtime. It
does not own harness-level composition.

| Content | Requiredness | Purpose |
| --- | --- | --- |
| Installable orbit guidance | Required | Produce the orbit block and linked guidance used in a runtime. |
| Subject and process content | Conditional | Provide files the orbit needs at runtime. |
| Local skill assets | Conditional | Provide local skills to activate in the runtime. |
| Prompt command assets | Conditional | Provide command templates to expose in the runtime. |
| Bootstrap content | Conditional | Provide first-use initialization for the installed orbit. |

A Source Revision is the authoring revision for one Orbit Package. It is where
an Orbit author writes reusable orbit content before publication.

| Content | Requiredness | Purpose |
| --- | --- | --- |
| Orbit guidance files | Required | Define the workflow purpose, feedback loop, control points, and boundaries. |
| Subject files | Conditional | Provide source material the workflow operates on or interprets. |
| Process files | Conditional | Provide execution templates, workpads, review procedures, or other reusable process material. |
| Local skill assets | Conditional | Package local agent skills with the orbit. |
| Prompt command assets | Conditional | Package command prompts with the orbit. |
| Bootstrap content | Conditional | Initialize the orbit after installation. |
| Member hints | Conditional | Propose package member truth from Markdown frontmatter or directory markers. |

Orbit Template Revisions and Source Revisions each describe exactly one Orbit
Package. Multi-orbit composition belongs in a Harness Runtime or Harness
Package.

## Content Size And Splitting

Content size is driven by consumer decisions, not page count. If content cannot
change what an agent, human, bootstrap flow, publish flow, or maintainer decides,
it should not enter the runtime surface.

Keep one rule document while the content still has:

- one objective
- one done probe
- one record target
- one primary runtime reader set
- differences that are only examples, local steps, or conditional branches

Split into child documents when one of these is true:

- different readers need independent entry points
- independent rule clusters or state machines can evolve separately
- one document has become hard to scan for fixed rules
- templates or generated contracts have real runtime consumers

Split an Orbit Workflow only when the authored contract has split:

- objective changed into two classes of work
- done probe or record target is no longer shared
- one part can fail without blocking the other
- abnormal exit logic is meaningfully different
- primary consumers or handoff chains differ

If only tools, backend mappings, or local steps differ, keep one workflow first
and use adapters, rule sections, or skill division.

## Source-Only Material

Source authoring repositories may contain author notes, experiments, old
reference material, migration notes, and release notes. These are source-only
unless they are installed into the runtime surface or read by runtime consumers.

Keep source-only material when it:

- helps authors maintain or publish the package
- records background not stable enough for the runtime contract
- preserves old reference material without presenting it as current rules

If source-only material starts affecting runtime decisions, promote it to a
clear runtime surface file. If it only explains history, keep it outside the
runtime surface or delete it.

## Common Failure Signs

- `AGENTS.md` and a linked rule document define the same rule.
- `HUMANS.md` repeats the README or skill workflow.
- A standalone safety document only says what not to do and has no independent
  fact source.
- An `INDEX.md` is only a pass-through file list.
- A template has no bootstrap, install, or runtime consumer.
- A rule document mostly explains history instead of current judgment.
- README is treated as an agent runtime rule source.
- Source-only experiments are treated as published contract.

## Record Minimums

Records should be enough for the next consumer to continue, not a full internal
thought log.

- Success records need result evidence and necessary state.
- Failure records need the failed condition and why it could not be satisfied.
- Abnormal exits need attempted actions, current blocker, and suggested next
  step.
- External stops need current state and a handoff point.

Do not require full command output, full process logs, or agent-internal plans
unless a later consumer actually reads them to decide what to do.
