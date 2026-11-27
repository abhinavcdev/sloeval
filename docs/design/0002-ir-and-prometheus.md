# 0002: IR, Prometheus rules and alerts, Alertmanager routing

Status: **accepted** (2026-11-27). Open questions resolved with the recommendations: O1 bucket approach, O2 `server` default for `target: agent`, O3 one alert name per SLO split by `sloe_severity`, O4 unweighted rollup (Sloth-style).

Builds on 0001 (accepted). There are no new dependencies. PromQL is generated as
strings and checked with `promtool` in CI.

## 1. Pipeline

```
spec files -> validate (0001) -> ir.Build -> backend/prometheus -> rules + promtool tests
                                         -> backend/alertmanager -> route snippet
```

`sloe generate -o <dir> [--metric-naming=underscore|utf8] <file|dir>...`:
- It validates first. If there are any errors, it prints the diagnostics and
  exits with 1 without writing anything. Warnings go to stderr.
- Output files:
  - `<dir>/prometheus/<id>.rules.yaml`
  - `<dir>/prometheus/<id>.test.yaml`
  - `<dir>/alertmanager/routes.yaml`

  `<id>` is `metadata.name`, prefixed with `<namespace>-` when a namespace is set.
- Output is byte-for-byte deterministic: sorted keys, stable ordering, and
  no timestamps or version strings in the files.
- Files are written atomically (temp file + rename). Other files in `<dir>`
  are never deleted.

## 2. API amendments to v1alpha1 (need approval)

These come from the Phase 0 open questions. All are additive, so existing
specs keep validating.

**2a. `target` for operation-level SLIs.** Model-inference metrics
(`gen_ai.client.inference.*`) don't carry `gen_ai.agent.name`, so agent
SLOs need the agent metrics. I propose adding
`target: inference | agent` (default `inference`) to `latency`, `errorRatio`
and `evalCoverage`.

| SLI | target | source | Metric (semconv-genai v0.1.0) |
|---|---|---|---|
| latency ttft | inference | client (default) | `gen_ai.client.inference.time_to_first_chunk` |
| latency ttft | inference | server | `gen_ai.server.time_to_first_token` |
| latency end_to_end | inference | client (default) | `gen_ai.client.inference.duration` |
| latency end_to_end | inference | server | `gen_ai.server.request.duration` |
| latency end_to_end | agent | server (default for agent) | `gen_ai.invoke_agent.duration`, measured by the agent runtime itself (in-process frameworks) |
| latency end_to_end | agent | client | `gen_ai.client.invoke_agent.duration`, measured by the caller of a remote agent |
| errorRatio | inference / agent | n/a | `_count` of the end_to_end metric for that target |

- `ttft` with `target: agent` is an error, because semconv has no agent TTFT metric.
- The default `source` differs by target, because in-process agents report
  `gen_ai.invoke_agent.duration` and not the client metric. The docs say so.

**2b. Re-enabling alerts per SLO.** `alerting.disabled`, `page.disabled` and
`ticket.disabled` become tri-state (`*bool` in Go; YAML is unchanged). The
nearest level that sets a value wins (SLO over spec), so an SLO can set
`disabled: false` to turn alerting back on.

Merge rules for the effective alerting of an SLO:
- **labels and annotations:** merged per key, and the SLO's value wins on a conflict.
- **windows:** if the SLO sets windows, they replace the spec's list entirely.
- **name:** an SLO-level name wins. A spec-level name is a prefix:
  `<Name><SLOCamel>`.

**2c. Selector keys must exist on the metric.** Each `semconv.Metric` gets
the list of attributes it carries (from semconv). A selector key that the
target metric doesn't carry is a **warning**, because the SLI would match
nothing. Static labels (`team`, `tier`, ...) are not checked, since they are
added by the rules, not read from metrics.

**2d. sloe-owned cost metric names.** UCUM has no currency unit, and
annotation units such as `{USD}` add no suffix on OTLP ingestion. So the
instrument names carry the unit: `sloe.gen_ai.request.cost.usd` and
`sloe.gen_ai.task.cost.usd`. The Prometheus names stay
`sloe_gen_ai_{request,task}_cost_usd`.

## 3. IR (`internal/ir`)

