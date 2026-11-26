# Phase 2: Grafana dashboards and v0.1.0 release

Read CLAUDE.md and docs/design/0001–0002. Phase 1 is merged.

Step 1: write docs/design/0003-grafana.md, then show it to me and wait for approval. It covers:
- The dashboard JSON model version targeted (pin it).
- Plain JSON generated from Go structs vs a Grafana Foundation SDK. Ask before adding the SDK.
- Layout. Row per SLO:
  - stat panels: SLI now, objective, error budget remaining;
  - burn-rate time series with page/ticket thresholds;
  - budget-burn-down over the window.
- Type-specific panels: latency percentiles, cost per request/task, eval score
  distribution, eval coverage, tool success by tool, judge agreement.
- Templating variables limited to allowlisted labels. Datasource as a variable.
- An overview dashboard across all specs in one generate run.

Step 2, after approval:
1. internal/backend/grafana with golden JSON per example spec.
2. A test that every panel query only references recording rules or semconv metrics
   that Phase 1 produces, and only allowlisted labels.
3. `sloe generate` writes grafana/<name>.json (plus overview.json).
4. Docs:
   - README quickstart: write spec -> validate -> generate -> load rules into
     Prometheus/Mimir -> import dashboard.
   - docs/sli-reference.md with the exact PromQL per SLI type.
   - docs/labels.md.
5. Release: propose a tool (e.g. goreleaser) and ask before adding it. Build multi-arch binaries,
   checksums, SBOM and a container image. Tag v0.1.0 only when I say so.

Optional, ask first: an e2e job that runs Prometheus + Grafana in docker-compose,
loads the generated files, and checks that the dashboards render with no query errors.

Stop after Phase 2. Summarize what's done, what's left, and open questions.
