# Demo: Harness Developer Authors Orbits In One Runtime

This demo shows a developer using one Harness Runtime in Author View as both a
Harness Author and an Orbit Author. The developer initializes a runtime, creates
new hosted Orbit authored truth, links a remote skill dependency as authored
truth, publishes the Orbit Package, and publishes the composed Harness Package.

This is allowed when the same repository is intentionally used as the authoring
workspace for both the Orbit Workflow and the composed harness.

## Step 1: Initialize A Runtime And Enter Author View

```bash
mkdir harness-authoring
cd harness-authoring
git init -b main
printf '# Harness Authoring Demo\n' > README.md
git add README.md
git commit -m "Create base repository"

hyard init runtime --json
hyard view author --json
```

Expected result:

- The repository becomes a Harness Runtime.
- Author View is selected.
- Author View exposes Orbit Package publication and Harness Package publication
  as next actions.

Terminal display:

```json
{
  "selected_view": "author",
  "next_actions": [
    "render editable guidance with `hyard guide render`",
    "publish an Orbit Package",
    "publish current runtime as a Harness Package"
  ]
}
```
## Step 2: Create An Orbit With A Spec Entry Point

```bash
hyard orbit create onboarding \
  --with-spec \
  --name "Onboarding Orbit" \
  --description "Prepare newcomer onboarding notes." \
  --json
```

Expected result:

- `.harness/orbits/onboarding.yaml` is created.
- `docs/onboarding.md` and `docs/onboarding/README.md` are created.
- The runtime now has hosted authored truth for the `onboarding` Orbit Package.

Terminal display:

```json
{
  "schema": "members",
  "file": ".../.harness/orbits/onboarding.yaml",
  "orbit": {
    "ID": "onboarding",
    "Name": "Onboarding Orbit"
  }
}
```

Now write the workflow rules:

```bash
$EDITOR docs/onboarding.md
$EDITOR docs/onboarding/README.md
```

Expected result:

- `docs/onboarding.md` states the workflow objective, scope, done probe, and
  handoff.
- `docs/onboarding/README.md` stays a short entry point.

## Step 3: Link A Remote Skill Dependency

Remote skill links write authored Orbit truth. Current Harness Yard can diagnose
the link, but remote skill pinning and materialization are not a stable runtime
contract yet.

```bash
hyard orbit skill link https://github.com/zack-nova/hyard-demo-docs-orbit \
  --orbit onboarding \
  --json
```

Expected result:

- The remote skill URI is recorded in `.harness/orbits/onboarding.yaml`.
- Framework activation remains under `hyard agent` and `hyard start`.

Terminal display:

```json
{
  "orbit": "onboarding",
  "uri": "https://github.com/zack-nova/hyard-demo-docs-orbit",
  "required": false
}
```

Inspect the link:

```bash
hyard orbit skill inspect --orbit onboarding
```

Terminal display:

```text
orbit: onboarding
remote skill: https://github.com/zack-nova/hyard-demo-docs-orbit (recommended)
```

## Step 4: Save Guidance, Commit, And Publish The Orbit Package

```bash
hyard guide save --orbit onboarding --target all --json
git add .
git commit -m "Author onboarding orbit"
hyard publish orbit onboarding --json
```

Expected result:

- Root guidance edits are saved back into authored truth when present.
- Git records the authored Orbit truth.
- `hyard publish orbit` creates `orbit-template/onboarding`.

Terminal display:

```json
{
  "branch": "orbit-template/onboarding",
  "source_branch": "main",
  "local_publish": {
    "success": true,
    "changed": true
  }
}
```

## Step 5: Publish The Composed Harness Package

```bash
hyard publish harness onboarding-lab --json
```

Expected result:

- The same repository also publishes a Harness Package.
- `hyard publish harness` creates `harness-template/onboarding-lab`.

Terminal display:

```json
{
  "branch": "harness-template/onboarding-lab",
  "source_branch": "main",
  "local_publish": {
    "success": true,
    "changed": true
  }
}
```
