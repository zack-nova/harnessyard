# Harness Yard

Harness Yard turns ordinary Git repositories and authored package truth into reusable harness runtimes and templates for agent-assisted work.

## Language

**Harness Runtime**:
A Git repository that has Harness Yard control-plane truth and can install, activate, check, and publish harness content.
_Avoid_: workspace, plain repo

**Control-Plane Truth**:
Versioned Harness Yard files that identify revisions, packages, installs, ownership, templates, or runtime configuration.
_Avoid_: runtime cache, presentation content

**Authored Package Truth**:
Versioned Harness Yard truth that defines reusable package content, metadata, capabilities, or behavior before installation or publication.
_Avoid_: generated runtime output, presentation guidance

**Harness Yard Revision**:
A Git worktree revision recognized by Harness Yard as runtime, source, orbit-template, or harness-template truth.
_Avoid_: state, mode, view

**Runtime View**:
A repository-local presentation intent that distinguishes runtime consumption from package authoring.
_Avoid_: projection mode, global mode

**Run View**:
The runtime-user view where root guidance is presentation output and publication targets the composed Harness Runtime.
_Avoid_: authoring view, orbit publishing view

**Author View**:
The authored-truth view where materialized guidance blocks and content hints may be reconciled back into package truth.
_Avoid_: runtime view, run mode

**Ordinary Repository**:
A Git repository that may already contain agent-facing files but has not yet been initialized as a Harness Runtime.
_Avoid_: normal repo

**Adoption**:
An invasive conversion of an Ordinary Repository into a Harness Runtime by writing Harness Yard control-plane truth into the current repository.
_Avoid_: extraction, detached export

**Runtime Initialization**:
A minimal conversion of an existing Git repository into a Harness Runtime without adopting existing agent-facing assets as package truth.
_Avoid_: adoption, package install

**Source Adoption**:
A conversion of an existing Git repository or branch with prewritten authoring content into a source authoring revision for one Orbit Package.
_Avoid_: runtime adoption, create source first

**Source Revision**:
An authoring Harness Yard Revision that owns editable truth for one Orbit Package before publication.
_Avoid_: runtime source, template branch

**Adopted Orbit**:
The default single orbit created during Adoption to own the adopted repository's agent-facing truth and selected content.
_Avoid_: imported package

**Recommended Position**:
A Harness Yard conventional source-truth path for an adopted asset, such as `skills/<orbit-id>/*` for local skills or `commands/<orbit-id>/**/*.md` for prompt commands.
_Avoid_: generated output path

**Move Plan**:
A reviewed set of repository path relocations plus the corresponding Harness Yard truth updates needed to keep ownership, capabilities, and guidance references valid.
_Avoid_: git mv list

**Layout Optimization**:
A repository-wide operation that proposes or applies Harness Yard-friendly file placement across adopted members and agent assets.
_Avoid_: package move, orbit-only move

**Audit**:
A read-only standard review that inspects Harness Yard revision identity and control-plane truth across source, runtime, orbit-template, and harness-template revisions.
_Avoid_: review, check, prepare

**Audit Finding**:
A stable diagnostic emitted by Audit with severity, code, path, message, and optional package or revision scope.
_Avoid_: check finding, readiness reason

**Runtime Check**:
A read-only diagnostic for one Harness Runtime's structure, readiness, and view-aware presentation state.
_Avoid_: audit, package authoring reconciliation

**Harness Template**:
A reusable branch-form package exported from a Harness Runtime for installation into another repository.
_Avoid_: scaffold, snapshot

**Orbit Template Revision**:
An installable Harness Yard Revision exported from Orbit Package authoring truth for installation into a Harness Runtime.
_Avoid_: source revision, harness template

**Orbit Package**:
A reusable or installed package whose boundary is one orbit's authored truth and projected agent assets.
_Avoid_: member, folder

**Orbit Workflow**:
The public-facing name for an Orbit Package when emphasizing that one orbit is an atomic, closed-loop workflow.
_Avoid_: generic workflow, workflow id

**Harness Package**:
A reusable or installed package whose boundary is a composed harness workspace and its member orbits.
_Avoid_: workspace, bundle

**Package Registry**:
A catalog that resolves public package handles into installable Harness Package and Orbit Package locators.
_Avoid_: template repo, git remote, package manager

**Official Catalog**:
The default public Package Registry catalog owned outside the Harness Yard CLI source repository.
_Avoid_: product source tree, hosted registration service

**Package Namespace**:
A registry ownership prefix used to group public Package Handles before they resolve to concrete Package Identities.
_Avoid_: package type, GitHub remote, folder

**Package Handle**:
A registry-facing short name that users pass to package lifecycle commands.
_Avoid_: branch ref, local folder, display title

**Curated Handle**:
A bare Package Handle reviewed by the Official Catalog and mapped to a namespaced Package Handle.
_Avoid_: author-owned global package name, mutable default branch

**Package Handle Coordinate**:
A registry-facing install selector made from a Package Handle and optional version or dist-tag.
_Avoid_: Package Identity, Git locator, npm scoped package

**Registry Entry**:
A Package Registry record that maps a Package Handle to package metadata, status, and commit-pinned install locator data.
_Avoid_: source package manifest, local install record

**Registry Entry Candidate**:
A generated YAML proposal for adding or updating a Registry Entry in the Official Catalog.
_Avoid_: publish artifact, trusted registry fact

**Package Identity**:
The stable user-facing identity of an Orbit Package or Harness Package, made from package type, package name, and optional version when present.
_Avoid_: display name, branch ref, workflow id

**User Convention**:
A user-facing Harness Yard rule for how people name, organize, edit, configure, publish, or recover Harness Yard Revisions.
_Avoid_: internal schema, generated implementation detail

**Public Product Documentation**:
The user-facing documentation path that helps Runtime Users, Harness Authors, and Orbit Authors understand, configure, use, and author Harness Yard.
_Avoid_: maintainer documentation, release notes

**Configuration Reference**:
A public reference document that explains program-readable Harness Yard files, editing policies, conformance requirements, and validation commands.
_Avoid_: content writing guide, YAML schema dump

**Content And Workflow Guide**:
A public guide that explains maintained content responsibilities, file sizing, file purpose, and Orbit Workflow organization.
_Avoid_: control-plane schema, package lifecycle tutorial

**Concepts Guide**:
A short public guide that explains Harness Yard's product object model and mental model before command tutorials or reference details.
_Avoid_: command tutorial, configuration reference

**Maintained Content File**:
A file that a human or agent intentionally authors, reviews, or edits as package content, guidance, or user-facing convention.
_Avoid_: control-plane schema, repo-local cache

**Guidance File**:
A Maintained Content File that gives agents or humans rules, boundaries, workflow instructions, or handoff guidance.
_Avoid_: control-plane truth, cache

**Subject File**:
A Maintained Content File that contains the target product, domain, repository, or documentation content an Orbit Workflow operates on or interprets.
_Avoid_: process log, package metadata

**Process File**:
A Maintained Content File that records or templates workflow execution steps, checklists, workpads, review notes, or other process material.
_Avoid_: subject content, runtime cache

**Local Skill Asset**:
A package-owned Maintained Content File or directory that provides a local agent skill and its supporting resources.
_Avoid_: remote skill dependency, ordinary rule document

**Prompt Command Asset**:
A package-owned Maintained Content File that provides a prompt command template for an agent or user command surface.
_Avoid_: shell script, package lifecycle command

**Bootstrap Content**:
Initialization-only maintained content used to bring a Harness Runtime or Orbit Workflow into first usable state.
_Avoid_: steady-state rules, member role

**Runtime User**:
A person using an installed Harness Runtime to consume guidance, start an agent handoff, or publish the composed runtime.
_Avoid_: package author

**Harness Author**:
A person optimizing a Harness Runtime or Harness Package while using the runtime as a working environment.
_Avoid_: plain runtime user

**Orbit Author**:
A person maintaining Source Revisions, Orbit Template Revisions, or Orbit Package authored truth.
_Avoid_: runtime consumer

**Maintainer**:
A person responsible for plumbing, compatibility, release, or recovery behavior beyond ordinary package use.
_Avoid_: ordinary user

**Harness Workflow Block**:
A root guidance block owned by a Harness Package and written with public workflow marker language.
_Avoid_: orbit block, generic workflow

**Workflow ID**:
The root guidance marker field whose value equals the owning package name for its owner kind.
_Avoid_: display alias, package identity, renamed orbit id

**Workflow Owner Kind**:
The public marker owner category that says whether a root guidance block is owned by an Orbit Package or a Harness Package.
_Avoid_: arbitrary namespace, replacement package type

