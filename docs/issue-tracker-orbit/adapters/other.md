# Other Backend Adapter

Use this adapter only when the repository uses an issue tracker not covered by GitHub, GitLab, or local markdown.

Bootstrap must write the selected backend mapping into `tracker-contract.md`. Freeform prose is not enough; contract consumers need a machine-readable contract block.

## Required Mapping

The generated tracker contract must specify:

- how to identify an issue,
- where state is stored,
- where issue type is stored,
- how optional metadata is stored,
- where each canonical issue section is stored,
- how the review artifact is identified,
- where review and land evidence is recorded,
- how validation evidence is recorded,
- which workflow changes require dry-run,
- which states or conflicts are hard stops.

If any required mapping is unclear, stop bootstrap and ask a human maintainer.
