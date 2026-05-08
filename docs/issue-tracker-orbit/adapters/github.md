# GitHub Adapter

Use this adapter when issues live in GitHub Issues. Use the `gh` CLI for tracker operations.

## Mapping

```yaml
backend: github

issue:
  id_format: "#<number>"
  url_format: "https://github.com/<owner>/<repo>/issues/<number>"
  state:
    representation: label
    prefix: "state:"
  type:
    representation: label
    prefix: "type:"
  metadata:
    priority:
      representation: label
      prefix: "priority:"
    size:
      representation: label
      prefix: "size:"
    delivery_mode:
      representation: label
      prefix: "delivery-mode:"
    resolution:
      representation: label
      prefix: "resolution:"

sections:
  storage: issue_comment
  headings:
    triage-notes: "## Triage Notes"
    dev-brief: "## Dev Brief"
    dev-workpad: "## Dev Workpad"
    review-sweep: "## Review Sweep"
    human-review-decision: "## Human Review Decision"
    debt-notes: "## Debt Notes"
    out-of-scope-catalog: "## Out-of-Scope Catalog"
  storage_overrides:
    out-of-scope-catalog: issue_body

review_artifact:
  storage: github_pull_request
  link_rule: closing_reference

templates:
  issue_templates: ".github/ISSUE_TEMPLATE/"
  pull_request_template: ".github/pull_request_template.md"
```

## Rules

- GitHub labels carry canonical state, type, and optional metadata facts.
- If delivery mode is used, GitHub labels should be `delivery-mode:afk` or `delivery-mode:hitl`.
- GitHub issue comments carry delivery issue sections.
- GitHub issue body carries the Out-of-Scope Catalog section for issues with type `out-of-scope`.
- GitHub pull requests are the review artifact path for review and land.
- GitHub branch protection, required checks, and merge settings remain GitHub-side policy.
- Bootstrap must dry-run label and template changes before applying them.

Concrete commands and API clients belong to the contract consumer, tool, or human process that performs the work.