**Package Installation**:
The lifecycle operation that applies an Orbit Package or Harness Package to a Harness Runtime with package provenance.
_Avoid_: add

**Package Variables**:
Named inputs that an installable package may declare for Package Installation; absence means the package needs no user-provided values.
_Avoid_: environment variables, runtime state

**Sensitive Package Variable**:
A Package Variable whose value must not be persisted inline or displayed in normal diagnostics.
_Avoid_: ordinary variable, debug value

**Runtime Bindings**:
Runtime-owned values or value sources that satisfy Package Variables for a Harness Runtime.
_Avoid_: repo vars, fill-in vars, `.orbit/vars.yaml`

**Value Source**:
An explicit non-inline place a Runtime Binding can read from, such as an environment variable or a file.
_Avoid_: implicit shell expansion, GitHub Actions expression

**Scoped Bindings**:
Runtime Bindings limited to one package or orbit namespace when a shared Package Variable name needs package-specific values.
_Avoid_: global variables, environment variables

**Package Template Reference**:
A namespaced placeholder in package-owned content that resolves from a declared Package Variable during Package Installation.
_Avoid_: shell variable, GitHub Actions expression, `$name`

**Package-Owned Runtime File**:
A runtime file written into the current Harness Runtime by Package Installation and owned by that package.
_Avoid_: projection path, processed file

**Package Ownership Scope**:
The package-derived set of Package-Owned Runtime Files that a Package Uninstallation may delete.
_Avoid_: projection scope, write scope, export scope, orchestration scope

**Package Uninstallation**:
The lifecycle operation that fully removes an installed Orbit Package or Harness Package from a Harness Runtime.
_Avoid_: remove

**Agent Framework**:
A supported local agent tool that can consume Harness Yard guidance and agent assets.
_Avoid_: model provider, global agent environment

**Agent Framework Launcher**:
The framework-specific contract that tells Harness Yard how to start an Agent Framework with a Start Prompt.
_Avoid_: hard-coded exec command, terminal hack

**Framework Activation**:
The controlled materialization of agent assets for a selected Agent Framework with Harness Yard ownership recorded.
_Avoid_: config sync, installing the agent

**Bootstrap Agent Skill**:
A framework-specific agent skill that guides an agent through pending runtime bootstrap guidance.
_Avoid_: init skill, normal workflow skill

**Bootstrap Initialization Entry**:
The first user-facing or agent-facing handoff in a Harness Runtime that decides
whether pending bootstrap guidance should be read, materialized, skipped, or
reported.
_Avoid_: unconditional render step

**Bootstrap Guide**:
Pending runtime initialization guidance exposed for one or more runtime orbits
through the repository bootstrap surface; it may be a plain Run View
`BOOTSTRAP.md` payload or an owner-marked guidance block.
_Avoid_: always-rendered guide, marker-only guide

**Guide Render**:
An authoring or recovery action that materializes authored guidance into editable
runtime guidance artifacts.
_Avoid_: required bootstrap discovery step

**Harness Start**:
The high-level handoff flow that turns an installed Harness Runtime into an initialized interactive agent session.
_Avoid_: package install, plain prepare, app start

**Start Prompt**:
The stable initial prompt used by Harness Start to hand bootstrap and harness-introduction work to an Agent Framework.
_Avoid_: ad hoc agent message, framework-specific prompt

**Interactive Agent Session**:
A terminal session where a selected Agent Framework runs inside the Harness Runtime after handoff.
_Avoid_: printed next command, detached task

**Agent Asset**:
A repository artifact intended to shape agent behavior, such as root guidance, local skills, hooks, commands, or agent configuration.
_Avoid_: config file, AI stuff

**Root Guidance Artifact**:
A repository-root maintained guidance file that presents agent-facing, bootstrap, or human-facing entry information for a Harness Runtime or reusable template.
_Avoid_: canonical authored truth, orbit-local document

**Run View Root Guidance**:
Root guidance presented for runtime consumption after authoring markers have been removed or ignored.
_Avoid_: authored truth, backfill lane

**Run View Guidance Output**:
An explicit Run View action that writes runtime-facing root guidance into presentation files.
_Avoid_: guide sync, authoring render

**Run View Cleanup**:
The presentation operation that removes visible authoring markers and consumed hints from a Harness Runtime.
_Avoid_: backfill, authored truth sync

**Marked Guidance Resolution**:
The explicit choice made before Run View Cleanup removes a drifted marked root guidance block's authoring identity.
_Avoid_: force, overwrite

**Referenced Guidance Document**:
A document linked from root agent guidance that supplies agent-facing rules, constraints, or operating context.
_Avoid_: normal documentation

**Guidance Discovery**:
The adoption-time scan that finds candidate agent-facing documents referenced from root agent guidance.
_Avoid_: docs import, crawler

**Member Hint Frontmatter**:
A temporary Markdown authoring hint that proposes Orbit member truth through a nested `orbit_member` YAML mapping.
_Avoid_: flat `name` hint, document metadata

**Directory Member Marker**:
A temporary `.orbit-member.yaml` authoring hint that proposes member truth for the directory that contains it.
_Avoid_: directory frontmatter, detached member paths

**Flat Member Hint**:
A legacy shorthand that used top-level Markdown frontmatter fields as member truth.
_Avoid_: canonical member hint

**Behavior Scope Defaults**:
Orbit-level role-to-scope defaults that decide which member roles participate in projection, write, export, and orchestration unless a member overrides them.
_Avoid_: rule

## Relationships

