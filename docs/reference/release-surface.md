# Release Surface

This document defines the public release contract for Harness Yard CLI.

## Public Product Name

The current release headline is:

```text
Harness Yard CLI (hyard)
```

## Canonical Binary

The canonical public binary is:

```text
hyard
```

Formal release assets distribute `hyard` only.

Public product documentation should teach the top-level `hyard` command surface.

## Installation Channels

The recommended user installation channel is Homebrew:

```bash
brew tap zack-nova/tap
brew install hyard
```

Users may also install the formula with its fully qualified name:

```bash
brew install zack-nova/tap/hyard
```

The repository install script is also supported:

```bash
curl -fsSL https://raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh | bash
```

The install script installs `hyard` from the latest release and points users to
`hyard --help`.

## Package Lifecycle Surface

The canonical top-level package lifecycle surface is:

```bash
hyard install <template-source>
hyard install <namespace>/<name>
hyard install <namespace>/<name>@<semver>
hyard install <namespace>/<name>@latest
hyard install <curated-name>
hyard install <curated-name>[@latest]
hyard clone <namespace>/<harness-name> [repo-name]
hyard clone <curated-harness-name> [repo-name]
hyard uninstall orbit <orbit-package>
hyard uninstall harness <harness-package>
```

Package Handle Coordinates are case-insensitive and use
`namespace/name[@version-or-tag]` or curated `name[@version-or-tag]` syntax.
They are not npm-style `@namespace/name`. User-facing examples should prefer
package handles such as:

```bash
hyard install acme/docs
hyard install acme/docs@0.1.0
hyard install docs
```

Curated bare handles such as `docs` are reviewed aliases. `latest` is an
explicit registry dist-tag, so `acme/docs` is equivalent to `acme/docs@latest`;
documentation must not describe it as an inferred newest version or Git branch.
`hyard clone <handle>` is the public shortcut for creating a new Harness Runtime
from a registry-backed Harness Package. If the handle resolves to an Orbit
Package, users should create or enter a runtime and use `hyard install <handle>`
instead.
If fresh registry data is unavailable, stale cached bare or `latest`
resolutions may install with a warning. Mention `HYARD_CACHE_DIR` only in
troubleshooting context for the user-level registry cache.

User-facing package lifecycle documentation should prefer `uninstall`.
Scoped member-editing documentation may continue to use add/remove language, such as
`hyard orbit member add` and `hyard orbit member remove`, when it describes collection
membership rather than installed package lifecycle.

Package Uninstallation is package-manager-style removal from the current Harness
Runtime. The public `uninstall` surface deletes the active package record,
hosted OrbitSpec when no active package still owns it, unambiguous owned root
guidance, and package-owned runtime files. Retained Git history, audit output,
provenance data, detached install records, or removed hosted OrbitSpecs must not
make the package appear installed, active, reapplicable, or readiness-relevant.

## Runtime Bindings Surface

The canonical Runtime Bindings file is:

```text
.harness/vars.yaml
```

The public Runtime Bindings management surface is:

```bash
hyard vars path
hyard vars init <package-source>
hyard vars validate
hyard vars doctor
hyard vars explain [name]
```

Public documentation should use **Runtime Bindings** for runtime-owned values
that satisfy Package Variables. It should not teach users the historical
`bindings` command tree or `.orbit/vars.yaml`.

Package lifecycle commands may continue to accept `--bindings` to point at a
Runtime Bindings file, but examples should use `.harness/vars.yaml`.

The public Runtime Bindings schema version is `2`. Pre-release schema `1`
bindings documents and scalar shorthand are not part of the public release
contract.

The minimum public schema supports inline values and explicit value sources:

```yaml
schema_version: 2
variables:
  project_name:
    value: Harness Yard
  github_token:
    value_from:
      env: GITHUB_TOKEN
  issue_payload:
    value_from:
      file: .harness/context/issue.json
scoped_variables:
  docs:
    variables:
      project_name:
        value: Harness Yard Docs
```

`scoped_variables.<namespace>.variables` remains the public Scoped Bindings
shape in schema `2`. The namespace is the installed package or orbit namespace
whose Package Variables should receive the scoped value.

Package-owned runtime file rendering uses namespaced Package Template References:

```md
Project: {{ vars.project_name }}
```

The initial Package Template Reference context supports only the `vars`
namespace. References to `package.*`, `runtime.*`, `context.*`, `secrets.*`, or
any other namespace must fail with an unsupported namespace diagnostic instead
of being preserved or rendered as an empty string.

