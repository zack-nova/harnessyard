# Demo: Runtime User Fast Start

This demo shows the shortest Runtime User path: clone a published Harness
Template, inspect it, and hand off to an Agent Framework.

The demo uses the public Product Lab fixture:

```text
https://github.com/zack-nova/hyard-demo-product-lab.git
ref: harness-template/product-lab
```

## Step 1: Clone The Harness Template

Run this from the directory where you want the demo runtime folder created:

```bash
hyard clone https://github.com/zack-nova/hyard-demo-product-lab.git product-lab --ref harness-template/product-lab
cd product-lab
```

Expected result:

- A new Harness Runtime appears in `product-lab`.
- The runtime contains the `docs`, `review`, and `release` Orbit Workflows.
- The next action is Harness Start.

Terminal display:

```json
{
  "harness_id": "product-lab",
  "member_ids": ["docs", "release", "review"],
  "member_count": 3,
  "readiness": {
    "status": "ready"
  },
  "next_actions": [
    {
      "kind": "harness_start",
      "command": "hyard start"
    }
  ]
}
```
## Step 2: Check Runtime Readiness

```bash
hyard check --json
hyard view status --json
```

Expected result:

- `hyard check` reports no findings.
- Run View is selected.
- Run View allows publishing the current runtime as a Harness Package.

Terminal display:

```json
{
  "ok": true,
  "finding_count": 0,
  "readiness": {
    "status": "ready"
  }
}
```

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

## Step 3: Start With Codex

Use the live command for the actual demo:

```bash
hyard start --with codex
```

Expected result:

- Harness Start resolves the Codex Agent Framework from the explicit `--with`
  choice.
- Harness Yard applies project-local agent activation.
- Harness Yard installs the bootstrap agent skill when needed.
- Harness Yard launches or hands off to an interactive Codex session.

Terminal display:

```text
harness start handed off to codex
```

For a non-launching preview, use:

```bash
hyard start --with codex --dry-run --json
```

Terminal display:

```json
{
  "dry_run": true,
  "framework_resolution": {
    "status": "resolved",
    "selected_framework": "codex",
    "selection_source": "explicit_local"
  },
  "activation": {
    "status": "planned",
    "route": "project",
    "framework": "codex"
  },
  "launcher": {
    "framework": "codex",
    "status": "launchable",
    "launchable": true
  }
}
```
