# Phase 4: Python SDK and judge runner

Read CLAUDE.md and docs/design/*. Python lives only in sdk/python. Go stays
for everything else.

Step 1: write docs/design/0005-python-sdk.md, then show it to me and wait for approval. It covers:
- Package name and layout. Python version floor. Tooling proposal (e.g. uv, ruff, mypy,
  pytest); ask before each dependency. The runtime deps should be the OTel API/SDK only.
- SDK API:
  - `record_evaluation(name, score_value=None, score_label=None, explanation=None, response_id=..., ...)`
    emits gen_ai.evaluation.result with attribute names from a Python copy of the semconv
    mapping. That copy is generated from the Go file and checked in CI so they can't drift.
  - Helpers to mark task roots (`sloe.task.id`) and client TTFT.
  - Explanation text goes in the event body or attributes, never in metric labels. Document this.
- Judge runner:
  - CLI that samples traces or a dataset, calls an LLM judge (provider pluggable, no provider
    SDK required by default), and emits evaluation events.
  - Calibration mode: compares judge labels to reference labels (e.g. human review) and
    emits `sloe_judge_agreement_total{judge,reference,outcome}`.
  - Rate limits, retries, and a cost cap per run.
- Testing with an in-memory OTel exporter. No network in unit tests.

Step 2, after approval:
- Implementation with tests.
- CI job: lint, type-check, test, and the semconv drift check.
- An example that runs the judge on a small fixture and shows the series the connector
  and the Phase 1 rules consume.
- Packaging (pyproject) ready for PyPI. Publish only when I say so.

Stop after Phase 4. Summarize what's done, what's left, and open questions.
