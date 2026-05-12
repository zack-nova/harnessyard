---
status: accepted
---

# 0005 Runtime Bindings Contract

Harness Yard will expose Runtime Bindings through `.harness/vars.yaml` and the
public `hyard vars` command surface, with schema `2` as the first public
bindings schema. We chose a pre-release breaking reset because the previous
bindings shape mixed product terminology, internal paths, plaintext-only values,
and permissive unresolved rendering in ways that would make Package Variables
hard to diagnose and unsafe to pass into agent-facing runtime output.

## Considered Options

We considered keeping schema `1`, `.orbit/vars.yaml` compatibility, `$name`
template references, and unresolved placeholders as compatibility surfaces. We
rejected that because Harness Yard has not published the variable contract yet,
and carrying those surfaces forward would turn pre-release implementation
details into durable user concepts.

## Consequences

Runtime Bindings are canonical at `.harness/vars.yaml`; `.orbit/vars.yaml`,
schema `1`, scalar shorthand, `$name` template references, unresolved
placeholders, `--strict-bindings`, and public `--allow-unresolved-bindings` are
not part of the release contract. Runtime Bindings schema `2` supports inline
values and explicit `value_from.env` or `value_from.file` sources, with
sensitive Package Variables limited to `value_from.env` and redacted
diagnostics. Package-owned runtime rendering uses strict `{{ vars.<name> }}`
Package Template References, fails closed on unsupported namespaces, unknown or
unresolved variables, and malformed Harness Yard template syntax, and leaves
GitHub Actions `${{ ... }}` expressions outside the Harness Yard renderer.