- A **Harness Yard Revision** has exactly one revision kind: **Harness Runtime**, **Source Revision**, **Orbit Template Revision**, or **Harness Template**.
- An **Adoption** converts exactly one **Ordinary Repository** into exactly one **Harness Runtime**.
- **Adoption** is exposed as the top-level `hyard adopt` command because it is heavier than ordinary runtime initialization.
- Write-mode **Adoption** requires a clean worktree; `--check` may inspect a dirty worktree without mutating it.
- **Adoption** refuses an existing **Harness Runtime** and points the user to **Layout Optimization** instead.
- **Runtime Initialization** is the preferred first step when a user wants to assemble packages into an existing repository without importing existing agent-facing assets.
- **Runtime Initialization** is exposed as `hyard init runtime`; bare `hyard init` is a command group and does not default to runtime.
- An **Adoption** creates one **Adopted Orbit** by default, with its id derived from the repository name unless the user overrides it.
- **Adoption** is agent-first by default and does not automatically adopt the whole repository.
- The first **Adoption** version does not ask broad prompts for all `docs/` content or all source/business content.
- **Source Adoption** is the authoring-side counterpart to `hyard create source`: the user may write content first, then convert the repository into source truth for one Orbit Package.
- **Source Adoption** does not create a Harness Runtime, root runtime marker block, or runtime-level agent truth.
- **Source Adoption** writes source revision identity, hosted OrbitSpec truth, and member truth derived from authored content hints.
- **Source Adoption** defaults the Orbit Package id from the Git repository root directory name unless the user supplies `--orbit`.
- **Source Adoption** uses **Member Hint Frontmatter** and **Directory Member Marker** as temporary input, then consumes those hints into OrbitSpec member truth.
- Existing root guidance files discovered during **Source Adoption** may become Orbit Package meta templates instead of runtime guidance blocks.
- **Source Adoption** reports publishing the Orbit Package as the next handoff action after writing source truth.
- A **Source Revision** and an **Orbit Template Revision** each describe exactly one **Orbit Package**.
- A **Source Revision** may publish an **Orbit Template Revision**.
- Public demo documentation may show an **Orbit Template Revision** authoring repository as a dedicated single-orbit development path: create or initialize an orbit-template repository, develop exactly one Orbit Workflow, and publish/update the corresponding `orbit-template/<package>` branch.
- Multi-orbit composition belongs to a **Harness Runtime** or **Harness Package**, not a **Source Revision** or **Orbit Template Revision**.
- A **Harness Runtime** may publish one or more **Harness Templates** over time.
- A **Harness Template** is exported from a **Harness Runtime**, not directly from an **Ordinary Repository**.
- A **Runtime User** or **Harness Author** sharing a composed runtime should publish a **Harness Package**.
- An **Orbit Author** sharing reusable orbit authored truth should publish an **Orbit Package**.
- A single person may use one **Harness Runtime** in **Author View** as both a **Harness Author** and an **Orbit Author**, creating hosted Orbit Workflow authored truth, publishing Orbit Packages, and publishing the composed Harness Package from the same repository when that is the intended authoring scenario.
- Template save commands are lower-level export primitives rather than the main user publication path.
- Cloning a **Harness Template** should suggest **Harness Start** as the next handoff action but should not start an agent automatically.
- Early harness optimization demos may use manual Git checkpoints before publishing a Harness Template.
- A **Package Identity** uses package type to distinguish Orbit Packages from Harness Packages, and package name as the stable user-facing package name.
- Display `name` and `description` metadata do not replace **Package Identity**.
- A **Package Registry** lets public commands resolve **Package Handles** without exposing Git branch locators in ordinary demos.
- A **Package Registry** registers **Orbit Packages** and **Harness Packages** only; it does not register arbitrary tools, services, agent frameworks, or non-Harness Yard package shapes.
- The Harness Yard CLI source repository owns **Package Registry** schema, resolver behavior, product documentation, `hyard registry entry` candidate generation, and package installation semantics; the **Official Catalog** repository owns catalog entries, namespace ownership records, curated handles, and registry review policy.
- The **Official Catalog** source is `zack-nova/hyard-registry`; Package Registry resolution also supports other Git remotes as registry sources.
- Public registration uses catalog-as-code through reviewed **Registry Entry Candidates**. The first version does not provide a hosted registration service, account system, OAuth flow, automatic registration API, or automatic pull request creation.
- Registry-backed **Package Installation** resolves **Package Handle Coordinates** to versioned, commit-pinned **Orbit Package** or **Harness Package** locators.
- Bare **Package Handle** installation is limited to **Curated Handles**, which point at namespaced **Package Handles** and resolve through an explicit registry `latest` dist-tag before selecting a versioned commit-pinned locator.
- Registry-backed **Package Installation** uses a user-level global cache whose location can be overridden with `HYARD_CACHE_DIR`. Exact versions may be cached; bare and `latest` resolutions refresh from the registry when available and may use a previously verified cached resolution with a warning when the registry is unavailable.
- Package Registry status is package-level: deprecated packages warn, yanked packages require explicit override, and blocked packages cannot be installed.
- The first registry entry candidate generator is a separate `hyard registry entry` command, not an option folded into `hyard publish`.
- A submittable **Registry Entry Candidate** is YAML and requires target path, package type, package identity, source repository, ref, commit reachability, commit SHA, package status, validation evidence, and installability validation; local-only publication results may produce preview output but not a submittable entry.
- A **Package Handle** is not necessarily the same string as **Package Identity**; the registry resolves a handle to a package type, package name, version, and locator.
- Public **Package Handle Coordinates** are case-insensitive and use `namespace/name[@version-or-tag]` or curated `name[@version-or-tag]` syntax, not npm-style `@namespace/name`.
- `latest` is an explicit registry dist-tag; it is not inferred from Git branches, newest registry merges, or highest SemVer versions.
- Registry version entries resolve to commit SHAs. Branches and tags may be provenance inputs, but Package Installation uses the resolved commit.
- Early demos may use explicit GitHub package locators before **Package Registry** resolution is ready.
- A **User Convention** may point to visible configuration files, modifying commands, defaults, and recommended settings, but it does not define internal Harness Yard schema or generated implementation behavior.
- User convention documentation is organized by convention type and marks which **Harness Yard Revisions** each convention applies to.
- User convention documentation interprets `MUST`, `MUST NOT`, `REQUIRED`, `SHOULD`, `SHOULD NOT`, `RECOMMENDED`, `MAY`, and `OPTIONAL` according to RFC 2119.
- User convention documentation describes currently supported behavior; future work may appear only as non-normative notes or open questions.
- **Public Product Documentation** primarily serves **Runtime Users**, **Harness Authors**, and **Orbit Authors**; maintainer-focused plumbing, release, architecture, and recovery material belongs in maintainer documentation.
- **Public Product Documentation** should teach the top-level `hyard` command surface and omit lower-level compatibility or plumbing commands before those surfaces become intentional user-facing product paths.
- The target **Public Product Documentation** set is `README.md`, `docs/installation.md`, `docs/quickstart.md`, `docs/concepts.md`, `docs/reference/configuration.md`, `docs/guides/content-and-workflows.md`, `docs/guides/harness-authoring.md`, and `docs/guides/orbit-authoring.md`.
- Public `README.md` should stay a short product introduction and reader-routing entry point rather than a complete documentation index.
- `docs/README.md` should be the complete documentation index, ordered by reader path before lower-level reference and maintainer material.
- Public end-to-end demo documentation belongs under `docs/demos/` and should be linked from `docs/README.md`; Quickstart and authoring guides remain shorter reader-path guides instead of carrying full terminal transcript demos.
- Public end-to-end demo documentation may use dedicated lightweight demo fixture repositories when existing real Orbit Packages or Harness Packages would force readers to understand unrelated workflow domains before they can understand Harness Yard.
- Dedicated demo fixture repositories should have narrow, obvious Orbit Workflow boundaries and a composed Harness Package whose package composition demonstrates the Harness Yard object model without becoming a heavyweight product workflow.
- The initial public demo fixture set is `zack-nova/hyard-demo-docs-orbit` publishing package `docs` on `orbit-template/docs`, `zack-nova/hyard-demo-review-orbit` publishing package `review` on `orbit-template/review`, `zack-nova/hyard-demo-release-orbit` publishing package `release` on `orbit-template/release`, and `zack-nova/hyard-demo-product-lab` publishing package `product-lab` on `harness-template/product-lab` composed from `docs`, `review`, and `release`.
- A **Concepts Guide** should explain the core Harness Yard product model once so tutorial, configuration, and authoring documents do not duplicate revision, package, workflow, and view terminology.
- The public Quickstart should serve the **Runtime User** first-success path; **Harness Author** and **Orbit Author** first-success paths belong in their separate authoring guides.
- A **Configuration Reference** defines the program-readable files, editing policies, conformance requirements, and validation commands users need to keep Harness Yard truth valid.
- A **Configuration Reference** documents the user-maintainable configuration contract, not every internal YAML field or a generated schema dump.
- A **Configuration Reference** may identify maintained content files as readable or editable surfaces, but detailed file purpose, sizing, and writing guidance belongs to the **Content And Workflow Guide**.
- A **Content And Workflow Guide** defines how maintained content files should be written, sized, split, and organized into Orbit Workflows without becoming a YAML schema reference.
- A **Content And Workflow Guide** includes Orbit Workflow sizing, authored-contract questions, file responsibilities, runtime surface boundaries, and source-only content boundaries, but it does not own package publication tutorials.
- Existing user convention and content responsibility material should be reorganized under **Configuration Reference** and **Content And Workflow Guide** responsibilities instead of remaining parallel primary documentation entry points; the old `docs/reference/user-conventions.md` and `docs/reference/content-responsibilities.md` paths should be deleted after their current decisions are migrated.
- Historical orbit authoring manuals may inform public orbit authoring guidance, but they are not active product documentation unless their current decisions are extracted into **Public Product Documentation**.
- Public orbit authoring guidance should extract a current Orbit Author first-success path from historical authoring manuals rather than migrating the old manual structure wholesale.
- Public harness authoring guidance should explain the Harness Author path for composing multiple Orbit Workflows into a reusable Harness Package, including package composition, variables, root guidance, readiness checks, and `hyard publish harness`.
- Harness authoring guidance and orbit authoring guidance are separate public authoring paths because **Harness Authors** compose multiple **Orbit Workflows** into a reusable **Harness Package**, while **Orbit Authors** maintain one **Orbit Package**.
- Public demo documentation may present code repository developers as an independent **Harness Author** scenario when they initialize an existing repository, assemble packages, select Agent Frameworks, tune Run View presentation, and publish the composed **Harness Package**; this should not introduce a separate durable product role.
- User-visible files are documented with one editing policy: tool-owned **Control-Plane Truth**, hand-editable but validated **Authored Package Truth**, or directly editable content and presentation.
- Tool-owned **Control-Plane Truth** may be inspected and committed, but user convention documentation should direct changes through `hyard` commands.
- Hand-edited **Authored Package Truth** must be validated with the relevant `hyard` audit, check, or orbit validation command before publication or runtime use.
- User convention documentation treats normal Git commits as the sharing and evidence mechanism for versioned Harness Yard truth.
- User convention documentation may recommend `hyard` scoped worktree commands as helpers while preserving normal Git commands when users respect package and path ownership.
- User convention documentation omits non-user-visible runtime cache and local state details unless a user-facing command requires them.
- User convention configuration tables stay user-facing: applicable revisions, visible files, modifying commands, defaults, recommendations, and validation commands, not full YAML schema.
- User convention documentation uses lightweight role labels such as **Runtime User**, **Harness Author**, **Orbit Author**, and **Maintainer** only where the role changes the convention.
- User convention documentation may include a short prohibited-actions section for common user-facing mistakes, but should not restate internal architecture bans.
- User convention documentation for harness and orbit content responsibilities covers **Maintained Content Files**, not internal control-plane schemas or repo-local cache files.
- Harness and orbit content responsibility documentation may include a short "not maintained content" boundary table for control-plane and cache files, but it must not expand their YAML schema or make hand-editing them the normal path.
- Harness and orbit content responsibility documentation is organized around **Harness Runtime**, **Harness Template**, **Orbit Template Revision**, and **Source Revision**; **Run View** and **Author View** are view-specific notes within those revisions, not additional revision kinds.
- Harness and orbit content responsibility documentation belongs under the **Content And Workflow Guide** rather than the old `docs/reference/content-responsibilities.md` path.
- Harness and orbit content responsibility documentation marks file responsibilities as Required, Conditional, or Optional per revision kind, rather than requiring every revision to carry the same files.
- Harness and orbit content responsibility tables use recommendations for content shape; those recommendations are not independent validation constraints.
- Harness and orbit content responsibility documentation covers the **Root Guidance Artifacts** `AGENTS.md`, `BOOTSTRAP.md`, and `HUMANS.md`.
- `AGENTS.md` is the agent work entry point and should stay under 200 lines by linking to deeper maintained content instead of carrying full package documentation.
- `AGENTS.md` and its linked steady-state guidance should assume normal runtime operation and generally describe only run-state rules.
- Steady-state guidance should remind users to initialize only when missing initialization blocks or makes unsafe the current run-state work; it should not carry the detailed initialization workflow.
- `BOOTSTRAP.md` is the bootstrap entry point for pending initialization guidance and should not become a steady-state rule index.
- Detailed initialization rules belong in **Bootstrap Content** such as `BOOTSTRAP.md` or a bootstrap-oriented skill, and that initialization content may be removed after setup is complete.
- `HUMANS.md` is an optional human-facing orientation surface and must not replace agent-facing rules in `AGENTS.md`.
- An **Orbit Workflow** contributes a root guidance block to `AGENTS.md` rather than requiring a separate root `AGENTS.md` per orbit.
- An **Orbit Workflow** root guidance block should stay under 30 lines and link to deeper maintained content when it needs more detail.
- Source-side orbit content responsibility documentation classifies maintained content by responsibility: **Guidance File**, **Subject File**, **Process File**, **Local Skill Asset**, **Prompt Command Asset**, and **Bootstrap Content**.
- Common paths such as `skills/<orbit-id>/*` and `commands/<orbit-id>/**/*.md` are examples of **Recommended Positions**, not the definition of the content categories.
- **Bootstrap Content** may use `lane: bootstrap`, but bootstrap is lifecycle metadata rather than a fifth orbit member role.
- A **Harness Runtime** may install and uninstall **Orbit Packages** and **Harness Packages** through package lifecycle commands.
- A composed harness is described concretely as either a **Harness Runtime** when running inside a Git repository or a **Harness Package** when published for reuse; avoid using standalone "harness" as a storage, runtime, or package identity when one of those precise terms applies.
- A **Harness Runtime** or **Harness Package** may combine multiple **Orbit Workflows** into one agent work system.
- A **Package Installation** may declare zero **Package Variables**; zero variables is a complete variable contract, not a missing one.
- A **Package Installation** records its complete **Package Variables** contract, including the explicit zero-variable case.
- Users provide **Package Variables** through **Runtime Bindings**, either in the canonical `.harness/vars.yaml` file or through an explicit `--bindings` file.
- `.orbit/vars.yaml` is not a **Runtime Bindings** path or a public product compatibility surface.
- **Runtime Bindings** may provide inline values or explicit **Value Sources**; implicit environment expansion is not part of the Runtime Bindings model.
- **Sensitive Package Variables** must be bound through non-persisted value sources and redacted in diagnostics.
- Declaration defaults satisfy **Package Variables** only when no scoped or global **Runtime Binding** is present.
- Package-owned content resolves **Package Template References** through the `{{ vars.<name> }}` syntax.
- The initial **Package Template Reference** context contains only the `vars` namespace.
- **Package Template Reference** rendering fails closed on unsupported namespaces, unknown variables, unresolved variables, and malformed syntax.
- Missing required **Runtime Bindings** block **Package Installation** rather than producing unresolved runtime placeholders.
- Interactive **Package Installation** may collect missing required **Runtime Bindings** before writing package-owned runtime output.
- Public Runtime Bindings management is exposed through the `hyard vars` command surface.
- User convention documentation locates Runtime Bindings files and commands without defining the complete bindings schema.
- **Package Installation** in **Run View** may automatically compose root guidance and apply **Run View Cleanup** after package truth is written.
- **Package Installation** in **Run View** outputs guidance incrementally for the newly installed package rather than recomposing existing markerless presentation guidance.
- **Package Installation** appends incremental **Run View Root Guidance** to the end of the relevant root guidance file with stable separation.
- **Package Installation** succeeds when package truth is written even if **Run View Cleanup** is deferred by marked guidance with unsaved author edits.
- A deferred **Run View Cleanup** should be reported as presentation cleanup work, not as failed installation or generic authored-truth drift.
- **Package Installation** should not prompt for **Marked Guidance Resolution**; explicit `view run` owns that choice.
- **Package Installation** should defer **Run View Cleanup** instead of implicitly saving, re-rendering, or stripping drifted marked guidance.
- **Package Installation** in **Run View** should leave users with markerless **Run View Root Guidance** when automatic output and cleanup complete safely.
- A **Harness Start** does not replace **Package Installation** or **Package Uninstallation**; public package lifecycle examples should use `install` and `uninstall`.
- A **Harness Start** may choose an **Agent Framework** from explicit user choice, local readiness, or package recommendation.
- **Harness Start** framework resolution prefers explicit `--with`, then existing repo-local selection, then an unambiguous ready or recommended ready Agent Framework.
- Ambiguous **Harness Start** framework resolution asks interactively when possible and fails closed with candidates in non-interactive contexts.
- An explicit **Harness Start** framework choice becomes the Harness Runtime's repo-local agent selection unless the user asks for a temporary start.
- A **Harness Start** performs **Framework Activation** before handing control to an **Agent Framework**.
- A **Harness Start** uses a **Bootstrap Agent Skill** when pending runtime bootstrap guidance needs agent-led initialization.
- A **Harness Start** installs the **Bootstrap Agent Skill** after project-only Framework Activation and before launching the Interactive Agent Session.
- **Bootstrap Initialization Entry** discovers an existing **Bootstrap Guide** in
  `BOOTSTRAP.md` without running **Guide Render**.
