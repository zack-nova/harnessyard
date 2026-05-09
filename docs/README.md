# Documentation

This directory contains the product, contributor, reference, and maintainer
documentation for Harness Yard.

## Start Using Harness Yard

- [Installation](./installation.md): install `hyard` and verify the CLI.
- [Quickstart](./quickstart.md): initialize or clone a Harness Runtime, install
  packages, check readiness, and start an agent.
- [Concepts](./concepts.md): learn the product model before reading reference or
  authoring material.

## Author Reusable Work

- [Harness Authoring](./guides/harness-authoring.md): compose multiple Orbit
  Workflows into a reusable Harness Package.
- [Orbit Authoring](./guides/orbit-authoring.md): author and publish one Orbit
  Package.
- [Content And Workflows](./guides/content-and-workflows.md): organize guidance,
  subject, process, skill, command, and bootstrap content.

## Reference

- [Configuration Reference](./reference/configuration.md): understand
  program-readable truth, editing policy, conformance, and validation.
- [Release Surface](./reference/release-surface.md): public release contract for
  `hyard`.

## Contribute And Maintain

- [Testing for Contributors](./contributing/testing.md)
- [Current Architecture](./maintainers/current-architecture.md)
- [Unresolved Work](./maintainers/unresolved-work.md)
- [Release Guide](./maintainers/release.md)
- [Root Guidance Workflow Marker Plan](./maintainers/root-guidance-workflow-marker-plan.md)
- [Testing Strategy](./maintainers/testing-strategy.md)

## Structure Rules

Root `AGENTS.md` is a thin agent entry point. Keep detailed product,
architecture, testing, and release rules in this directory.

Testing rules belong in [Testing for Contributors](./contributing/testing.md).
Maintainer testing strategy belongs in
[Testing Strategy](./maintainers/testing-strategy.md).

Design and implementation specs live alongside these pages while they are active
references. Older or lower-frequency background documents can move into a
dedicated background directory when needed.