```go
type Spec struct {
	ID        string            // "<ns>-<name>" or "<name>"; also the output file name
	Service   string
	SLOs      []SLO
}

type SLO struct {
	ID          string            // sloe_id label: "<spec ID>/<slo name>"
	Service     string            // sloe_service
	Name        string            // sloe_slo
	Objective   float64           // ratio, e.g. 0.995
	Budget      float64           // 1 - Objective
	Window      time.Duration
	Labels      map[string]string // spec.labels + slo.labels (slo wins), allowlisted
	SLI         SLI
	Alerts      []Alert           // 0-2 (page, ticket), after merge and disable
}

type SLI struct {
	Kind     string              // latency, errorRatio, ...
	Matchers []Matcher           // spec selector + SLI selector, sorted by name
	Errors   Query               // bad events (rate) - template over window
	Total    Query               // all events (rate) - template over window
	// Or, for ratio-of-counters kinds whose "good" comes from another metric:
	Good     *Query
}

type Matcher struct{ Name, Op, Value string } // Op: = != =~ !~

type Alert struct {
	Severity    string            // page | ticket
	Name        string            // alertname
	Windows     []BurnWindow      // long/short/burn rate, resolved and scaled
	Labels      map[string]string // routing labels + policy labels + sloe_severity
	Annotations map[string]string
}

type BurnWindow struct {
	Long, Short time.Duration
	BurnRate    float64
}
```

- **Default windows** are expressed as budget consumed (0001 section 3):
  burnRate = consumed × window / long. A 30d window gives 14.4 / 6 / 3 / 1, and a
  28d window gives 13.44 / 5.6 / 2.8 / 0.933.
- **Custom windows** are used as written.
- **Required rate windows** are the union of every alert's long and short windows,
  plus `5m`, which the 30d rollup needs.
- `ir.Build` is pure, and the input must already be validated. Table-driven
  tests cover the merge rules, scaling and window sets.

## 4. PromQL per SLI kind

Notation:
- `S` is the SLO's matchers rendered as `k="v"`, sorted.
- `W` is the rate window.
- `M` is the metric's Prometheus base name (underscore mode).
- Every expression aggregates with `sum(...)` and no `by`, so no source
  labels survive into SLI series.

The **error ratio** is what gets recorded. Bad and total are both rates.

| Kind | Bad | Total |
|---|---|---|
| latency | `M_count{S,error_type=""}` − `M_bucket{S,error_type="",le=T}` | `M_count{S,error_type=""}` |
| errorRatio | `M_count{S,error_type!="",X}` | `M_count{S,X}` |
| cost | `M_count{S}` − `M_bucket{S,le=T}` | `M_count{S}` |
| evalScore (minScore) | `sloe_gen_ai_evaluation_score_bucket{S,gen_ai_evaluation_name=E,le=B⁻}` | `..._score_count{S,gen_ai_evaluation_name=E}` |
| evalScore (passLabels) | `..._results_total{S,E,gen_ai_evaluation_score_label!~P,gen_ai_evaluation_score_label!=""}` | `..._results_total{S,E,gen_ai_evaluation_score_label!=""}` |
| evalCoverage | total − `sloe_gen_ai_evaluation_evaluated_operations_total{S,E}` (clamped at 0) | `M_count{S}` of the target's metric |
| toolSuccess | `gen_ai_execute_tool_duration_seconds_count{S,error_type!="",Tools}` | `..._count{S,Tools}` |
| judgeAgreement | `sloe_judge_agreement_total{S,judge=J,reference=R,outcome="disagree"}` | `sloe_judge_agreement_total{S,judge=J,reference=R}` |

Each kind is rendered as:

```
(sum(rate(BAD[W])) or vector(0)) / sum(rate(TOTAL[W]))
```

Where bad is a difference of two series, it is written as
`sum(rate(A[W])) - sum(rate(B[W]))`. A ratio with zero traffic is NaN, so it
doesn't record and doesn't alert. This is intended and documented.

Decisions in that table:
- **Latency counts successful requests only** (`error_type=""`). Fast failures
  would otherwise look like good latency, and failures are already counted
  by `errorRatio`.
- **`errorRatio.excludeErrorTypes`** becomes `X = error_type!~"a|b"` on both bad
  and total. Excluded outcomes, such as a user cancelling, are neither good nor bad.
- **Regex values** are escaped with `regexp.QuoteMeta`. Label values are already
  restricted to `[A-Za-z0-9_.:/-]`.
- **`le` formatting:** histogram `le` values are written in Prometheus float
  format. Prometheus 3 normalizes integers to `"1.0"` on scrape, and OTLP may
  write `"1"`. So for an integer boundary sloe emits `le=~"1|1\\.0"`, and for
  any other value a plain `le="0.64"`.
- **evalScore minScore and `le` (≤) semantics.** A pass means
  `score >= minScore`, so a fail means `score < minScore`. sloe counts fails as
  `bucket{le=B⁻}`, where `B⁻` is the largest bucket boundary strictly below
  `minScore`. This is exact when no score falls in `(B⁻, minScore)`. That holds
  for discrete scorers (0/1, 1–5, 0.1 steps) with the default buckets.
  Otherwise sloe over-counts passes. Validation already warns when `minScore`
  isn't a boundary. It will also warn when `minScore` is the lowest boundary,
  because then no `B⁻` exists. See open question O1 for an exact alternative.