- **Guide Render** may create or refresh a **Bootstrap Guide** only when
  discovery shows that materialization is likely missing and a human chooses
  authoring or recovery.
- **Bootstrap Initialization Entry** owns non-destructive bootstrap discovery.
- **Guide Render** owns materializing authored guidance; it is not mandatory when
  a **Bootstrap Guide** is already present.
- Installed bootstrap skill discovery is a plain `BOOTSTRAP.md` existence check
  instead of a guide render or guide inspection command.
- Installed bootstrap skill guidance does not run **Guide Render** automatically;
  if `BOOTSTRAP.md` is absent, it reminds the user that bootstrap may be
  unnecessary or that authored guidance may need to be rendered separately.
- Installed bootstrap skill guidance does not run guide inspection before
  bootstrap execution; it treats an existing `BOOTSTRAP.md` as the
  initialization entry regardless of whether the file contains owner markers.
- Bootstrap closeout runs only after an existing `BOOTSTRAP.md` has been read and
  executed, and a retained plain `BOOTSTRAP.md` is not a closeout failure.
- A **Harness Start** defaults to project-only **Framework Activation** and must not write the user's global agent environment unless the user explicitly chooses a global route.
- User convention documentation describes Agent Framework selection and activation at the user-command level, not adapter-native schema level.
- User convention documentation must not present unresolved remote skill pinning or materialization behavior as a supported user contract.
- A **Harness Start** does not require a globally clean worktree, but must fail closed on conflicting local edits to paths it would write.
- A **Harness Start** does not create Git commits; checkpointing belongs to authoring and publishing flows.
- A **Harness Start** succeeds only when it can launch an **Interactive Agent Session** for the selected **Agent Framework**.
- A **Harness Start** requires a terminal-launchable **Agent Framework** for success.
- A **Harness Start** fails closed with manual next actions, a reusable prompt, and usage instructions when the selected **Agent Framework** cannot be launched interactively.
- A **Harness Start** may preview the handoff without mutation or launch for tests and manual fallback.
- A **Harness Start** dry run does not write selection, activation, bootstrap skill, or launch state.
- A **Harness Start** prompt-only mode does not mutate runtime files or launch an Agent Framework.
- A **Harness Start** launches an **Interactive Agent Session** through an **Agent Framework Launcher**, not through command-specific branching in the start flow.
- An **Agent Framework Launcher** owns executable discovery, argument construction, working directory, environment policy, and interactive support for one Agent Framework.
- The first **Harness Start** launcher supports Codex; other supported Agent Frameworks may return prompt and manual usage instructions until their launchers are implemented.
- A Codex **Agent Framework Launcher** must verify a stable CLI invocation contract before Harness Start can launch Codex interactively.
- **Framework Activation** and an **Interactive Agent Session** are separate lifecycle steps: activation materializes agent assets, while the session runs the agent.
- Bootstrap completion during **Harness Start** is agent-led through the **Bootstrap Agent Skill**, not inferred by `hyard` outside the session.
- A **Harness Start** uses one **Start Prompt** that first directs pending bootstrap work and then asks the same session to introduce the harness.
- A **Start Prompt** is shared across Agent Framework launchers; framework-specific code only decides how to deliver it.
- A **Bootstrap Agent Skill** is temporary bootstrap guidance, not an **Orbit Package** skill dependency or steady-state workflow skill.
- A **Bootstrap Agent Skill** installed by **Harness Start** remains until the user explicitly removes it.
- **Framework Activation** must keep repo-local activation truth separate from the user's global agent environment unless the user explicitly chooses a global route.
- An **Orbit Workflow** is an **Orbit Package** described through public workflow language; it does not replace orbit as the internal compatibility and storage term.
- An **Orbit Workflow** marker uses the `orbit:` namespace and its **Workflow ID** is the same stable identity as the internal orbit id; it is not a separate display alias.
- A **Harness Workflow Block** marker uses the `harness:` namespace and its **Workflow ID** is the same stable identity as the owning harness package id; it is not a separate display alias.
- Orbit ids and harness ids remain valid concrete identities; **Workflow ID** is their public root-guidance marker umbrella.
- **Workflow ID** is explained as root guidance marker syntax, not as a separately configurable user identity.
- A root guidance block's generic identity is **Workflow Owner Kind** plus **Workflow ID**.
- **Workflow Owner Kind** values are limited to `orbit` and `harness`, matching the public marker namespaces.
- Public workflow marker syntax applies to root guidance blocks, not authored member hints, manifest schema, branch locators, or storage paths.
- `OwnerKind + WorkflowID` implementation naming applies to root guidance marker parsing, rendering, replacement, extraction, and removal, not to repository-wide OrbitSpec or manifest identity fields.
- Public workflow marker namespaces are limited to `orbit:` and `harness:`.
- Root guidance marker reads and writes use `workflow`; pre-release `orbit_id` marker attributes do not need compatibility support.
- Root guidance markers are strict single-line HTML comments with one double-quoted `workflow` attribute.
- Harness-owned root guidance blocks use the `harness:` namespace; pre-release harness-owned `orbit:` blocks do not need compatibility migration.
- Root guidance marker parsers accept exactly one `workflow` attribute; duplicate attributes and unknown attributes are invalid.
- Root guidance block uniqueness is scoped by marker namespace and `workflow` value, so `orbit/docs` and `harness/docs` are distinct block owners.
- User-facing diagnostics call root guidance blocks `orbit block` or `harness block` by owner kind rather than generic workflow blocks.
- Top-level package lifecycle commands use install/uninstall language; scoped member-editing commands may use add/remove language.
- A **Package Uninstallation** targets one installed package name; an ambiguous name must be qualified as an **Orbit Package** or **Harness Package**.
- A **Package Uninstallation** identifies its target by the installed package name, not by a versioned package coordinate.
- A **Package Uninstallation** removes the target package from the current Harness Runtime rather than detaching it from active membership while retaining package truth.
- After **Package Uninstallation**, retained provenance or audit evidence must not keep the target visible as installed, active, reapplicable, or readiness-relevant.
- A **Package Uninstallation** deletes the target package's install record instead of retaining a detached install record in `.harness/installs/`.
- A **Package Uninstallation** deletes the target package's hosted OrbitSpec from `.harness/orbits/` unless another active package still owns or references it.
- A **Package Uninstallation** removes the target package's marked root guidance block when the block has unambiguous owner identity.
- A **Package Uninstallation** does not infer ownership from markerless **Run View Root Guidance** and must leave markerless presentation text untouched.
- A **Package Uninstallation** may modify or delete an untracked root guidance artifact when the removed content has unambiguous target-package owner identity and any remaining content can be preserved.
- A **Package Uninstallation** deletes target-owned **Package-Owned Runtime Files** regardless of whether Git reports them as tracked, staged, unstaged, or untracked.
- A **Package Uninstallation** may delete locally changed **Package-Owned Runtime Files** after warning and explicit confirmation.
- In non-interactive **Package Uninstallation**, `--yes` confirms deletion of target-owned **Package-Owned Runtime Files**, including files with local Git changes.
- A **Package Uninstallation** dry run reports target-owned files that would be deleted and highlights any local Git changes requiring confirmation.
- A single **Orbit Package** uninstallation may execute without confirmation when all deleted files are clearly target-owned and no local Git changes, shared ownership, global agent cleanup, or multi-package side effects require confirmation.
- A **Package Uninstallation** asks for confirmation when it will delete locally changed target-owned files or perform broader side effects such as global agent cleanup or multi-package removal.
- A **Package Uninstallation** cleans up empty directories created by deleting target-owned files, stopping at repository, runtime-control, and non-empty directory boundaries.
- A member role such as subject, rule, or process does not by itself decide **Package Uninstallation** deletion; package ownership decides deletion.
- Process-role files may be **Package-Owned Runtime Files** and should be deleted during **Package Uninstallation** when owned by the target package.
- Files that an orbit reads, projects, edits, processes, or scopes over are not deleted by **Package Uninstallation** unless they are **Package-Owned Runtime Files**.
- A **Package Ownership Scope** is derived from the installed package's authored member truth and source payload, not from projection, write, export, or orchestration scope.
- A **Package Uninstallation** uses the target package's **Package Ownership Scope** to decide which runtime files to delete.
- Package type ambiguity during **Package Uninstallation** is never guessed; the user must choose `orbit` or `harness` explicitly.
- A **Package Uninstallation** may target a manually added **Orbit Package**, but command output must disclose the manual source because no install provenance exists.
- Uninstalling a **Harness Package** fully removes that harness package, its included orbit packages, harness-owned runtime files, harness-owned root guidance, and package records as one package lifecycle operation.
- Uninstalling one **Orbit Package** may execute immediately, while uninstalling a **Harness Package** needs preview and confirmation support because it can remove multiple orbits and global agent outputs.
- Top-level `remove` remains a compatibility alias for **Package Uninstallation**, but documentation and user-facing examples should prefer `uninstall`.
- The compatibility `remove` surface stays callable for existing scripts but should be hidden from top-level help.
- The compatibility `remove` surface may keep its own help output, but that help should identify `uninstall` as the preferred command.
- The compatibility `remove` surface must preserve **Package Uninstallation** semantics rather than expose lower-level detach or shrink behavior.
- Lower-level runtime member detach, bundle shrink, or member removal may exist as plumbing or maintainer operations, but they are not public **Package Uninstallation**.
- Package uninstallation JSON should preserve existing remove-shaped result fields for compatibility, even when the canonical command name is `uninstall`.
- Human-readable output from the canonical `uninstall` surface should use `uninstalled` language.
- Preview and confirmation output from the canonical `uninstall` surface should describe targets as items to uninstall.
- Package uninstallation error guidance should prefer `uninstall` commands, including when reached through the compatibility `remove` surface.
- The `uninstall` and compatibility `remove` command surfaces should share one package-uninstallation implementation so lifecycle semantics do not drift.
- **Runtime View** selection changes presentation and publication defaults, not package identity or canonical authored truth.
- **Run View** is the default runtime-user presentation, but it is not a universal recommendation for every role.
- **Run View** serves Harness Runtime users who consume installed guidance and publish the composed runtime.
- **Author View** serves developers and authors who are using a Harness Runtime while optimizing harness or orbit authored truth.
- Root guidance artifacts are user-visible guidance containers, not canonical package schema.
- User convention documentation describes root guidance markers only to the extent needed for user editing, save, cleanup, and recovery.
- **Run View Root Guidance** is a materialized presentation artifact, not an authored backfill lane.
- Markerless **Run View Root Guidance** must not create authored-truth drift by differing from Orbit Package guidance templates.
- Existing markerless **Run View Root Guidance** is presentation text and should not be recomposed from package truth during later **Package Installation**.
- Existing markerless **Run View Root Guidance** should not be reordered by later **Package Installation** because it no longer has owner identity.
- Existing markerless **Run View Root Guidance** should not be deduplicated automatically because repeated text cannot be safely attributed to an owner.
- **Runtime Check** should not report duplicate-looking markerless **Run View Root Guidance** because repeated presentation text has no reliable owner identity.
- Standalone **Run View Guidance Output** outside **Package Installation** requires explicit user confirmation or an explicit non-interactive option.
- The explicit non-interactive option for standalone **Run View Guidance Output** should use output language, not force language.
- Standalone **Run View Guidance Output** is presentation output and must be treated as additive or replace-risky because markerless guidance no longer carries owner identity for precise block replacement.
- Marked root guidance blocks preserve owner identity for explicit reconciliation before **Run View** cleanup removes that identity.
- **Run View Cleanup** must fail closed on marked root guidance blocks with unsaved author edits unless the user explicitly chooses to discard the authoring identity.
- **Run View Cleanup** must not fail closed on markerless **Run View Root Guidance** merely because it differs from package guidance templates.
- Interactive **Run View Cleanup** resolves drifted marked guidance through **Marked Guidance Resolution** before deleting markers.
- **Marked Guidance Resolution** choices are: save current block to authored truth before cleanup, re-render authored truth before cleanup, or strip markers in place and keep the current text as **Run View Root Guidance**.
- Non-interactive **Run View Cleanup** must fail closed on unresolved drifted marked guidance and report the available **Marked Guidance Resolution** paths.
- **Runtime Check** must not report markerless **Run View Root Guidance** as install-backed runtime file drift.
- **Runtime Check** should report root guidance diagnostics according to **Runtime View**: Run View checks presentation usability, while **Author View** checks authored reconciliation risk.
- **Runtime Check** should still fail closed on malformed or duplicate root guidance markers because those make owner identity ambiguous.
- **Author View** is the correct view for `guide render`, `guide save`, content hint reconciliation, and Orbit Package publication.
- `--with-spec` authoring bootstrap creates maintained rule content under `docs/<orbit-id>/` rather than declaring an empty directory pattern only.
- `--with-spec` authoring bootstrap includes the rule directory in the existing spec rule member instead of creating a separate rule member.
- `--with-spec` authoring bootstrap keeps the existing `spec` member name when extending that member to include the initial rule directory.
- `--with-spec` authoring bootstrap writes a minimal `docs/<orbit-id>/README.md` rule entry point and does not add member hint metadata to that file.
- `--with-spec` authoring bootstrap applies the same spec member and rule directory behavior across orbit create, source authoring, and orbit-template authoring creation paths.
- `--with-spec` authoring bootstrap fails closed when the generated rule directory README already exists, rather than silently adopting existing README content.
- `--with-spec` authoring bootstrap fails closed when `docs/<orbit-id>/` already exists, because including `docs/<orbit-id>/**` would otherwise silently adopt existing directory content as rule content.
- `--with-spec` authoring bootstrap keeps creating `docs/<orbit-id>.md` and adds `docs/<orbit-id>/README.md`; the existing `spec` member includes both the spec document and `docs/<orbit-id>/**`.
- `--with-spec` authoring bootstrap keeps the existing minimal `docs/<orbit-id>.md` content unchanged while using `docs/<orbit-id>/README.md` as the rule directory entry point.
- Public help and user documentation for `--with-spec` authoring bootstrap must describe both generated files and the expanded spec rule member scope.
- Authoring bootstrap without `--with-spec` keeps the existing no-content side effect behavior and does not create `docs/<orbit-id>/`.
- Generated `--with-spec` rule directory README titles use the stable orbit id rather than optional display name metadata.
- A **Referenced Guidance Document** discovered during **Adoption** is a candidate for rule content, but its final member role requires user confirmation.
- **Guidance Discovery** follows references from root guidance one hop by default; recursive discovery requires an explicit user choice.
- A directory reference found during **Guidance Discovery** stays a directory member rather than expanding into separate file members.
- A **Member Hint Frontmatter** must be strict YAML frontmatter at the start of a Markdown file and must contain a nested `orbit_member` mapping.
- Ordinary Markdown content without **Member Hint Frontmatter** remains valid content and does not need YAML frontmatter.
- A **Member Hint Frontmatter** may coexist with ordinary document metadata, but only the nested `orbit_member` mapping is Harness Yard member truth.
- A **Member Hint Frontmatter** describes the Markdown file or marker directory where it appears; it does not declare arbitrary member paths.
- A **Member Hint Frontmatter** may declare ordinary content roles, but must not declare the control-plane `meta` role.
- A **Member Hint Frontmatter** may declare `lane: bootstrap`; no other member lane has canonical meaning.
- A **Member Hint Frontmatter** must not declare member scopes; projection, write, export, and orchestration participation come from **Behavior Scope Defaults** or explicit OrbitSpec member truth.
- A **Member Hint Frontmatter** may omit `name`; missing name is derived from the hinted file or marker directory.
- A **Member Hint Frontmatter** may omit `description`; missing description means the authored member has no description.
- A file-level **Member Hint Frontmatter** defaults to the `rule` role when `role` is omitted.
- A directory-level member marker defaults to the `process` role when `role` is omitted.
- A malformed **Member Hint Frontmatter** fails closed instead of being treated as ordinary Markdown metadata.
- Member Hint parsing may normalize CRLF to LF before enforcing the strict YAML frontmatter delimiter shape.
- A **Member Hint Frontmatter** accepts only `name`, `description`, `role`, and `lane` inside `orbit_member`.
- User convention documentation may list **Member Hint Frontmatter** and **Directory Member Marker** fields because they are user-authored input.
- Applying content hints consumes a **Member Hint Frontmatter** by writing Orbit member truth and removing only the `orbit_member` mapping.
- Applying content hints preserves ordinary Markdown frontmatter metadata and deletes the whole frontmatter block only when removing `orbit_member` leaves it empty.
- A **Directory Member Marker** remains the canonical way to hint that a whole directory is one member, and it must use a nested `orbit_member` mapping.
- **Adoption** confirms member range, movement to **Recommended Positions**, and member roles as separate stages.
- **Adoption** may offer batch acceptance for recommended member roles, but must provide an individual role-edit path.
- **Adoption** presents **Recommended Position** moves for every adopted member candidate, but applies only the moves the user confirms.
- Local skills discovered during **Adoption** become local skill capability truth, not ordinary member-role content.
- Native agent config and hook definitions discovered during **Adoption** become runtime-level agent truth.
- Repository-local hook handlers discovered during **Adoption** belong to the **Adopted Orbit** rather than runtime-level config.
- **Adoption** chooses the recommended agent framework from project footprint, not merely globally installed tools.
- The first **Adoption** version fully supports Codex project assets and reports other detected agent footprints as unsupported for adoption.
- The first **Adoption** write set includes runtime manifest truth, one adopted orbit spec, root `AGENTS.md` marker block, Codex runtime agent truth when present, optional Codex config sidecars, and confirmed layout moves.
- The first **Adoption** version does not automatically create `.harness/vars.yaml` or create a Git commit.
- **Adoption** validates the generated runtime and reports next actions after writing, but does not automatically apply agent activation, publish templates, or create commits.
- A **Move Plan** is produced by repository-wide **Layout Optimization**, not by a package-scoped command.
- **Layout Optimization** runs from a repository root or working directory and infers the affected harness/orbit truth from the repository.
- **Layout Optimization** lives under `hyard layout optimize`; bare `hyard layout` is a command group, not the completing operation.
- **Layout Optimization** defaults to interactive confirmation; `--check` previews without mutation, and `--yes` applies the default recommendations without prompts when no conflicts block it.
- **Layout Optimization** supports both **Adoption** previews for an **Ordinary Repository** and ongoing optimization for an existing **Harness Runtime**.
- **Audit** is exposed as the top-level `hyard audit` command because it spans source, runtime, orbit-template, and harness-template revisions rather than orbit-only authoring.
- **Audit** is read-only; it does not prepare package content, apply layout moves, checkpoint changes, publish templates, or activate agent frameworks.
- **Audit** reports the detected revision kind and separates runtime audit results from package audit results.
- **Audit** inspects only the current Git worktree resolved from the command's working directory or explicit path; it does not scan all local branches, fetch remote branches, or checkout another revision.
- **Audit** fails closed with a `not_hyard_revision` status when the current Git worktree does not contain recognizable Harness Yard revision identity; ordinary repository discovery remains part of Adoption and Runtime Initialization flows.
- **Audit** status values are `pass`, `warn`, `fail`, and `not_hyard_revision`; advisory findings produce `warn`, blocking findings produce `fail`, and a finding-free audit produces `pass`.
- **Audit Findings** have their own taxonomy rather than reusing runtime-check finding kinds, though Audit may map runtime-check findings into audit-specific codes.
- **Audit** may report dirty worktree state as an advisory **Audit Finding**, but dirty tracked or untracked files do not by themselves make the audited Harness Yard revision invalid.
- **Audit** treats invalid or missing Harness Yard control-plane truth as blocking, while missing or untracked declared command and skill capability roots are advisory because they affect package usefulness and publish evidence rather than revision identity.
- **Audit** treats authored content member patterns that match no current tracked files as advisory unless they are required control-plane truth.
- **Audit** includes runtime check and readiness summaries when auditing a runtime revision, mapping their diagnostics into **Audit Findings** while keeping `hyard check` as the detailed runtime diagnostic command.
- **Audit** validates template installability for orbit-template and harness-template revisions through the existing template source loading contracts without applying the template to a runtime; installability failures are blocking.
- **Audit** does not require a source revision to be ready to publish as an orbit template; source publish readiness remains the job of `hyard orbit prepare <package> --check --json`.
- **Audit** audits every package declared in the current worktree revision by default rather than requiring a package argument.
- **Audit** JSON output includes both package-level result summaries and a flat finding list so humans can scan package health while scripts can filter stable findings.
- **Audit** exits with status code 0 for `pass` and `warn`, and non-zero for `fail`, `not_hyard_revision`, and internal command errors.
- **Audit** default text output is a human-readable summary; machine consumers should use `--json`.

