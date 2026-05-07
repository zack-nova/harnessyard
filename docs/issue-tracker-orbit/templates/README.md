# Templates

This directory stores reference templates for backend initialization and issue section use.

## Backend Templates

`backends/` stores initialization templates by backend. After Bootstrap selects a backend, read only that backend's subdirectory.

```text
backends/github/
backends/local-markdown/
```

Files under `backends/github/` are used to safely install or merge into the real `.github/` directory when the GitHub backend is selected.

Files under `backends/local-markdown/` are used by the local markdown backend to create issue files and local review artifacts.

## Issue Section Templates

Files under `issue-sections/` are used by responsible **Contract Consumers** to create or update canonical issue sections in the issue tracker.

Optional issue sections, such as `debt-notes`, are created only when there is real content to record.