The pre-release `$project_name` shorthand is not part of the public template
contract. GitHub Actions expressions such as `${{ secrets.GITHUB_TOKEN }}` are
not Harness Yard Package Template References and must not be interpreted by the
Harness Yard renderer.

Package Template Reference rendering is strict. Public installation rendering
must fail before writing package-owned runtime output for unsupported namespaces,
unknown `vars.*` references, unresolved declared variables, or malformed Harness
Yard template syntax. Diagnostics should include the file path when available;
spelling suggestions are not required in the initial contract.

Package Variable declarations may include `required`, `description`,
`sensitive`, and `default`. Later type, enum, validation, migration, and IDE
schema support should extend this contract without reintroducing schema `1`.
`default` is a declaration-side fallback used only when no scoped or global
Runtime Binding is present. Defaults are reported as `source: default`, satisfy
required variables, and do not get written to `.harness/vars.yaml` unless a user
explicitly asks `hyard vars init --defaults` to materialize them. Sensitive
Package Variables must not declare defaults.

Sensitive Package Variables must not be bound with inline `value` or
`value_from.file`. The initial public contract only accepts `value_from.env` for
`sensitive: true` declarations, and diagnostics must redact resolved sensitive
values.

Required Package Variables are strict by default. Public `hyard install` flows
must fail before writing package-owned runtime output when required Runtime
Bindings are missing. Public documentation should not teach
`--strict-bindings`, `--allow-unresolved-bindings`, or unresolved runtime
placeholders as normal product behavior.

When `hyard install` is attached to an interactive terminal and required Runtime
Bindings are missing, it may prompt for the missing values, write them to
`.harness/vars.yaml`, validate them, and then continue installation. In
non-interactive or CI contexts it must fail with recovery commands such as:

```bash
hyard vars init <package-source> --out .harness/vars.yaml
hyard install <package-source> --bindings .harness/vars.yaml
```

The initial `hyard vars doctor` scope is limited to Runtime Bindings problems
that can block safe package installation or variable resolution:

- invalid `.harness/vars.yaml` schema or schema version
- invalid variable names
- bindings that provide both `value` and `value_from`, or neither
- blank `value_from.env` or `value_from.file`
- missing required Runtime Bindings for installed or selected packages
- sensitive Package Variables bound through inline `value` or `value_from.file`
- unset environment variables referenced by `value_from.env`
- warnings for undeclared bindings and missing `value_from.file` paths

Template usage scanning, unknown template references, typed validation, enum
validation, deprecation, migration, JSON Schema generation, and secret-manager
integration are not part of the initial `doctor` contract.

The initial `hyard vars explain [name]` scope is limited to the declaration and
selected resolution result. Text and JSON output should report:

- variable name
- resolved or unresolved status
- value source, using `.harness/vars.yaml`, `env:<name>`, `file:<path>`, or
  `default`
- redacted value for sensitive variables and literal value for non-sensitive
  variables
- required and sensitive flags
- installed package or orbit ids that declared the variable
- selected scope, either global or scoped namespace

Usage locations, template reference locations, shadowed candidates, and full
precedence trees are not part of the initial `explain` contract.

## Audit Review Surface

The public read-only review command is:

```bash
hyard audit
hyard audit --json
```

Audit is scoped to the current Git worktree and reports `pass`, `warn`, `fail`,
or `not_hyard_revision`. `pass` and `warn` exit 0; `fail` and
`not_hyard_revision` exit non-zero. Public docs should describe Audit as the
standard broad review command, while keeping runtime detail on `hyard check` and
Orbit Package publish readiness on `hyard orbit prepare <package> --check --json`.

## Harness Start Demo Paths

Public demo examples should use Package Handle Coordinates once registry entries
exist. Explicit Git locators are the advanced escape hatch when no registry
entry is available.

Clone a Harness Package by handle and hand off to Codex:

```bash
hyard clone acme/frontend-lab demo-runtime
cd demo-runtime
hyard start --with codex
```

Assemble packages into an existing Git repository with Runtime Initialization and
typed Package Installation and Package Uninstallation:

```bash
hyard init runtime
hyard install acme/frontend-lab
hyard install acme/docs --bindings .harness/vars.yaml
hyard install acme/api@latest --bindings .harness/vars.yaml
hyard install acme/ui@0.1.0 --bindings .harness/vars.yaml
hyard install docs
hyard uninstall harness frontend-lab
hyard uninstall orbit docs
```

Advanced explicit Git locator examples remain available for unpublished packages:

