# Phase 6: More backends (OpenSLO, Datadog)

Read CLAUDE.md and docs/design/*. Backends consume the IR only.

Step 1: write docs/design/0007-backends.md, then show it to me and wait for approval. It covers:
- OpenSLO v1:
  - Export: AISLO -> SLO/SLI/AlertPolicy/AlertCondition documents. Say which AI SLI types
    map cleanly and which become ratio metrics with annotations.
  - Import: OpenSLO ratio/threshold SLIs -> AISLO where possible, with clear errors otherwise.
- Datadog:
  - Choose the output form (API JSON vs Terraform HCL) and justify it.
  - Metric-based SLOs plus burn-rate monitors.
  - Metric names and tag mapping from OTel GenAI metrics as ingested by Datadog. Pin the
    mapping in internal/semconv, not in the backend.
  - The label allowlist applies to tags too.
- The backend interface, and how `sloe generate --backend=...` selects backends.

Step 2, after approval:
- Implementations with golden files.
- A round-trip test for OpenSLO.
- Schema validation of the OpenSLO output against the official schema if a schema file can
  be vendored. Ask first.

Stop after Phase 6. Summarize what's done, what's left, and open questions.
