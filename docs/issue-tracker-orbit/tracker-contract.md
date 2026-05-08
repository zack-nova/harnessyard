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
    url_format: "https://github.com/zack-nova/harnessyard/issues/<number>"
    state:
        required: true
        cardinality: exactly_one
        representation: label
        prefix: "state:"
        values:
            needs-triage: "state:needs-triage"
            needs-info: "state:needs-info"
            needs-split: "state:needs-split"
            blocked: "state:blocked"
            ready-for-dev: "state:ready-for-dev"
            in-progress: "state:in-progress"
            in-review: "state:in-review"
            human-review: "state:human-review"
            to-rework: "state:to-rework"
            to-merge: "state:to-merge"
            merged: "state:merged"
            cancelled: "state:cancelled"
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
            out-of-scope: "type:out-of-scope"
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
        delivery_mode:
            required: false
            representation: label
            prefix: "delivery-mode:"
            values:
                afk: "delivery-mode:afk"
                hitl: "delivery-mode:hitl"
        resolution:
            required: false
            representation: label
            prefix: "resolution:"
            values:
                wontfix: "resolution:wontfix"
                duplicate: "resolution:duplicate"

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
    out-of-scope-catalog:
        storage: issue_body
        heading: '## Out-of-Scope Catalog'

review_artifact:
    required: true
    storage: github_pull_request
    link_rule: closing_reference

templates:
    issue_templates: ".github/ISSUE_TEMPLATE/"
    pull_request_template: ".github/pull_request_template.md"

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
        - split_state_advancement_without_resolution
        - duplicate_resolution_without_superseding_issue
        - blocked_state_advancement_without_unblock
        - validation_failed_without_waiver
        - review_artifact_missing
        - hitl_review_output_without_human_decision
        - invalid_human_review_decision
        - invalid_delivery_mode
        - hitl_delivery_mode_without_rationale
        - runtime_ownership_modeled_as_issue_fact
```

## Notes

- `backend` is the only backend selector. Do not add a second selector for PR/MR/local review type.
- Do not add `consumers`, `permissions`, or runtime actor role fields. Consumer action authority is defined by the consumer's own orbit, tool, or human process.
- Do not add `backend_mapping`; put machine mapping facts directly under `issue`, `sections`, `review_artifact`, and `templates`.
- `issue.type` is the source of truth for issue type; the Dev Brief Type line is only a human-readable mirror and uses the canonical type value.
- `issue.metadata.delivery_mode` is optional. When present, it records whether a delivery slice is `afk` or `hitl`; it is not a state role or runtime ownership field.
- `blocked` and `needs-split` are canonical state roles. Record blockers, split reasons, child issue references, and intended resume states in issue text before state advancement.
- `cancelled` is the terminal non-delivery state. Use `resolution:wontfix` for out-of-scope ordinary feature requests and `resolution:duplicate` for superseded issues when the tracker contract supports resolution metadata.
- `sections` maps canonical issue section storage and headings. Required and optional section semantics are defined by the backend-neutral core.
- Concrete commands, API clients, and execution procedures belong to contract consumers, tools, or human process.
- Contract consumers must read the YAML block before reading explanatory docs.
