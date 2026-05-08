# Local Markdown Adapter

Use this adapter when issues live as markdown files in the repository.

## Mapping

```yaml
backend: local-markdown

issue:
    id_format: '<path>'
    open_issue_path_pattern: '.scratch/issues/open/<number>-<slug>.md'
    closed_issue_path_pattern: '.scratch/issues/closed/<number>-<slug>.md'
    optional_feature_open_issue_path_pattern: '.scratch/<feature-slug>/issues/open/<number>-<slug>.md'
    optional_feature_closed_issue_path_pattern: '.scratch/<feature-slug>/issues/closed/<number>-<slug>.md'
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
        delivery_mode:
            representation: frontmatter
            field: delivery_mode
        resolution:
            representation: frontmatter
            field: resolution

sections:
    storage: markdown_section
    headings:
        triage-notes: '## Triage Notes'
        dev-brief: '## Dev Brief'
        dev-workpad: '## Dev Workpad'
        review-sweep: '## Review Sweep'
        human-review-decision: '## Human Review Decision'
        debt-notes: '## Debt Notes'
        out-of-scope-catalog: '## Out-of-Scope Catalog'

review_artifact:
    storage: local_markdown_file
    default_path_pattern: '.scratch/reviews/<issue-number>-<slug>-review.md'
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
- AFK / HITL assessment,
- human decision when the issue is in `human-review` or delivery mode is `hitl`.

It does not simulate inline PR comments, checks UI, branch protections, or merge queues.

## Operations

- Create delivery issue: create a markdown file from `templates/backends/local-markdown/issue.template.md` in the configured open issue folder.
- Create out-of-scope catalog issue: create a markdown file from `templates/backends/local-markdown/out-of-scope-catalog.template.md` in the configured open issue folder.
- Read issue: parse frontmatter and canonical markdown sections.
- Update state or metadata: edit frontmatter. If delivery mode is used, set `delivery_mode` to `afk` or `hitl`.
- Add or update issue section: edit the matching markdown heading.
- Read out-of-scope catalog: parse issues with type `out-of-scope`, state `cancelled`, and the `## Out-of-Scope Catalog` heading.
- Close issue: move the markdown file from the configured open issue folder to the configured closed issue folder.
- Create review artifact: create a markdown file from `templates/backends/local-markdown/review-artifact.template.md`.

Bootstrap must choose the concrete open issue path pattern, closed issue path pattern, and review artifact path pattern for the repository.
