# Phase 3: OTel Collector connector

Read CLAUDE.md and docs/design/*. This lives in connector/ as a separate Go module.

Step 1: write docs/design/0004-connector.md, then show it to me and wait for approval. It covers:
- Component type: connector (traces+logs -> metrics) and its ID (e.g. `sloe`).
  Collector version pinned, built with ocb (builder manifest in connector/).
  List the collector deps and ask before adding them.
- Inputs:
  - gen_ai.evaluation.result events (as log records with event.name and as span events;
    support both).
  - gen_ai client/agent/tool spans for token usage, errors, tool calls and task roots.
- Outputs: the exact metric names/types/units/labels Phase 1 queries expect, taken from
  internal/semconv. Move the shared mapping into a small importable package if needed; no
  copy-paste of names.
  - evaluation results counter + score histogram (by gen_ai_evaluation_name, score label).
  - cost histograms per request and per task (per="request"|"task").
  - client TTFT if the instrumentation doesn't emit it.
  - tool call outcome counter.
- Price table: user-supplied YAML keyed by provider + model (input, output and cached input
  per 1M tokens, effective dates). Unknown model: count it in a self-metric and don't guess.
  Ship an example table only, clearly marked as not maintained prices.
- Task cost (design 0001 Q3): per-trace buffering bounded by time and count, the
  `sloe.task.id` override, and self-metrics for dropped or late spans.
- Label enforcement: attributes are mapped to labels only through the allowlist.
  Prompts, completions, user IDs and free text are never read into labels.
- Configurable histogram buckets, defaulting to the semconv advisory buckets, kept in
  sync with the compiler's bucket-boundary validation.

Step 2, after approval:
- Implementation with pdata-based table-driven tests.
- Golden metric output (pmetrictest or equivalent).
- An e2e test: OTLP in -> connector -> Prometheus exposition. Assert that Phase 1's generated
  rules evaluate correctly against it (promtool test with the recorded series is fine).
- README for the connector with a collector config example.

Stop after Phase 3. Summarize what's done, what's left, and open questions.