- **evalCoverage:**
  - The connector counts each operation at most once per evaluator: it dedupes
    by `gen_ai.response.id` with a bounded cache.
  - It copies the operation's allowlisted attributes onto the counter, so `S`
    matches both series.
  - If evaluation is sampled, coverage equals the sampling rate. The objective
    has to be at or below it, and the docs say so.
- **Native histograms** are out of scope for v1alpha1, so the classic
  `_bucket`/`_count` series are required. This is documented.

### Metric naming (`--metric-naming`)

- `underscore` (default) matches Prometheus `UnderscoreEscapingWithSuffixes`,
  using the names in `internal/semconv`.
- `utf8` matches `NoUTF8EscapingWithSuffixes`. Metric and label names keep their
  dots and are quoted:
  `{"gen_ai.client.inference.duration_seconds_count", "gen_ai.request.model"="x"}`.
- The translation lives in `internal/semconv`, as one function per mode.
  Recording rule names (`sloe:*`) and `sloe_*` labels are the same in both modes.
- I'm not yet sure the suffix rules exactly match Prometheus' translator.
  The Phase 3 e2e test (OTLP → Prometheus) will check them. Until it passes,
  `utf8` is marked experimental in `--help`.

## 5. Recording rules

For each SLO there is a rule group `sloe:<sloe_id>:sli` holding one rule per
required window:

```yaml
- record: sloe:sli_error:ratio_rate5m      # ... _rate30m, _rate1h, _rate2h, _rate6h, _rate1d, _rate3d
  expr: <section 4 expression with W=5m>
  labels: {sloe_id: ..., sloe_service: ..., sloe_slo: ..., sloe_window: 30d, <static labels>}
```

A second group, `sloe:<sloe_id>:meta`, has an evaluation interval of 1m:

| Record | Expression |
|---|---|
| `sloe:sli_error:ratio_rate<window>` | `sum_over_time(sloe:sli_error:ratio_rate5m{sloe_id="x"}[<window>]) / count_over_time(sloe:sli_error:ratio_rate5m{sloe_id="x"}[<window>])` |
| `sloe:objective:ratio` | `vector(<objective>)` + labels |
| `sloe:error_budget:ratio` | `vector(<budget>)` + labels |
| `sloe:error_budget_remaining:ratio` | `1 - sloe:sli_error:ratio_rate<window>{…} / <budget>` |
| `sloe:burn_rate:ratio_rate5m` (and 1h) | `sloe:sli_error:ratio_rate5m{…} / <budget>` |
| `sloe:slo_info` | `vector(1)` + labels incl. `sloe_sli_kind`, `sloe_objective` |

- The window rollup averages the 5m ratios. This is the same approach Sloth
  uses, and it isn't weighted by traffic. The docs say so.
- Rule names follow the Prometheus `level:metric:operations` convention, and
  window suffixes use `FormatDuration` (`5m`, `1d`, `30d`).

## 6. Alerts

There is one alert per enabled severity per SLO, in group `sloe:<sloe_id>:alerts`:

```yaml
- alert: SupportAgentTtftErrorBudgetBurn        # default: Camel(service)+Camel(slo)+"ErrorBudgetBurn"
  expr: |
    (sloe:sli_error:ratio_rate1h{sloe_id="x"} > (14.4 * 0.01) and sloe:sli_error:ratio_rate5m{sloe_id="x"} > (14.4 * 0.01))
    or
    (sloe:sli_error:ratio_rate6h{sloe_id="x"} > (6 * 0.01) and sloe:sli_error:ratio_rate30m{sloe_id="x"} > (6 * 0.01))
  labels: {sloe_severity: page, <routing labels>, <policy labels e.g. severity: critical>, <static labels>}
  annotations:
    summary: "support-agent / ttft is burning its error budget too fast (page)"
    description: "Objective 99% over 30d. Long and short burn-rate windows: 1h/5m at 14.4x, 6h/30m at 6x."
    runbook_url: <from alerting.annotations, if set>
```

- Page and ticket are two alerts with the same name, told apart by
  `sloe_severity`. That keeps silences and inhibition simple:
  page inhibits ticket for the same `sloe_id`. The inhibit rule goes in the
  Alertmanager snippet.
- There's no `for:`. The short window already guards against blips and resets
  (SRE Workbook).
- Annotations contain no `$value` templates, so generated promtool tests can
  assert annotations exactly. Burn rates show on the dashboards (Phase 2).
- User annotations are free text and never become labels. The allowlist test
  scans every golden for this.

## 7. Generated `promtool test rules`

