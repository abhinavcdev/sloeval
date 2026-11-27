# sloeval

`sloe` is an SLO compiler for LLM and agent systems. You write one AISLO
spec, and `sloe` generates Prometheus recording rules, multi-window
multi-burn-rate alerts (the Google SRE Workbook model) with `promtool` unit
tests, and Alertmanager routing. Prometheus or Mimir does the runtime work;
there is no sloe server in the request path.

Besides latency (time to first token, end to end) and error ratio, it has
SLI templates for AI systems: cost per request or task, evaluation scores
from OpenTelemetry `gen_ai.evaluation.result` events, evaluation coverage,
tool success and LLM-judge agreement with human labels. Metric names follow
the [OpenTelemetry GenAI semantic conventions](https://github.com/open-telemetry/semantic-conventions-genai)
(pinned to v0.1.0).

> Status: early development (`v1alpha1`). Grafana dashboards, the OTel
> Collector connector, the Python SDK and the Kubernetes operator are planned.

## Quick start

```sh
go install github.com/abhinavcdev/sloeval/cmd/sloe@latest

sloe validate examples/specs/support-agent.yaml
sloe generate -o out examples/specs/support-agent.yaml
promtool check rules out/prometheus/*.rules.yaml
promtool test rules out/prometheus/*.test.yaml
```

Then load `out/prometheus/*.rules.yaml` into Prometheus or the Mimir ruler,
and merge `out/alertmanager/routes.yaml` into your Alertmanager config.

A minimal spec:

```yaml
apiVersion: sloeval.dev/v1alpha1
kind: AISLO
metadata:
  name: chat
spec:
  service: chat
  labels: {team: ml}
  slos:
    - name: ttft
      objective: 99
      sli:
        latency: {stage: ttft, threshold: 0.64s}
    - name: helpfulness
      objective: 90
      sli:
        evalScore: {evaluator: helpfulness, minScore: 0.7}
```

The same document works as a plain file or as a Kubernetes custom resource.
See [`examples/specs/support-agent.yaml`](examples/specs/support-agent.yaml)
for every SLI kind, and [`docs/design/`](docs/design) for the design.

## Requirements on your telemetry

- Classic histograms (`_bucket`/`_count`). Thresholds should sit on bucket
  boundaries; `sloe validate` warns when they don't.
- If your selectors use `service_name` or `deployment_environment_name`,
  promote those resource attributes to labels in your OTLP ingestion config
  (Prometheus `otlp.promote_resource_attributes`).
- Labels are restricted to an allowlist of bounded-cardinality keys. Prompts,
  user IDs and other free text are rejected.

## Development

```sh
make build test lint      # golangci-lint v2
make golden               # regenerate golden files (UPDATE_GOLDEN=1)
make promtool promtool-check
```

See [CONTRIBUTING.md](CONTRIBUTING.md). Licensed under Apache-2.0.
