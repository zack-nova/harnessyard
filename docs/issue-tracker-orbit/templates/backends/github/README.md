# GitHub Backend Templates

These files are reference issue and PR templates for the GitHub backend. They are not installed directly into the repository root.

During Bootstrap, safely install or merge them into:

```text
.github/ISSUE_TEMPLATE/
.github/pull_request_template.md
```

If target files already exist, show the diff and wait for human maintainer confirmation.

These templates use GitHub labels to represent the core canonical state role and issue type:

```text
state:<role>
type:<type>
```

If the repository enables delivery mode metadata, use optional labels:

```text
delivery-mode:afk
delivery-mode:hitl
```

`out-of-scope-catalog.yml` is a special catalog issue template. It creates an issue with `state:cancelled` and `type:out-of-scope` and uses the issue body for `## Out-of-Scope Catalog` instead of delivery sections. Install the template during bootstrap, but create the actual catalog issue lazily when the maintainer enables the catalog or the first out-of-scope decision needs to be recorded.
