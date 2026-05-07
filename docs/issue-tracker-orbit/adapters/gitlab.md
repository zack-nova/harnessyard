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
    blocked:
      representation: label
      prefix: "blocked:"
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

review_artifact:
  storage: gitlab_merge_request
  link_rule: closing_reference
```

## Rules

- GitLab labels carry canonical state, type, and optional metadata facts.
- GitLab issue notes carry canonical issue sections.
- GitLab merge requests are the review artifact path for review and land.
- GitLab branch protection, required pipelines, approvals, and merge settings remain GitLab-side policy.
- GitLab calls comments notes and PRs merge requests. The core contract terms remain issue section and review artifact.

Concrete commands and API clients belong to the contract consumer, tool, or human process that performs the work.
