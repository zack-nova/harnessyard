# Issue Tracker Contract Orbit Documentation Index

After this orbit is installed into a code repository, it defines that repository's issue tracker contract.

## Runtime Entry Point

`tracker-contract.md` is the fixed read entry point for **Contract Consumers**.

A **Contract Consumer** is any external agent, runtime participant, or tool that must read and follow this repository's issue tracker contract.

If `tracker-contract.md` is still `pending-bootstrap`, run `BOOTSTRAP.md` first. During normal operation, do not infer the current repository's issue tracker from adapter templates.

## Core

`core/` defines the platform-independent contract language:

1. `core/01-core-model.md`: boundaries for issue tracker, backend, state, type, metadata, section, and review artifact.
2. `core/02-state-machine.md`: canonical state roles and gates.
3. `core/03-issue-sections.md`: issue sections such as Dev Brief, Dev Workpad, Debt Notes, Review Sweep, and Human Review Decision.
4. `core/04-safety-rules.md`: dry-run, hard stops, and prohibited actions.

## Adapters

`adapters/` defines mapping templates from core to concrete backends:

- `adapters/github.md`
- `adapters/gitlab.md`
- `adapters/local-markdown.md`
- `adapters/other.md`

Bootstrap selects one backend adapter and generates the current repository's `tracker-contract.md`.

## Templates

`templates/` stores reference templates for backends and issue sections. Backend initialization templates live under `templates/backends/<backend>/`. Bootstrap may install, merge, or copy these templates as needed, but normal operation is governed by `tracker-contract.md`.
