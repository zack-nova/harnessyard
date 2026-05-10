# Dedicated demo fixture repositories

Public end-to-end demos should use dedicated lightweight Harness Yard fixture repositories instead of reusing existing production-oriented Orbit Packages. We chose separate demo fixtures because the demos need to teach the Harness Yard object model first; existing issue-tracker, design-memory, and development-discipline packages are real and useful, but their workflow domains add too much unrelated context for first-pass product demos.

## Consequences

The demo fixtures become maintained public assets, not throwaway examples. They should keep Orbit Workflow boundaries narrow and obvious, compose into a small Harness Package, and remain suitable for future quickstart smoke tests.
