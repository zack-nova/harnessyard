# Tracker Contract

This file is generated or completed during bootstrap. It is the contract-consumer-readable source of truth for the repository's selected issue tracker backend and runtime rule mappings.

If `status` is `pending-bootstrap`, complete `BOOTSTRAP.md` first.

```yaml
version: 1
status: active
backend: github

repository:
    id: zack-nova/harnessyard
    default_branch: main

issue:
    id_format: "#<number>"
    state:
        required: true
        cardinality: exactly_one
        representation: label
        prefix: "state:"
        values:
            needs-triage: "state:needs-triage"
            needs-info: "state:needs-info"
            ready-for-dev: "state:ready-for-dev"
            in-progress: "state:in-progress"
            in-review: "state:in-review"
            human-review: "state:human-review"
            to-rework: "state:to-rework"
            to-merge: "state:to-merge"
            merged: "state:merged"
    type:
        required: true
        cardinality: exactly_one
        representation: label
        prefix: "type:"
        values:
            bug: "type:bug"
            feature: "type:feature"
            task: "type:task"
            docs: "type:docs"
            chore: "type:chore"
    metadata:
        priority:
            required: false
            representation: label
            prefix: "priority:"
            values: []
        size:
            required: false
            representation: label
            prefix: "size:"
            values: []
        blocked:
            required: false
            representation: label
            prefix: "blocked:"
            blocks_advancement: true
            values: []
        resolution:
            required: false
            representation: label
            prefix: "resolution:"
            values: []

sections:
    triage-notes:
        storage: issue_comment
        heading: '## Triage Notes'
    dev-brief:
        storage: issue_comment
        heading: '## Dev Brief'
    dev-workpad:
        storage: issue_comment
        heading: '## Dev Workpad'
    review-sweep:
        storage: issue_comment
        heading: '## Review Sweep'
    human-review-decision:
        storage: issue_comment
        heading: '## Human Review Decision'
    debt-notes:
        storage: issue_comment
        heading: '## Debt Notes'

review_artifact:
    required: true

backend_mapping:
    github:
        issue_id: "#<number>"
        issue_url: "https://github.com/zack-nova/harnessyard/issues/<number>"
        issue_state: "exactly one GitHub label with prefix state:"
        issue_type: "exactly one GitHub label with prefix type:"
        optional_metadata:
            priority: "zero or one GitHub label with prefix priority:"
            size: "zero or one GitHub label with prefix size:"
            blocked: "zero or one GitHub label with prefix blocked:"
            resolution: "zero or one GitHub label with prefix resolution:"
        sections: "GitHub issue comments containing the configured canonical headings"
        review_artifact:
            storage: github_pull_request
            link_rule: closing_reference
        templates:
            issue_templates: ".github/ISSUE_TEMPLATE/"
            pull_request_template: ".github/pull_request_template.md"
        metadata_sync:
            required_dry_run_before_apply: true

validation:
    commands:
        - "mise run fix"
        - "mise run ci"
    required_before_review: true
    required_before_land: true

merge:
    method: squash
    delete_branch: true

safety:
    dry_run_required_for:
        - batch_issue_edits
        - batch_metadata_changes
        - template_install_or_overwrite
        - branch_push
        - review_artifact_create
        - merge_or_land
    hard_stops:
        - issue_has_no_state
        - issue_has_multiple_states
        - dev_brief_type_missing_or_mismatched
        - required_section_missing
        - active_blocked_metadata
        - validation_failed_without_waiver
        - review_artifact_missing
        - review_output_without_human_decision
        - runtime_ownership_modeled_as_issue_fact
```

## Notes

- `backend` is the only backend selector. Do not add a second selector for PR/MR/local review type.
- Do not add `consumers`, `permissions`, or runtime actor role fields. Consumer action authority is defined by the consumer's own orbit, tool, or human process.
- `issue.type` is the source of truth for issue type; the Dev Brief Type line is only a human-readable mirror and uses the canonical type value.
- `sections` maps canonical issue section storage and headings. Required and optional section semantics are defined by the backend-neutral core.
- `review_artifact.required` is a core gate. Its concrete form is defined by the selected backend mapping.
- Concrete commands, API clients, and execution procedures belong to contract consumers, tools, or human process.
- Contract consumers must read the YAML block before reading explanatory docs.