For each alert, sloe generates synthetic **source** series. That way the tests
exercise the SLI expressions as well as the alert logic.
- The series are a constant total rate, e.g. `0+600x<N>` at 1m resolution.
- They carry a bad-series rate that gives an exact error ratio `r`, shaped
  for the SLI kind. For histograms, that means `_count` and `_bucket{le=…}` at
  the right `le` format.
- They carry the SLO's matchers as labels, plus one non-matching series to
  check that the selector is applied.

The test cases for each alert are:

| Case | Error ratio r | eval_time | Expectation |
|---|---|---|---|
| fires-\<long\>-\<short\>, one per window pair | just above that pair's threshold and below the next higher one (or 1.0 at most) | the pair's long window + short window | alert firing, with exact labels and annotations |
| quiet | 0.9 × the lowest threshold | the longest long window | no alert (`exp_alerts: []`) |
| no-traffic | no series | 10m | no alert |

- The series interval is 1m for page tests and 5m for ticket tests, which
  keeps the 3d cases fast.
- Each SLO also gets one rule-value test (`promql_expr_test`). It checks that
  `sloe:sli_error:ratio_rate5m` equals `r`, within promtool's float tolerance.
- If `r` can't be represented, the generator fails loudly and doesn't skip
  the case. For example, a 99.99% objective with a 0.9 × threshold ratio
  needs a high total rate, so the total rate scales with 1/budget.
- CI runs `promtool check rules` and `promtool test rules` on every golden
  `*.rules.yaml` / `*.test.yaml` (the Makefile target already exists).

## 8. Alertmanager (`alertmanager/routes.yaml`)

The output is a snippet for the user to merge into their own config, not a
full config:

```yaml
route:
  routes:
    - receiver: support-page          # placeholder: <team or service>-<severity>
      matchers: ['sloe_severity="page"', 'team="support"']
      group_by: [alertname, sloe_id]
    - receiver: support-ticket
      matchers: ['sloe_severity="ticket"', 'team="support"']
      group_by: [alertname, sloe_id]
      repeat_interval: 12h
inhibit_rules:
  - source_matchers: ['sloe_severity="page"']
    target_matchers: ['sloe_severity="ticket"']
    equal: [sloe_id]
receivers:                            # names only; fill in integrations
  - name: support-page
  - name: support-ticket
```

- There is one route per distinct (severity, `team`) pair across all inputs.
  When there's no `team` label, `sloe_service` is used.
- The file holds placeholders only and never contains credentials.
  Alertmanager's `amtool check-config` could check it, but that needs the
  Alertmanager binary. That job is optional, so I'll ask before adding it.

## 9. Tests

1. **ir:** table-driven tests for the merge rules, tri-state disable, window
   scaling (30d/28d/7d) and custom windows.
2. **prometheus:**
   - one golden per minimal spec per SLI kind (both latency sources and both
     targets, both evalScore modes);
   - goldens for `examples/specs/support-agent.yaml`;
   - a `--metric-naming=utf8` golden of the example.
3. **alertmanager:** goldens for the example and a multi-team fixture.
4. **Allowlist scan:** a test parses every golden rule file and checks that
   every label key in `labels:` and every matcher in `expr` is allowlisted, a
   `sloe_*` label, `le`, or one of the semconv-owned label names. Nothing in
   any spec may smuggle in a label.
5. **promtool:** the CI job runs check and test over all goldens.
   `make promtool-check` does the same locally.

## 10. Commits (after approval)

1. `api`: the amendments in section 2 (types, schema, validation and their goldens).
2. `semconv`: per-metric attributes, agent metrics, cost names, naming modes.
3. `ir`: Build and its tests.
4. `backend/prometheus`: recording rules and alerts, with goldens.
5. `backend/prometheus`: test generation, plus CI promtool over goldens.
6. `backend/alertmanager`, with goldens.
7. `cli`: `sloe generate`.
8. The allowlist scan test, and docs/README updates.

## 11. Open questions

- **O1 Exact evalScore thresholds.** The bucket approach in section 4 is exact
  only on bucket boundaries. An exact alternative: the connector also records
  the histogram of `-score`, so `le=-minScore` counts `score >= minScore`
  exactly. It's exact, but obscure, and it doubles the series count. My
  recommendation: ship the bucket approach now and revisit after Phase 3 with
  real evaluator data. Do you agree?
- **O2 Default `source` for `target: agent`.** I propose `server`, meaning the
  agent's own in-process measurement, even though inference defaults to
  `client`. The alternative is to require `source` whenever `target: agent`
  is set. Which do you prefer?
- **O3 Same alert name for page and ticket** (split by `sloe_severity`, like
  Sloth), or separate `...Page` / `...Ticket` names?
- **O4 The window rollup** (unweighted average of 5m ratios, as in Sloth), or
  a traffic-weighted ratio that needs the bad and total rates recorded as
  extra series (two more series per SLO)?
