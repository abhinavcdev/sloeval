# Phase 1: IR, Prometheus rules and alerts, Alertmanager routing

Read CLAUDE.md and docs/design/0001 first. Phase 0 is merged.

Step 1: write docs/design/0002-ir-and-prometheus.md, then show it to me and wait for approval. It covers:
- IR types (internal/ir): a normalized SLO with id, service, slo name, objective as a
  ratio, window, error budget, resolved selector (spec selector merged with the SLI selector),
  static labels, and a resolved alert policy (merged spec/slo alerting with defaults).
- Burn-rate windows expressed as budget consumed, so they scale to any window:
  page 2%/1h (short 5m) and 5%/6h (short 30m); ticket 10%/1d (short 2h) and 10%/3d (short 6h);
  burnRate = consumed * window / long. For a 30d window this reproduces 14.4 / 6 / 3 / 1.
- The good/total (or bad/total) PromQL for every SLI type and both latency sources, built
  only from names in internal/semconv. Resolve the edge cases explicitly:
  - evalScore minScore vs histogram `le` (<= semantics; how "score >= minScore" maps
    onto a bucket boundary).
  - Cost thresholds must be bucket boundaries.
  - evalCoverage when evaluation is sampled.
  - The error_type exclusion matcher.
- Recording rule naming (`sloe:sli_error:ratio_rate5m` ... `ratio_rate3d`, plus the window
  over 30d computed from 5m) and metadata series (objective, budget, budget remaining),
  all carrying sloe_service, sloe_slo and sloe_id labels.
- MWMB alert expressions (long AND short, per window pair, ORed per severity), alert names,
  and labels (`sloe_severity=page|ticket` plus allowlisted routing labels). Annotations
  are summary, description and runbook_url.
- How alert unit tests are generated: for each alert, synthetic input series for a "fires"
  case at each window pair and a "does not fire" case just under the threshold.
- `--metric-naming=underscore|utf8` handling.

Step 2, after approval, in small commits:
1. internal/ir: build from the validated spec, table-driven tests.
2. internal/backend/prometheus: rule groups, one group per SLO, with deterministic ordering.
3. Generated `promtool test rules` files, one test per alert minimum.
4. internal/backend/alertmanager: route-tree snippet that matches on sloe_severity and team;
   receivers are placeholders named in the output.
5. `sloe generate -o <dir> <file>...` writes prometheus/<name>.rules.yaml,
   prometheus/<name>.test.yaml and alertmanager/routes.yaml. `--metric-naming` applies.
6. Golden files for examples/specs/support-agent.yaml and one minimal spec per SLI type.
   CI runs `promtool check rules` and `promtool test rules` on all of them.

Constraints:
- No PromQL parser dependency; promtool in CI is the check.
- Ask before any new dependency.
- No label outside the allowlist may ever appear in output. Add a test that scans every
  golden file for this.

Done when: the goldens pass, promtool check and test pass in CI, and lint is clean.
Stop after Phase 1. Summarize what's done, what's left, and open questions.
