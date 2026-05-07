# Root guidance workflow markers

Root guidance blocks use owner-specific marker namespaces, `orbit:` and `harness:`, with a single `workflow` attribute for the block's public Workflow ID. We chose this shape so public guidance can describe orbit packages as atomic workflows and harness-owned guidance as harness blocks, while internal OrbitSpec, manifest, storage, and branch contracts remain orbit- and harness-based. Because Harness Yard has not released this marker contract yet, pre-release `orbit_id` root guidance markers do not need compatibility support.
