# Harness Yard Concepts

Harness Yard turns ordinary Git repositories and authored package truth into
reusable harness runtimes and templates for agent-assisted work.

This page explains the product model used by the rest of the documentation. It
does not teach command workflows or internal YAML schemas.

## The Short Model

Harness Yard keeps the working model Git-native:

- one repository
- one working tree
- normal Git commits as history
- `.harness/*` as versioned Harness Yard truth
- `.git/orbit/state/*` as repo-local runtime state and cache
- root guidance files as editable presentation for humans and agents

The public CLI is `hyard`. Product documentation teaches the user-facing
`hyard` command surface.

## Revisions

A Harness Yard revision is a Git worktree revision with exactly one revision
kind.

| Revision | Purpose | Typical user |
| --- | --- | --- |
| Harness Runtime | Install, run, inspect, start agents, and publish a composed harness. | Runtime user, Harness author |
| Source Revision | Author editable truth for one Orbit Package. | Orbit author |
| Orbit Template Revision | Installable output for one Orbit Package. | Orbit author, Runtime user |
| Harness Template | Installable output for a composed Harness Package. | Harness author, Runtime user |

Multi-orbit composition belongs in a Harness Runtime or Harness Package. Source
Revisions and Orbit Template Revisions describe exactly one Orbit Package.

## Packages And Workflows

An Orbit Package is the reusable package boundary for one orbit's authored truth
and projected agent assets. When the documentation emphasizes how people use it,
that same package may be called an Orbit Workflow: one atomic, closed-loop
workflow with a bounded purpose, operating rules, expected feedback, and a way
to return results or ask for human input.

A Harness Runtime or Harness Package can combine multiple Orbit Workflows into
one agent work system.

Package Identity is the stable user-facing identity of an Orbit Package or
Harness Package. It is made from package type, package name, and optional version
when present. Display names and descriptions are not stable identity.

Package Handles, when available through a registry, are short names that resolve
to installable package locators. Namespaced handles such as `acme/docs`,
`acme/docs@latest`, and `acme/docs@0.1.0`, plus curated bare handles such as
`docs`, can be installed through `hyard install`; examples may still use
explicit Git locators when no registry entry is available.

## Views

Run View is the default runtime-user presentation. It keeps authoring scaffolds
out of the ordinary working tree and presents root guidance as runtime-facing
text.

Author View is for Harness authors and Orbit authors who are editing authored
truth, marked guidance, member hints, or package content that should be saved
back to package truth.

Runtime View selection changes presentation and publication defaults. It does
not change package identity or the canonical authored truth.

## Root Guidance

Root guidance artifacts are repository-root maintained guidance files:

```text
AGENTS.md
HUMANS.md
BOOTSTRAP.md
```

They are user-visible containers, not internal schema. `AGENTS.md` is the agent
work entry point. `HUMANS.md` is optional human orientation. `BOOTSTRAP.md` is
only for pending initialization guidance.

When Author View materializes root guidance with owner identity, blocks use
strict HTML markers:

```html
<!-- orbit:begin workflow="docs" -->
<!-- orbit:end workflow="docs" -->

<!-- harness:begin workflow="workspace" -->
<!-- harness:end workflow="workspace" -->
```

The `workflow` value is marker syntax for the owning package name. It is not a
display alias or separate Package Identity.

## Package Lifecycle

Runtime users install and uninstall packages in a Harness Runtime:

```bash
hyard install <package-source>
hyard uninstall orbit <orbit-package>
hyard uninstall harness <harness-package>
```

Packages may declare Package Variables. Zero variables is a complete variable
contract, not a missing one. When variables are required, users usually provide
bindings through `.harness/vars.yaml` or an explicit `--bindings` file.

Sharing a composed runtime uses Harness Package publication:

```bash
hyard publish harness <harness-package>
```

Sharing reusable orbit authored truth uses Orbit Package publication:

```bash
hyard publish orbit <orbit-package>
```

## Agent Handoff

Harness Start is the high-level handoff that turns an installed Harness Runtime
into an initialized interactive agent session:

```bash
hyard start --with codex
```

Harness Start selects or confirms an Agent Framework, applies project-local
agent assets, prepares bootstrap guidance when needed, and delivers a Start
Prompt to the selected framework. It does not replace package installation or
create Git commits.

## Where To Go Next

- Use [Quickstart](./quickstart.md) to run a Harness Runtime for the first time.
- Use [Configuration Reference](./reference/configuration.md) to understand
  program-readable truth and validation.
- Use [Content And Workflows](./guides/content-and-workflows.md) to write
  maintained content and Orbit Workflows.
- Use [Harness Authoring](./guides/harness-authoring.md) to compose and publish
  a reusable Harness Package.
- Use [Orbit Authoring](./guides/orbit-authoring.md) to author and publish one
  Orbit Package.