```bash
hyard clone https://github.com/acme/harness-templates.git demo-runtime --ref harness-template/frontend-lab
hyard install https://github.com/acme/harness-templates.git --ref harness-template/frontend-lab
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/docs --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/api --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/ui --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/ops --bindings .harness/vars.yaml
```

Run View Package Installation outputs package guidance incrementally so each
newly installed package can be used immediately.

Framework Activation is the public `hyard agent plan`, `hyard agent apply`, and
`hyard agent check` surface for Agent Framework side effects. `hyard agent apply`
materializes project/global Agent Framework assets such as skills, commands,
config, hooks, and aliases, then records ownership in the activation ledger.
It must not be documented as composing root `AGENTS.md`, `HUMANS.md`, or
`BOOTSTRAP.md`. Run View Root Guidance output remains owned by Package
Installation and explicit guidance output commands. Bootstrap Guide output is a
guidance/bootstrap surface; Harness Start only discovers and executes an
existing Bootstrap Guide before bootstrap closeout.

Publish demos should make a normal Git checkpoint before publishing the current
runtime as a Harness Package:

```bash
git status --short
git add .
git commit -m "Optimize frontend lab harness"
hyard publish harness workspace
```

## Runtime View And Publication Surface

Run View is the recommended runtime-user view for a Harness Runtime. Runtime-user
documentation and examples should teach users to inspect `hyard view status`, clean
visible authoring scaffolds with `hyard view run`, and publish the current runtime
as a Harness Package.

Run View publication should use `hyard publish harness <harness-package>`.
Run View examples and next actions should not recommend Orbit Package publication
as the default way to share runtime work.

Author View is the authored-truth view. Author documentation should explain
`hyard view author`, `hyard guide render`, `hyard guide save`,
`hyard orbit content apply`, and Orbit Package publication through
`hyard publish orbit <orbit-package>`.

Orbit Package publication remains available for authoring. It should be
documented as an authoring surface, not as the recommended runtime-user
publication path.

## Registry Entry Candidate Surface

Package authors register published packages by generating a reviewable Registry
Entry Candidate and submitting the resulting YAML to the package registry
repository. Orbit and Harness candidates share the same YAML shape; validation
differs by package kind.

```bash
hyard registry entry orbit acme/docs@0.1.0 --source origin --ref orbit-template/docs --package docs
hyard registry entry harness acme/workspace@0.1.0 --source origin --ref harness-template/workspace --package workspace
```

Default output is stdout. `--out <path>` writes a chosen file, and
`--registry <path>` writes under the candidate target path in a local registry
checkout. Local-only evidence can preview candidate YAML but cannot create a
submittable candidate.

Maintainer-level registry source behavior is documented in
`docs/maintainers/package-registry-source-contract.md`.

Main `hyard --help` output must stay stable across Runtime View Selection. Runtime
View Selection may affect command behavior and status/next-action output for a
runtime repository, but it must not dynamically rewrite the main CLI help surface.

## Root Guidance Marker Surface

Root guidance blocks use owner-specific marker namespaces with a single double-quoted
`workflow` attribute.

Orbit package guidance uses `orbit:` markers:

```html
<!-- orbit:begin workflow="docs" -->
<!-- orbit:end workflow="docs" -->
```

Harness package guidance uses `harness:` markers:

```html
<!-- harness:begin workflow="workspace" -->
<!-- harness:end workflow="workspace" -->
```

Root guidance marker workflow language does not rename OrbitSpec, manifest fields,
storage paths, member hints, package identity, or template branch contracts.

## Release Assets

Each release should publish platform archives that contain the `hyard` binary.

Expected archive naming pattern:

```text
hyard_${VERSION}_${GOOS}_${GOARCH}.tar.gz
```

`VERSION` is the release version without the leading `v` tag prefix.

Expected supported target matrix:

| OS | Architecture |
| --- | --- |
| linux | amd64 |
| linux | arm64 |
| darwin | amd64 |
| darwin | arm64 |

Each release should also publish:

```text
checksums.txt
```

## Version Metadata

`hyard --version` should include release metadata injected at build time:

- version
- commit
- date
- builtBy

The documentation must describe the metadata fields, but should not hard-code a specific current version.

## Homebrew Formula

The release process should update the Homebrew formula:

```text
zack-nova/homebrew-tap/Formula/hyard.rb
```

## Latest Release Downloads

The install path may rely on GitHub's latest-release download URL pattern, provided release asset names remain stable.

## Current Release Boundaries

The current release surface does not include:

- Windows prebuilt archives
- code signing
- macOS notarization
- SBOM generation
- provenance attestations

These capabilities may be added later once the release surface is stable.
