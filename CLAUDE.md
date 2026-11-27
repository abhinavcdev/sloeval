# CLAUDE.md

Project memory for AI assistants (and a quick brief for humans).

## What this is

**sloeval** (CLI: `sloe`) is an open-source SLO compiler for LLM and agent
systems. Teams write one AISLO YAML spec (plain file or Kubernetes CRD, same
document). `sloe` generates Prometheus recording rules, multi-window
multi-burn-rate alerts (Google SRE Workbook model, like Sloth), Alertmanager
routing labels and Grafana dashboards. It adds AI-specific SLI templates:
latency (TTFT, end-to-end), error_ratio, cost (per request/task), eval_score
(from OpenTelemetry `gen_ai.evaluation.result` events), eval_coverage,
tool_success and judge_agreement.

## Architecture

- Compile-time design: Prometheus/Mimir does the runtime work. No sloe server
  in the request path.
- Pipeline: parse -> validate (JSON Schema + semantic checks) -> IR (normalized
  SLOs, windows, burn rates) -> backends (prometheus, grafana, alertmanager;
  later openslo, datadog).
- An OTel Collector connector (Go, `connector/`, separate module) converts
  `gen_ai.evaluation.result` events into counters and computes cost from token
  usage x a price table.
- All OTel GenAI semantic-convention names live in ONE file:
  `internal/semconv/semconv.go`. The conventions are in Development and will change.

## Constraints (non-negotiable)

- Go for CLI, compiler, connector, operator. Python only for the SDK and judge runner.
- Never put prompts, user IDs or free text in metric labels. The compiler
  enforces a label allowlist (`internal/labels`).
- Every generated rule file passes `promtool check rules`; every alert has a
  `promtool test rules` unit test.
- Table-driven tests and golden files for all generated output. golangci-lint clean. Apache-2.0.
- Small, reviewable commits. **Ask before adding a dependency.**
- Design changes go in `docs/design/` and are approved before code.

## Layout

```
cmd/sloe/                 main.go -> internal/cli
pkg/api/v1alpha1/         public types, Duration, defaults, embedded JSON Schema (schema/)
internal/cli/             subcommands, exit codes
internal/spec/            YAML loading with line/col per field path
internal/validate/        schema pass + semantic pass -> Diagnostics
internal/labels/          label allowlist / denylist / value rules
internal/semconv/         the single OTel GenAI naming file
internal/sli/              which semconv metrics each SLI reads (shared by validate and ir)
internal/ir/               normalized SLOs, SLI ratio terms, merged alerts, burn windows
internal/backend/prometheus/   recording rules, MWMB alerts, generated promtool tests
internal/backend/alertmanager/ routing snippet
internal/backend/grafana/  (Phase 2)
connector/, operator/     separate Go modules (later phases)
sdk/python/               (later)
examples/specs/           example specs
docs/design/              design docs (0001 = layout + API, 0002 = IR + Prometheus; both accepted)
docs/prompts/             per-phase task prompts
```

## Decisions from design 0001 (accepted)

- API group `sloeval.dev` (placeholder until the domain is registered; must be
  final before v0.1.0). Version `v1alpha1`, kind `AISLO`.
- SLI kind is a keyed one-of under `sli:` (exactly one of latency, errorRatio,
  cost, evalScore, evalCoverage, toolSuccess, judgeAgreement).
- `latency.source: client|server`, default `client`.
- Semconv is pinned to open-telemetry/semantic-conventions-genai v0.1.0 (GenAI moved out of
  the main semconv repo). Client TTFT = gen_ai.client.inference.time_to_first_chunk.
- Metric naming is a CLI option `--metric-naming=underscore|utf8` (default underscore), not a spec field.
- Cost per task: task = outermost `invoke_agent` span; `sloe.task.id` attribute overrides.
- judgeAgreement series come from the Python judge runner.
- Selectors are equality-only in v1alpha1. No raw PromQL SLI yet.
- Default burn-rate windows (30d): page 1h/5m@14.4, 6h/30m@6; ticket 1d/2h@3, 3d/6h@1.
- `sloe validate` / `sloe generate` exit codes: 0 ok (warnings allowed), 1 errors, 2 usage/IO.

## Decisions from design 0002 (accepted)

- `target: inference|agent` on latency/errorRatio/evalCoverage; agent latency defaults to
  source server (gen_ai.invoke_agent.duration). `disabled` is tri-state, SLO level wins.
- Error ratio = bad/total rates; latency counts successes only; zero traffic records nothing.
- evalScore minScore: failures = bucket below minScore (exact for discrete scorers).
- One alert name per SLO, page/ticket split by `sloe_severity`; no `for:`; annotations may
  only template `{{ $labels.x }}`.
- Window rollup = avg_over_time of the 5m ratio (Sloth-style, not traffic-weighted).
- Generated promtool tests: fire case per window pair, quiet case, no-traffic case,
  selector-checking value assertion. A golden scan test enforces the label allowlist.

## Commands

```sh
make build test lint golden promtool promtool-check   # golden = UPDATE_GOLDEN=1 go test ./...
go run ./cmd/sloe validate examples/specs/support-agent.yaml
go run ./cmd/sloe generate -o out examples/specs/
```
