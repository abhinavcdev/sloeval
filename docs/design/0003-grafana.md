# 0003: Grafana dashboards, docs and the v0.1.0 release

Status: **proposed** (Phase 2, step 1, awaiting approval)

Builds on 0001 and 0002 (both accepted). The dashboards themselves need no
new Go dependencies. The release tooling in section 7 is a set of CI tools,
and I'm asking before adding them.

Phase 1 follow-ups, kept as they are unless you say otherwise:
- one rule file and one test file per spec;
- latency SLIs count successful requests only, so timeouts show up in `errorRatio`;
- no tag or pre-release until you ask for one.

## 1. Target and format

- **Grafana JSON model `schemaVersion: 39`**: the classic dashboard JSON,
  which imports into Grafana 10.4, 11.x and 12.x. Newer versions migrate it
  forward on load. The Grafana 12 "v2" dashboard schema is still
  experimental, so we don't target it. When it stabilizes, it becomes a
  second output format behind a flag.
- **Plain JSON from Go structs** in `internal/backend/grafana`, not the
  Grafana Foundation SDK. The SDK is large and versioned per Grafana release,
  and we need only five panel types: `stat`, `timeseries`, `heatmap`,
  `table` and `text`. A small typed model is easier to golden-test and adds
  no dependency. If we ever need many more panel types, we can revisit the SDK
  then.
- **Deterministic output:**
  - sorted keys, and panel IDs assigned in layout order;
  - no `id`, `version` or timestamps;
  - `uid` is `sloe-` followed by the first 16 hex characters of
    sha256(spec ID). That keeps it stable across runs, under Grafana's
    40-character limit, and free of collisions between specs.
- **Datasource:** a `datasource` variable of type `prometheus`, and every
  panel references `${datasource}`. A sloe dashboard never names a concrete
  datasource.

Output files from `sloe generate`:
- `grafana/<id>.json`: one dashboard per spec;
- `grafana/overview.json`: one dashboard across every spec in the run.

## 2. Per-spec dashboard layout

- **Settings:**
  - Title: `SLOs / <service>`, with ` (<namespace>)` appended when a namespace is set.
  - Tags: `sloe`, `<service>`.
  - Default time range: `now-<longest SLO window>`. Refresh: 1m.
  - Links: a tag link to the other `sloe` dashboards, including the overview.
- **Variables:**
  1. `datasource`: the Prometheus datasource.
  2. `model`: multi-select over `gen_ai_request_model`, including "All"
     (`.*`). It is built with
     `label_values(<a source metric the spec reads>, gen_ai_request_model)`.
     It only filters the type-specific panels (section 3) whose metric carries
     that attribute. Recording-rule series are aggregated, so they don't carry it.
     This is the only variable over a source label. It's on the allowlist and
     has bounded cardinality, and it's the filter people actually reach for.
     More variables can be added later, but only over allowlisted keys.
- **A row per SLO**, titled `<slo name>: <objective>% over <window>` with the
  description as row help. Each row holds:

| Panel | Type | Query (all scoped with `sloe_id="<id>"`) |
|---|---|---|
| SLI over window | stat (%) | `1 - sloe:sli_error:ratio_rate<window>` (red below objective) |
| Objective | stat (%) | `sloe:objective:ratio` |
| Error budget remaining | stat (%) | `sloe:error_budget_remaining:ratio` (thresholds 0 / 25% / 50%) |
| Burn rate (1h) | stat | `sloe:burn_rate:ratio_rate1h` (colored at the page and ticket rates) |
| Burn rate by window | timeseries | `sloe:sli_error:ratio_rate<long> / <budget>` for each alert long window, with dashed threshold lines at each pair's burn rate from the IR (14.4, 6, 3, 1 for the default 30d) |
| Error budget burn-down | timeseries | `sloe:error_budget_remaining:ratio` over the dashboard range |
| SLI details | type-specific, see section 3 | raw source metrics with the SLO's selector |

An SLO with alerting disabled still gets its row. Its burn-rate panel just
has no threshold lines.

## 3. Type-specific panels

Every query below is built from the IR series and `internal/semconv`, not
from hand-written metric names. `S` is the SLO's merged selector, plus
`gen_ai_request_model=~"$model"` when the metric carries that attribute.
`by (...)` clauses use allowlisted labels only.

