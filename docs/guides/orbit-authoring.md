# Orbit Authoring

Use this guide when you want to author one reusable Orbit Workflow and publish
it as an Orbit Package.

If you want to compose multiple workflows into one reusable working environment,
use [Harness Authoring](./harness-authoring.md).

## Goal

An Orbit author defines one atomic workflow, writes its maintained content and
agent assets, validates authored truth, and publishes an Orbit Package:

```bash
hyard publish orbit <orbit-package>
```

Source Revisions and Orbit Template Revisions describe exactly one Orbit
Package. Multi-orbit composition belongs in a Harness Runtime or Harness
Package.

## Start With The Workflow Contract

Before writing files, define the workflow:

- objective: what class of work does it complete?
- scope boundary: what can it inspect, modify, and leave alone?
- rules: what decisions must stay stable?
- done probe: what external signal proves completion?
- failure condition: when should it report failure?
- abnormal exit hint: when should it stop and hand back?
- record target: where do results or evidence go?
- record minimum: what is the least useful handoff record?

Use [Content And Workflows](./content-and-workflows.md) for file sizing,
runtime surface boundaries, and splitting rules.

## Choose An Authoring Shape

Use a Source Revision when the orbit needs long-lived authoring work:

- multiple runtime skills
- docs, templates, bootstrap, or adapter material
- source-only notes, old references, experiments, or release notes
- conversion from a multi-orbit repository into independently publishable source

Create a standalone source revision:

```bash
hyard create source docs-source --orbit docs --name "Docs Orbit" --description "Docs authoring repo"
cd docs-source
```

Add `--with-spec` when the new orbit should start with a maintained rule entry
point:

```bash
hyard create source docs-source --orbit docs --with-spec --name "Docs Orbit" --description "Docs authoring repo"
```

The same flag is available on source and orbit-template create/init bootstrap
paths. It creates both `docs/<orbit-id>.md` and
`docs/<orbit-id>/README.md`; the generated `spec` rule member includes
`docs/<orbit-id>.md` and `docs/<orbit-id>/**`. The README is a minimal rule
entry point and should not carry `orbit_member` hint metadata.

Or convert existing authored content:

```bash
hyard adopt source --orbit docs
```

Small stable orbits may be maintained closer to template shape, but current
authoring should still validate the same package truth before publication.

## Minimum Source Shape

A useful source revision usually has:

```text
AGENTS.md
HUMANS.md
README.md
docs/<orbit-id>/...
skills/<orbit-id>/<skill>/SKILL.md
```

Add only when the workflow needs them:

```text
BOOTSTRAP.md
docs/<orbit-id>/templates/...
commands/<orbit-id>/**/*.md
```

After source adoption or source creation, authored truth lives in:

```text
.harness/manifest.yaml
.harness/orbits/<orbit-id>.yaml
```

Do not replace these with old copied-orbit metadata.

## Guidance And Assets

Use root guidance files as package templates and runtime entry points:

- `AGENTS.md` gives the agent entry rule, read order, and key fallback.
- `HUMANS.md` gives a human quick guide.
- `BOOTSTRAP.md` exists only when first-use initialization is needed.
- `README.md` summarizes source/package purpose and routes readers.

Write the orbit's `AGENTS.md` guidance as a thin runtime entry point, not as a
mandatory full-doc reading list. Start with a direct purpose title and a
user-facing purpose sentence, such as "Issue Triage Agent Notes" or "Release
Review Agent Notes", instead of a package ID, install instruction, or title that
only says the file is an `AGENTS.md`.

Useful runtime sections are the parts that change what an agent does for the
user during a run. A good orbit agents guide has:

- always-on boundaries: product identity, state model, and dangerous actions
  that are unsafe to miss on any run
- task-triggered reading: deeper docs grouped by the work that needs them,
  such as implementation, testing, release surface, issue flow, or design memory
- workflow entry points: skill or process triggers that point to the owning
  docs instead of copying the full state machine into `AGENTS.md`
- validation and reporting expectations: the smallest evidence the agent must
  record before handing back

Prefer "read this when the task touches X" over "read every file before every
task." Keep detailed procedures in linked docs, local skills, prompt commands,
or `BOOTSTRAP.md` when the procedure is initialization-only.

