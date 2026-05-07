# Local Markdown Adapter

Use this adapter when issues live as markdown files in the repository.

## Mapping

```yaml
backend: local-markdown

issue:
  id_format: "<path>"
  default_issue_path_pattern: ".scratch/issues/<number>-<slug>.md"
  optional_feature_issue_path_pattern: ".scratch/<feature-slug>/issues/<number>-<slug>.md"
  state:
    representation: frontmatter
    field: state
  type:
    representation: frontmatter
    field: type
  metadata:
    priority:
      representation: frontmatter
      field: priority
    size:
      representation: frontmatter
      field: size
    blocked:
      representation: frontmatter
      field: blocked
    resolution:
      representation: frontmatter
      field: resolution

sections:
  storage: markdown_section
  headings:
    triage-notes: "## Triage Notes"
    dev-brief: "## Dev Brief"
    dev-workpad: "## Dev Workpad"
    review-sweep: "## Review Sweep"
    human-review-decision: "## Human Review Decision"
    debt-notes: "## Debt Notes"

review_artifact:
  storage: local_markdown_file
  default_path_pattern: ".scratch/reviews/<issue-number>-<slug>-review.md"
```

## Local Review Artifact

Local markdown does not simulate a full PR platform. It simulates the review gate and review evidence only.

A local review artifact records:

- issue path,
- branch,
- base ref,
- diff command or commit range,
- validation commands and results,
- review status,
- human decision.

It does not simulate inline PR comments, checks UI, branch protections, or merge queues.

## Operations

- Create issue: create a markdown file from `templates/backends/local-markdown/issue.template.md`.
- Read issue: parse frontmatter and canonical markdown sections.
- Update state or metadata: edit frontmatter.
- Add or update issue section: edit the matching markdown heading.
- Create review artifact: create a markdown file from `templates/backends/local-markdown/review-artifact.template.md`.

Bootstrap must choose the concrete issue path pattern and review artifact path pattern for the repository.