| SLI kind | Panel(s) |
|---|---|
| latency | timeseries: p50 / p90 / p99 via `histogram_quantile(q, sum by (le) (rate(M_bucket{S}[$__rate_interval])))`, plus a threshold line at the SLO threshold |
| errorRatio | timeseries (stacked): `sum by (error_type) (rate(M_count{S, error_type!=""}[$__rate_interval]))`, and request rate |
| cost | timeseries: mean cost `sum(rate(M_sum{S}[…])) / sum(rate(M_count{S}[…]))` and p95 via `histogram_quantile`, plus a threshold line |
| evalScore (minScore) | heatmap of `sum by (le) (increase(sloe_gen_ai_evaluation_score_bucket{S,E}[$__rate_interval]))`, plus a pass-rate line |
| evalScore (passLabels) | timeseries (stacked): `sum by (gen_ai_evaluation_score_label) (rate(…_results_total{S,E}[…]))` |
| evalCoverage | timeseries: covered ÷ total operations (the IR's coverage terms), and the evaluation rate |
| toolSuccess | timeseries: error ratio `by (gen_ai_tool_name)`, plus a call-rate table by tool |
| judgeAgreement | timeseries: agreement ratio, and comparisons per hour by `outcome` |

The `_sum` series needs a small addition to `semconv.Metric`: a `Sum(n)`
method, to go with `Bucket` and `Count`.

## 4. Overview dashboard

The title is `SLOs / Overview`, with tags `sloe` and `overview`. The
datasource variable is the only variable.
- **Table:** one row per SLO from every spec in the run, with columns
  service, SLO, SLI kind, objective, window, budget remaining, burn rate (1h),
  and a link to the spec's dashboard. The queries are instant queries over
  `sloe:slo_info`, `sloe:error_budget_remaining:ratio` and
  `sloe:burn_rate:ratio_rate1h`. Each is scoped with
  `sloe_id=~"<the IDs in this run, escaped>"` and merged on `sloe_id` using
  Grafana's "join by field" transformation.
- **Stat:** firing SLO alerts by severity:
  `count by (sloe_severity) (ALERTS{alertstate="firing", sloe_id=~"…"})`.
  `ALERTS` is the Prometheus built-in series for alerts.
- **Text:** one line that says how the dashboard was generated (no version
  string, so the goldens stay stable).

## 5. Tests

1. **Goldens:** the JSON for the example spec, for one spec per SLI variant
   (reusing `internal/backend/prometheus/testdata/specs`), and for the overview.
2. **Query reference test:** this parses every golden dashboard and extracts
   each PromQL expression. It then checks every metric name and every label
   in the expressions.
   - **Metric names** must be one of: a recording rule that the Phase 1
     backend emits for that spec, a series of a `semconv` metric, or `ALERTS`.
   - **Labels** must be: allowlisted, `sloe_*`, `le`, or the judge runner's
     labels (`judge`, `reference`, `outcome`).
   - The variables `$model`, `$__rate_interval` and `${datasource}` are the
     only templating allowed.
3. **Structural test:**
   - panel IDs are unique;
   - `gridPos` rectangles don't overlap, and fit the 24-column grid;
   - every panel's datasource is `${datasource}`;
   - the `uid` is at most 40 characters.
4. **Optional, I'll ask first:**
   - **dashboard-linter in CI:** `grafana/dashboard-linter`, a Go tool run with
     a pinned `go run …@<version>`. It isn't a module dependency.
   - **An e2e job:** docker compose with Prometheus 3.5 and Grafana 12. It
     loads the generated rules and dashboards, replays the promtool test
     series via remote write, and checks the panel queries through Grafana's
     API with no errors. It is valuable, but it's the heaviest CI job and the
     flakiest, so it would be a separate workflow on a nightly schedule.

## 6. Docs

- **README quickstart:** write a spec, validate it, generate, load the rules
  into Prometheus or the Mimir ruler (`mimirtool rules load`), merge the
  Alertmanager snippet, and import the dashboards (UI, provisioning, or the
  Grafana API).
- **`docs/sli-reference.md`:** the exact PromQL for each SLI kind. It is
  **generated**: a test renders it from the testdata specs and checks it in as
  a golden, so the doc can never drift from the code.
- **`docs/labels.md`:**
  - the allowlist with the reason each label's cardinality is bounded;
  - the denylist patterns and the reasons for them;
  - the labels sloe sets itself (`sloe_*`);
  - how to promote resource attributes;
  - how to propose a new label.

  The allowlist table is also generated from `internal/labels` by a golden test.

## 7. Release tooling (needs your approval)

| Tool | Use | License | How it's used |
|---|---|---|---|
| GoReleaser (OSS) | multi-arch builds (linux/darwin/windows × amd64/arm64), archives, `checksums.txt`, GitHub release | MIT | `goreleaser/goreleaser-action`, pinned version, in a `release.yml` workflow on `v*` tags |
| Syft | an SBOM per archive (SPDX JSON), through GoReleaser's `sboms` | Apache-2.0 | `anchore/sbom-action/download-syft`, pinned |
| Docker Buildx | multi-arch image `ghcr.io/abhinavcdev/sloe:<tag>` on `gcr.io/distroless/static` (nonroot) | n/a | GoReleaser `dockers` + `docker_manifests` |

- The version is set through `-ldflags -X …/internal/cli.Version={{.Version}}`.
  The binary is built with `CGO_ENABLED=0` and `-trimpath`, for reproducible builds.
- The CI workflow also runs `goreleaser check` and a snapshot build
  (`goreleaser release --snapshot --skip=publish`) on every push. Release
  breakage then shows up before a tag is pushed.
- The release workflow needs `contents: write` and `packages: write`, and only
  for tag pushes. No secrets beyond `GITHUB_TOKEN`.
- Signing (cosign keyless) and provenance attestations are Phase 7.
- **Tagging:** I won't push `v0.1.0`. You tag it when you're ready. The API
  group (`sloeval.dev`) has to be final before then.

## 8. Commits (after approval)

1. `semconv`: `Sum(n)`. `grafana`: the dashboard model and the per-spec
   dashboard, with goldens.
2. `grafana`: the type-specific panels, with goldens.
3. `grafana`: the overview dashboard. `cli`: `generate` writes `grafana/`.
4. Tests: query references and structure.
5. Docs: README quickstart, the generated `sli-reference.md` and
   `labels.md` tables.
6. Release: `.goreleaser.yaml`, `Dockerfile`, `release.yml`, and the snapshot
   job in CI. This one only if you approve section 7.

## 9. Open questions

- **G1 Release tooling:** approve GoReleaser, Syft and the Buildx image
  (section 7)?
- **G2 Optional CI checks:** add dashboard-linter? Add the nightly
  compose-based e2e?
- **G3 Image registry:** is `ghcr.io/abhinavcdev/sloe` the right place for the
  image, or do you want a different registry or org?
- **G4 `$model` variable:** keep this one source-label variable, or ship
  with the datasource variable only?
