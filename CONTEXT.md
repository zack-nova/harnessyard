# Harness Yard

Harness Yard turns ordinary Git repositories and authored package truth into reusable harness runtimes and templates for agent-assisted work.

## Language

**Harness Runtime**:
A Git repository that has Harness Yard control-plane truth and can install, activate, check, and publish harness content.
_Avoid_: workspace, plain repo

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

**Package Handle**:
A registry-facing short name that users pass to package lifecycle commands.
_Avoid_: branch ref, local folder, display title

**Harness Workflow Block**:
A root guidance block owned by a Harness Package and written with public workflow marker language.
_Avoid_: orbit block, generic workflow

**Workflow ID**:
The public marker identity field that names either an Orbit Package owner or a Harness Package owner.
_Avoid_: display alias, renamed orbit id

**Workflow Owner Kind**:
The public marker owner category that says whether a root guidance block is owned by an Orbit Package or a Harness Package.
_Avoid_: arbitrary namespace, replacement package type

**Package Installation**:
The lifecycle operation that applies an Orbit Package or Harness Package to a Harness Runtime with package provenance.
_Avoid_: add

**Package Uninstallation**:
The lifecycle operation that removes an installed Orbit Package or Harness Package from a Harness Runtime.
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
- A **Harness Runtime** may publish one or more **Harness Templates** over time.
- A **Harness Template** is exported from a **Harness Runtime**, not directly from an **Ordinary Repository**.
- Cloning a **Harness Template** should suggest **Harness Start** as the next handoff action but should not start an agent automatically.
- Early harness optimization demos may use manual Git checkpoints before publishing a Harness Template.
- A **Package Registry** lets public commands resolve **Package Handles** without exposing Git branch locators in ordinary demos.
- Early demos may use explicit GitHub package locators before **Package Registry** resolution is ready.
- A **Harness Runtime** may install and uninstall **Orbit Packages** and **Harness Packages** through package lifecycle commands.
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
- A **Harness Start** defaults to project-only **Framework Activation** and must not write the user's global agent environment unless the user explicitly chooses a global route.
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
- Package type ambiguity during **Package Uninstallation** is never guessed; the user must choose `orbit` or `harness` explicitly.
- A **Package Uninstallation** may target a manually added **Orbit Package**, but command output must disclose the manual source because no install provenance exists.
- Uninstalling a **Harness Package** removes that harness package and its included orbit packages as one package lifecycle operation.
- Uninstalling one **Orbit Package** may execute immediately, while uninstalling a **Harness Package** needs preview and confirmation support because it can remove multiple orbits and global agent outputs.
- Top-level `remove` remains a compatibility alias for **Package Uninstallation**, but documentation and user-facing examples should prefer `uninstall`.
- The compatibility `remove` surface stays callable for existing scripts but should be hidden from top-level help.
- The compatibility `remove` surface may keep its own help output, but that help should identify `uninstall` as the preferred command.
- Package uninstallation JSON should preserve existing remove-shaped result fields for compatibility, even when the canonical command name is `uninstall`.
- Human-readable output from the canonical `uninstall` surface should use `uninstalled` language.
- Preview and confirmation output from the canonical `uninstall` surface should describe targets as items to uninstall.
- Package uninstallation error guidance should prefer `uninstall` commands, including when reached through the compatibility `remove` surface.
- The `uninstall` and compatibility `remove` command surfaces should share one package-uninstallation implementation so lifecycle semantics do not drift.
- **Runtime View** selection changes presentation and publication defaults, not package identity or canonical authored truth.
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

## Flagged Ambiguities

- "extract" was used to mean both detached template export and invasive runtime conversion; resolved: this feature is **Adoption**, and template export happens after adoption.
- "existing repository" in package-assembly demos could imply **Adoption**; resolved: use **Runtime Initialization** when the user only wants a Harness Runtime target for installing packages.
- "referenced docs" could mean a recursive documentation crawl; resolved: **Guidance Discovery** is conservative by default and asks before including all `docs/` directories.
- "source" in Adoption output could be confused with source branches and source authoring; resolved: adoption diagnostics should use terms like `derived_from` or `reason` instead.
- Adoption JSON avoids `source`; it uses `derived_from` for identity derivation and `evidence` for detection support.
- `hyard adopt source` could be confused with **Adoption**; resolved: **Source Adoption** is an authoring-revision conversion and does not create a Harness Runtime.
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
- "uninstall" could imply install provenance always exists; resolved: `hyard uninstall orbit` may remove a manually added orbit package, but must report its manual source.
- "workflow" could imply a generic CI workflow, arbitrary task graph, or a full Harness Runtime; resolved: public language may call an atomic closed-loop orbit package an **Orbit Workflow**, while internal and compatibility contracts remain orbit-based.
- `workflow="docs"` in public marker syntax could be misread as a display name distinct from the owner package id; resolved: marker `workflow` is a **Workflow ID** that must equal the underlying orbit id for `orbit:` blocks and the harness package id for `harness:` blocks.
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
- Top-level Markdown `name` frontmatter was used as a short content hint in older authoring docs, but it conflicts with ordinary document metadata; resolved: canonical **Member Hint Frontmatter** uses nested `orbit_member`, and **Flat Member Hint** is legacy.
- Strict Member Hint parsing could be misread as requiring frontmatter on every content Markdown file; resolved: only Markdown that declares **Member Hint Frontmatter** needs strict YAML frontmatter.
- Allowing `paths` in **Member Hint Frontmatter** would make a hint describe content somewhere else; resolved: member paths are derived from the hint location, and arbitrary paths stay in OrbitSpec member truth.
- `meta` is a valid OrbitSpec member role, but allowing it in **Member Hint Frontmatter** would let ordinary Markdown content declare control-plane truth; resolved: Member Hint roles are limited to ordinary content roles.
- Member lanes could become an open-ended lifecycle taxonomy; resolved: **Member Hint Frontmatter** only accepts the existing `bootstrap` lane.
- "rule" could mean the `rule` member role or the orbit-level behavior that decides scopes; resolved: use **Behavior Scope Defaults** for role-to-scope decisions.
- Scope overrides in **Member Hint Frontmatter** could make temporary hints carry durable behavior policy; resolved: the canonical Member Hint contract does not include `scopes`.
- Directory-level hints could be removed while tightening Markdown frontmatter, but that would lose the whole-directory member authoring path; resolved: keep **Directory Member Marker** and apply the same nested `orbit_member` shape.
- **Flat Member Hint** compatibility would preserve old authoring examples but keep document metadata ambiguous; resolved: do not support the old flat hint shape, and fail closed with guidance to use nested `orbit_member`.
- Ordinary Markdown frontmatter could be mistaken for Harness Yard truth; resolved: ignore frontmatter outside nested `orbit_member`, and never delete unrelated metadata during content hint application.
- Invalid **Member Hint Frontmatter** could be ignored as ordinary metadata, but that would hide a failed member declaration; resolved: nested `orbit_member` intent makes malformed frontmatter an invalid hint.
- Strict YAML frontmatter delimiters could reject CRLF-authored files unexpectedly; resolved: normalize CRLF before applying the delimiter contract.
- Unknown Member Hint fields could appear to take effect when they are ignored; resolved: unknown fields inside `orbit_member` are invalid.
