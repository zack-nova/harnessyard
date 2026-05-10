# Demo: Codebase Developer Composes A Runtime

This demo shows a codebase developer acting as a Harness Author in an existing
repository. The developer initializes a Harness Runtime, installs a few small
Orbit Packages, previews different Agent Framework handoffs, edits Run View
guidance, and publishes the whole runtime as a Harness Package.

This path stays in Run View. It does not enter Author View and does not publish
an Orbit Package.

## Step 1: Start In An Existing Repository

```bash
mkdir existing-app
cd existing-app
git init -b main
printf '# Existing App\n' > README.md
git add README.md
git commit -m "Create app"
```

Expected result:

- You have an ordinary Git repository with one normal commit.
- The repository is not yet a Harness Runtime.

Terminal display:

```text
Initialized empty Git repository in .../existing-app/.git/
[main (root-commit) 3d7757f] Create app
```

## Step 2: Initialize Runtime Metadata

```bash
hyard init runtime --json
```

Expected result:

- `.harness/manifest.yaml` is created.
- `.harness/orbits/` is created.
- The existing repository becomes a Harness Runtime.

Terminal display:

```json
{
  "manifest_created": true,
  "orbits_dir_created": true
}
```
## Step 3: Install Three Orbit Workflows

```bash
hyard install https://github.com/zack-nova/hyard-demo-docs-orbit.git --ref orbit-template/docs --json
hyard install https://github.com/zack-nova/hyard-demo-review-orbit.git --ref orbit-template/review --json
hyard install https://github.com/zack-nova/hyard-demo-release-orbit.git --ref orbit-template/release --json
```

Expected result:

- `docs`, `review`, and `release` are installed as Orbit Packages.
- `AGENTS.md`, `HUMANS.md`, and `docs/**` runtime content are written.
- Each install leaves the runtime ready.

Terminal display:

```json
{
  "orbit_id": "docs",
  "written_paths": [
    ".harness/installs/docs.yaml",
    ".harness/orbits/docs.yaml",
    "AGENTS.md",
    "HUMANS.md",
    "docs/docs.md"
  ],
  "readiness": {
    "status": "ready"
  }
}
```

Repeat output appears for `review` and `release`.

## Step 4: Preview Different Agent Frameworks

Preview Codex:

```bash
hyard start --with codex --dry-run --json
```

Terminal display:

```json
{
  "framework_resolution": {
    "status": "resolved",
    "selected_framework": "codex"
  },
  "activation": {
    "status": "planned",
    "route": "project",
    "framework": "codex"
  },
  "launcher": {
    "status": "launchable",
    "launchable": true
  }
}
```

Preview Claude Code:

```bash
hyard start --with claudecode --dry-run --json
```

Expected result:

- Harness Yard resolves the explicit framework choice.
- Project-local activation is planned.
- If a launcher is not implemented for that framework, Harness Yard returns
  manual fallback instructions instead of pretending it can launch.

Terminal display:

```json
{
  "framework_resolution": {
    "selected_framework": "claudecode"
  },
  "activation": {
    "route": "project",
    "framework": "claudecode"
  },
  "launcher": {
    "status": "unsupported",
    "launchable": false,
    "manual_fallback_instructions": [
      "Run `hyard start --print-prompt` and paste the Start Prompt into the selected agent."
    ]
  }
}
```

Use the live command when you want the actual handoff:

```bash
hyard start --with codex
```

## Step 5: Tune Run View Guidance

Edit `AGENTS.md` directly when you are tuning runtime presentation:

```bash
$EDITOR AGENTS.md
hyard view status --json
```

Expected result:

- The runtime remains in Run View.
- Publication is still the current runtime as a Harness Package.

Terminal display:

```json
{
  "selected_view": "run",
  "allowed_publication_actions": [
    "current_runtime_harness_package"
  ],
  "runtime": {
    "member_ids": ["docs", "release", "review"],
    "member_count": 3
  }
}
```

## Step 6: Commit And Publish The Harness Package

```bash
git status --short
git add .
git commit -m "Assemble product lab harness"
hyard publish harness product-lab-custom --json
```

Expected result:

- Normal Git history records the runtime change.
- `hyard publish harness` creates `harness-template/product-lab-custom`.
- Run View never publishes an Orbit Package by default.

Terminal display:

```json
{
  "package_name": "product-lab-custom",
  "branch": "harness-template/product-lab-custom",
  "source_branch": "main",
  "local_publish": {
    "success": true,
    "changed": true
  }
}
```
