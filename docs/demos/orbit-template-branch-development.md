# Demo: Develop An Orbit Template Branch

This demo shows the dedicated single-orbit package repository path. The source
branch owns authored truth for one Orbit Package, and publication writes the
installable `orbit-template/<package>` branch.

The demo uses the public Docs Orbit fixture:

```text
https://github.com/zack-nova/hyard-demo-docs-orbit.git
source branch: main
template branch: orbit-template/docs
```

## Step 1: Clone The Orbit Package Repository

```bash
git clone https://github.com/zack-nova/hyard-demo-docs-orbit.git
cd hyard-demo-docs-orbit
```

Expected result:

- `main` contains Source Revision authored truth.
- The repository owns exactly one Orbit Package: `docs`.

Terminal display:

```text
Cloning into 'hyard-demo-docs-orbit'...
```

## Step 2: Inspect The Authoring Revision

```bash
hyard audit
hyard orbit prepare docs --check --json
```

Expected result:

- `hyard audit` identifies a Source Revision.
- `hyard orbit prepare` reports the package is ready or gives concrete next
  actions.

Terminal display:

```text
repo_root: .../hyard-demo-docs-orbit
status: warn
revision_kind: source
packages:
  - type=orbit name=docs revision_role=source orbit_id=docs
```

The fixture may warn about empty optional command or local skill capability
paths. Those warnings are acceptable for this minimal demo package.

```json
{
  "ready": true,
  "next_actions": []
}
```

## Step 3: Edit The Orbit Workflow

```bash
$EDITOR docs/docs.md
$EDITOR docs/docs/README.md
hyard guide save --orbit docs --target all --json
```

Expected result:

- The durable rule document changes on `main`.
- Root guidance is saved back into `.harness/orbits/docs.yaml` when you edited
  root guidance blocks.
- The repository still describes exactly one Orbit Package.

Terminal display:

```json
{
  "target": "all",
  "artifact_count": 2
}
```

## Step 4: Commit The Source Change

```bash
git status --short
git add .
git commit -m "Update docs orbit rules"
```

Expected result:

- Normal Git history records the authored truth change.
- The source branch remains the editable long-lived branch.

Terminal display:

```text
[main 2f1a9cc] Update docs orbit rules
```

## Step 5: Publish The Orbit Template Branch

```bash
hyard publish orbit docs --json
```

Expected result:

- `hyard publish orbit` writes the installable template branch.
- The branch is `orbit-template/docs`.
- Runtime users can install that branch into any Harness Runtime.

Terminal display:

```json
{
  "branch": "orbit-template/docs",
  "source_branch": "main",
  "local_publish": {
    "success": true,
    "changed": true
  }
}
```

If you have push permission and want to update the remote template branch:

```bash
hyard publish orbit docs --push --remote origin --json
```

Expected result:

- `origin/orbit-template/docs` is updated.
- Users can install the registered template by handle after the registry entry
  or dist-tag has been updated to the new commit:

```bash
hyard install docs
```

Until that registry update exists, the explicit Git locator remains the advanced
escape hatch for unpublished package revisions.