## Example Dialogue

> **Dev:** "Can we extract this repository into a template?"
> **Domain expert:** "First adopt it into a Harness Runtime, then publish the Harness Template from that runtime."

> **Dev:** "AGENTS.md links to docs/architecture.md; should adoption make it a rule?"
> **Domain expert:** "Recommend rule, then ask me; if I decline, make me choose the correct member role or ignore it."

> **Agent:** "I found an existing **Bootstrap Guide**. Should I render first?"
> **Maintainer:** "No. Read the guide first; render is only for missing or
> recoverable guidance."

## Flagged Ambiguities

- "extract" was used to mean both detached template export and invasive runtime conversion; resolved: this feature is **Adoption**, and template export happens after adoption.
- "existing repository" in package-assembly demos could imply **Adoption**; resolved: use **Runtime Initialization** when the user only wants a Harness Runtime target for installing packages.
- "referenced docs" could mean a recursive documentation crawl; resolved: **Guidance Discovery** is conservative by default and asks before including all `docs/` directories.
- "source" in Adoption output could be confused with source branches and source authoring; resolved: adoption diagnostics should use terms like `derived_from` or `reason` instead.
- Adoption JSON avoids `source`; it uses `derived_from` for identity derivation and `evidence` for detection support.
- `hyard adopt source` could be confused with **Adoption**; resolved: **Source Adoption** is an authoring-revision conversion and does not create a Harness Runtime.
- "state" or "mode" could mean a **Harness Yard Revision** kind or a **Runtime View** presentation choice; resolved: use **Harness Yard Revision** for runtime/source/orbit-template/harness-template truth and reserve view language for Run View and Author View.
- "recommended Run View" could imply Run View is better for all roles; resolved: Run View is the default runtime-user presentation, while Author View is the expected mode for developers optimizing harness authored truth.
- "orbit name" could mean a display title or a stable package identity; resolved: **Source Adoption** defaults the Orbit Package id from the Git repository root directory name, while display naming stays optional or explicit.
- "add/remove" was considered for top-level package lifecycle, but it conflicts with scoped membership editing; resolved: top-level package lifecycle uses install/uninstall.
- "start" could be confused with installing packages or starting an application server; resolved: **Harness Start** is the agent handoff flow after a runtime already exists.
- "`harness1`" in demos could mean a local directory, branch, or registry entry; resolved: short names in public clone/install demos are **Package Handles** resolved through a **Package Registry**.
- "starting the agent" could mean selecting, activating, launching, or merely printing instructions; resolved: **Harness Start** launches an **Interactive Agent Session** after activation, or fails closed with manual next actions.
- "`start --with`" could mean a one-run override or a saved runtime preference; resolved: explicit **Harness Start** framework choices are saved repo-locally, with temporary start treated as an opt-out.
- **Harness Start** selection order could let a stale saved selection override the user's command; resolved: explicit `--with` wins over saved repo-local selection.
- "not affecting the global agent environment" could be treated as an advanced flag; resolved: **Harness Start** is project-only by default, and global routes require explicit user choice.
- "detected agent" could mean a desktop app, package, gateway, or CLI; resolved: **Harness Start** success requires a terminal-launchable framework, while non-launchable detections receive prompt and usage instructions.
- "launcher" could become scattered framework-specific process code; resolved: each **Agent Framework Launcher** declares its invocation contract, and **Harness Start** depends on that contract.
- "init skill" could be confused with ordinary skill dependencies; resolved: use **Bootstrap Agent Skill** for the framework-specific skill that drives runtime bootstrap.
- "Initialize the skill" previously implied that every bootstrap entry should run
  **Guide Render**. Resolution: bootstrap entry is discovery-first; render is a
  conditional recovery or authoring action.
