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

Blocked issues use `state:blocked`. Do not add `blocked:*` labels. If an automation publishes a blocked issue from a template that defaults to `state:needs-triage`, it must replace that default state label rather than add a second `state:*` label.
