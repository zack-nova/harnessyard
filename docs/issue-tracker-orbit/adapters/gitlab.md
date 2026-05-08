# GitLab Adapter

Use this adapter when issues live in GitLab Issues. Use the `glab` CLI for tracker operations.

## Mapping

```yaml
backend: gitlab

issue:
  id_format: "#<iid>"
  url_format: "https://<gitlab-host>/<namespace>/<project>/-/issues/<iid>"
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
  storage: issue_note
  headings:
    triage-notes: "## Triage Notes"
    dev-brief: "## Dev Brief"
    dev-workpad: "## Dev Workpad"
    review-sweep: "## Review Sweep"
    human-review-decision: "## Human Review Decision"
    debt-notes: "## Debt Notes"
    out-of-scope-catalog: "## Out-of-Scope Catalog"
  storage_overrides:
    out-of-scope-catalog: issue_description

review_artifact:
  storage: gitlab_merge_request
  link_rule: closing_reference
```

## Rules

- GitLab labels carry canonical state, type, and optional metadata facts.
- If delivery mode is used, GitLab labels should be `delivery-mode:afk` or `delivery-mode:hitl`.
- GitLab issue notes carry delivery issue sections.
- GitLab issue description carries the Out-of-Scope Catalog section for issues with type `out-of-scope`.
- GitLab merge requests are the review artifact path for review and land.
- GitLab branch protection, required pipelines, approvals, and merge settings remain GitLab-side policy.
- GitLab calls comments notes and PRs merge requests. The core contract terms remain issue section and review artifact.

Concrete commands and API clients belong to the contract consumer, tool, or human process that performs the work.