Avoid boilerplate that the package structure already communicates. Do not spend
the orbit `AGENTS.md` explaining how to install the package, restating its
package ID as the heading, or giving self-referential instructions such as
"this file tells agents what to do" unless that sentence adds a concrete
runtime rule. Put publication, installation, and authoring background in
`README.md` or maintainer docs instead.

Use local skills when an agent needs a reusable runtime workflow. A skill file
owns trigger conditions, workflow steps, tool usage, and completion reporting.

Use prompt commands only when the orbit provides a callable prompt command
surface. Use docs when the content is durable rule, subject, or process material
that a workflow reads.

## Member Hints

Member hints are temporary authoring input for ordinary installable content.
They are not used for skills, commands, or root guidance files.

Directory marker:

```yaml
# docs/docs/.orbit-member.yaml
orbit_member:
    name: docs-rules
    role: rule
```

Markdown frontmatter:

```yaml
---
orbit_member:
    name: review-process
    description: Review process
    role: process
---
```

Apply tracked content hints before publication:

```bash
hyard orbit content apply docs --check --json
hyard orbit content apply docs
```

See [Configuration Reference](../reference/configuration.md) for the supported
hint fields and validation commands.

## Authoring In A Runtime

When authoring inside a Harness Runtime, select Author View before materializing
editable guide artifacts:

```bash
hyard view author
```

Render and save guidance:

```bash
hyard guide render --orbit docs --target all
hyard guide save --orbit docs --target all
```

Return to Run View when the runtime should be consumed or published as a
runtime-facing surface.

## Validate

Run focused checks before publishing:

```bash
hyard orbit validate
hyard orbit content apply docs --check --json
hyard orbit prepare docs --check --json
hyard audit
```

Use the package name when the repository contains more than one candidate or
when clarity matters.

## Publish

Publish the authored orbit as an Orbit Package:

```bash
hyard publish orbit docs --json
```

The user-facing publication path is `hyard publish orbit <package>`.

After publication, generate a reviewable Registry Entry Candidate for the
package handle you want users to install:

```bash
hyard registry entry orbit acme/docs@0.1.0 --source origin --ref orbit-template/docs --package docs
```

The candidate YAML is catalog-as-code input for the package registry review
flow. Use stdout for review or `--out <path>` for a chosen candidate artifact.
Use `--registry <path>` only when you want Harness Yard to merge the candidate
into a local registry checkout as the final `packages/<namespace>/index.yaml`
catalog index shape.

## Pre-Publish Checklist

- The orbit has one objective, one scope boundary, and one done probe.
- `AGENTS.md` is an entry point, not a full manual.
- `AGENTS.md` starts with a direct purpose title and purpose sentence, not a
  package ID or install-oriented heading.
- `AGENTS.md` uses task-triggered reading instead of making every run pre-read
  the whole orbit documentation set.
- `AGENTS.md` keeps runtime-value sections and removes boilerplate already
  implied by file location, package structure, or linked authoring docs.
- Only hard safety and product boundaries are always-on; detailed workflow
  rules live in linked docs, skills, commands, or bootstrap guidance.
- `HUMANS.md` is human orientation, not duplicated agent workflow.
- `README.md` is source/package documentation, not runtime rule truth.
- `BOOTSTRAP.md` exists only if there is real initialization state.
- Skills own runtime skill workflows.
- Commands own callable prompt command surfaces.
- Rule docs do not repeat skill procedures.
- Member hints have been checked or applied.
- Source-only old references and experiments are not exported as runtime truth.
- `hyard orbit prepare <package> --check --json` passes or every reported
  blocker has been resolved.

## Common Mistakes

- Using one Source Revision to compose multiple Orbit Packages.
- Publishing with uncommitted `.harness/*` truth.
- Treating markerless Run View guidance as authored Orbit Package truth.
- Turning `AGENTS.md` into a mandatory full documentation reading list.
- Titling orbit `AGENTS.md` guidance with only the package ID or file role
  instead of the direct runtime purpose.
- Filling orbit `AGENTS.md` with install boilerplate, publication instructions,
  or self-referential text that does not change runtime behavior.
- Putting capability-owned skill or command paths into ordinary content members.
- Keeping old authoring references as runtime rules.
- Adding templates that no bootstrap, install, or runtime consumer reads.