- "Existing bootstrap guide" was briefly narrowed to owner-marked blocks.
  Resolution: a plain Run View `BOOTSTRAP.md` payload is also a **Bootstrap
  Guide** and should be executed during initialization.
- "Discover bootstrap" was briefly modeled as `hyard guide render --check`.
  Resolution: installed bootstrap skill discovery is a plain `BOOTSTRAP.md`
  existence check; render/check commands belong to authoring or recovery.
- "uninstall" could imply install provenance always exists; resolved: `hyard uninstall orbit` may remove a manually added orbit package, but must report its manual source.
- "uninstall" could imply detaching an active member while retaining hosted package truth; resolved: **Package Uninstallation** fully removes the package from the current Harness Runtime, and any retained evidence must not keep it installed, active, reapplicable, or readiness-relevant.
- A detached install record could be treated as uninstall evidence, but it keeps package provenance in the active install-record host; resolved: **Package Uninstallation** deletes the install record, and ordinary Git history carries uninstall evidence.
- Keeping a hosted OrbitSpec after uninstall could support later authoring, but it leaves package truth in the runtime host; resolved: **Package Uninstallation** deletes the hosted OrbitSpec unless another active package still owns or references it.
- "uninstall root guidance" could imply deleting presentation text by content matching; resolved: **Package Uninstallation** removes only marked root guidance blocks with unambiguous owner identity and leaves markerless **Run View Root Guidance** untouched.
- Untracked root guidance could be treated as dirty user work, but freshly installed packages often create untracked root guidance before checkpoint; resolved: **Package Uninstallation** uses marked block owner identity to remove target-owned content and preserve the rest.
- "files touched by an orbit" could mean projected, processed, edited, or owned files; resolved: **Package Uninstallation** deletes **Package-Owned Runtime Files**, not every file in an orbit's projection or work scope.
- Local Git changes on owned files could block uninstall, but that makes wrong-package cleanup depend on checkpoint timing; resolved: **Package Uninstallation** warns and asks for confirmation before deleting locally changed **Package-Owned Runtime Files**.
- Non-interactive uninstall could need a separate force flag for owned-file deletion, but that would split the package lifecycle confirmation model; resolved: `--yes` confirms deletion of target-owned files, and `--dry-run` previews the deletion set.
- Single-orbit uninstall could always prompt for safety, but that would make wrong-package rollback feel unlike package-manager uninstall; resolved: clean single-orbit uninstall may execute immediately, while local edits or broader side effects require confirmation.
- Package-owned file deletion could leave empty directories behind; resolved: **Package Uninstallation** removes empty directories caused by owned-file deletion while stopping at safe runtime boundaries.
- Harness package uninstall could be treated as bundle shrink or member detach, but that would not match package-manager uninstall; resolved: **Package Uninstallation** fully removes the installed Harness Package and its owned runtime surface.
- Compatibility `remove` could keep legacy detach/shrink behavior, but that would split package lifecycle semantics; resolved: top-level `remove` remains an alias for **Package Uninstallation**, while lower-level detach/shrink behavior belongs to plumbing or maintainer operations.
- Process-role files could be preserved because they are projection-only in older scoped operations; resolved: process-role files are deleted during **Package Uninstallation** when they are target-owned **Package-Owned Runtime Files**.
- "scope" could mean projection, write, export, orchestration, or ownership; resolved: uninstall deletion uses **Package Ownership Scope**, a package-derived ownership set distinct from behavior scopes.
- "workflow" could imply a generic CI workflow, arbitrary task graph, or a full Harness Runtime; resolved: public language may call an atomic closed-loop orbit package an **Orbit Workflow**, while internal and compatibility contracts remain orbit-based.
- "harness" alone could imply a runtime repository, reusable package, template revision, or compatibility command tree; resolved: use **Harness Runtime** or **Harness Package** for concrete product states, and use lowercase descriptive "harness" only as an umbrella for a composed agent work system.
- "content file" could include generated control-plane truth or runtime cache files; resolved: **Maintained Content File** means a human- or agent-maintained content or guidance file, not internal schema or repo-local cache.
- Orbit/source content could be documented only by current repository paths, but that would make layout decisions look semantic; resolved: document maintained content by responsibility first and list paths as recommended positions or examples.
- Bootstrap content could be mistaken for a normal member role; resolved: bootstrap is lifecycle metadata for initialization-only content, not a fifth member role.
- Content responsibility table "constraints" could be mistaken for hard validation requirements; resolved: these tables describe recommended content shape, while file presence is marked separately as Required, Conditional, or Optional.
- Root `AGENTS.md` could grow into a bootstrap procedure; resolved: steady-state root guidance assumes normal runtime operation, mentions initialization only when missing initialization blocks safe runtime work, and leaves detailed initialization to removable **Bootstrap Content** or bootstrap-oriented skills.
- `workflow="docs"` in public marker syntax could be misread as a display name distinct from the owner package id; resolved: marker `workflow` is a **Workflow ID** that must equal the underlying orbit id for `orbit:` blocks and the harness package id for `harness:` blocks.
- **Package Identity** could be split into separate user-facing orbit id and harness id concepts; resolved: the user-facing identity shape is package type plus package name, while root guidance markers use owner kind plus **Workflow ID** for block ownership.
- **Workflow ID** could be misread as a replacement for concrete orbit and harness identities; resolved: it is only their public root-guidance marker umbrella.
- `OwnerKind + WorkflowID` could be misread as replacing package identity throughout the repository; resolved: it is the generic identity for root guidance blocks only.
- The workflow naming change could sprawl into `.orbit-member.yaml`, `orbit_member:`, `.harness/orbits/*`, or `orbit-template/*`; resolved: the first compatibility design only changes root guidance marker syntax.
- Code-level `WorkflowID` naming could sprawl into all `OrbitID` fields; resolved: it applies only to root guidance block parser/render/replace/extract/remove APIs.
- Dual marker fields could make users think `workflow` and `orbit_id` have separate meanings; resolved: root guidance marker syntax uses only `workflow`, with no pre-release `orbit_id` compatibility requirement.
- Existing harness-owned guidance blocks may have been written as `orbit:` markers during pre-release development; resolved: no compatibility migration is required, and harness-owned blocks use `harness:` going forward.
- Flexible marker attribute parsing could accidentally allow loose comment parsing; resolved: root guidance marker parsing permits exactly one `workflow` attribute and rejects duplicates or unknown attributes.
- Flexible HTML comment parsing could accept too many marker shapes; resolved: root guidance markers stay strict, single-line, and double-quoted.
- Marker namespaces could become an accidental extension point; resolved: root guidance markers only allow `orbit:` and `harness:`.
- Root guidance marker uniqueness could accidentally block same-name orbit and harness packages; resolved: uniqueness is by marker namespace plus `workflow`, not by `workflow` alone.
- Generic workflow wording could make diagnostics less clear; resolved: user-facing messages prefer `orbit block` and `harness block`.
- "remove" remains valid for compatibility, but is no longer the preferred top-level package lifecycle term; resolved: docs and examples should guide users to `uninstall`.
- "uninstall" JSON could rename remove-shaped fields, but that would break machine consumers; resolved: keep existing result field names and optionally add an action field.
- Versioned package coordinates during uninstall could imply version matching; resolved: uninstall targets the installed package name only.
- "tool" in public registration discussions could imply arbitrary CLIs, plugins, services, or agent frameworks; resolved: public registry entries are **Orbit Package** or **Harness Package** entries exposed through **Package Handles**.
- A bare **Package Handle** such as `docs` could imply installing a mutable default branch or letting any author claim a global name; resolved: bare names are **Curated Handles** that point at namespaced handles, and default registry resolution selects a versioned, commit-pinned package locator.
- `latest` could mean highest SemVer, newest merged registry version, or source default branch; resolved: first-version registry resolution treats `latest` as an explicit dist-tag pointer.
- Registry resolution failures could fall back to guessed GitHub locators or mutable branches; resolved: registry-backed installation never guesses locators, exact-version cache use may not infer `latest`, stale cached bare or `latest` resolutions require a warning when the registry is unavailable, yanked packages require explicit override, and blocked packages cannot be installed.
- Public registration could mean hand-authoring complete registry records or using hosted account flows; resolved: the first product-side registration UX is generated registry entry candidates.
- Registry entry generation could be folded into `hyard publish`, but that would couple publication, remote verification, registry candidate output, and PR submission recovery; resolved: first-version candidate generation uses a separate `hyard registry entry` command.
- Registry candidates could be generated from local-only unpublished package results; resolved: submittable entries require remote reachability and package validation, while local-only output is preview-only.
- Hosting registry catalog data in the CLI source repository could mix third-party package submissions with product development; resolved: catalog data and registry review policy live in the official catalog repository, while this repository owns schema, resolver, product documentation, and package installation semantics.
- `@namespace/name` syntax could mirror npm scoped packages, but it conflicts with `@` as the version separator in Harness Yard coordinates; resolved: Package Handle Coordinates use `namespace/name[@version-or-tag]`.
- Top-level Markdown `name` frontmatter was used as a short content hint in older authoring docs, but it conflicts with ordinary document metadata; resolved: canonical **Member Hint Frontmatter** uses nested `orbit_member`, and **Flat Member Hint** is legacy.
- Strict Member Hint parsing could be misread as requiring frontmatter on every content Markdown file; resolved: only Markdown that declares **Member Hint Frontmatter** needs strict YAML frontmatter.
- Allowing `paths` in **Member Hint Frontmatter** would make a hint describe content somewhere else; resolved: member paths are derived from the hint location, and arbitrary paths stay in OrbitSpec member truth.
- `meta` is a valid OrbitSpec member role, but allowing it in **Member Hint Frontmatter** would let ordinary Markdown content declare control-plane truth; resolved: Member Hint roles are limited to ordinary content roles.
- Member lanes could become an open-ended lifecycle taxonomy; resolved: **Member Hint Frontmatter** only accepts the existing `bootstrap` lane.
- "rule" could mean the `rule` member role or the orbit-level behavior that decides scopes; resolved: use **Behavior Scope Defaults** for role-to-scope decisions.
- "`--with-spec` rule directory" could be confused with **Directory Member Marker** role defaults; resolved: it changes the OrbitSpec members created by `--with-spec` and does not change the default role for directory member markers.
- Scope overrides in **Member Hint Frontmatter** could make temporary hints carry durable behavior policy; resolved: the canonical Member Hint contract does not include `scopes`.
- Directory-level hints could be removed while tightening Markdown frontmatter, but that would lose the whole-directory member authoring path; resolved: keep **Directory Member Marker** and apply the same nested `orbit_member` shape.
- **Flat Member Hint** compatibility would preserve old authoring examples but keep document metadata ambiguous; resolved: do not support the old flat hint shape, and treat flat-looking frontmatter as ordinary Markdown metadata unless nested `orbit_member` is present.
- Ordinary Markdown frontmatter could be mistaken for Harness Yard truth; resolved: ignore frontmatter outside nested `orbit_member`, and never delete unrelated metadata during content hint application.
- Invalid **Member Hint Frontmatter** could be ignored as ordinary metadata, but that would hide a failed member declaration; resolved: nested `orbit_member` intent makes malformed frontmatter an invalid hint.
- Strict YAML frontmatter delimiters could reject CRLF-authored files unexpectedly; resolved: normalize CRLF before applying the delimiter contract.
- Unknown Member Hint fields could appear to take effect when they are ignored; resolved: unknown fields inside `orbit_member` are invalid.

## Open Questions

- Should Harness Yard eventually expose a dedicated bootstrap discovery command
  if a plain `BOOTSTRAP.md` existence check becomes too weak?
- How should bootstrap closeout messaging describe a plain Run View
  `BOOTSTRAP.md` payload when no owner-marked bootstrap block is removed?
