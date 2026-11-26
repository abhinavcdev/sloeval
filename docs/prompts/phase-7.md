# Phase 7: Hardening toward v1beta1

Read CLAUDE.md and docs/design/*.

Step 1: write docs/design/0008-v1beta1.md, then show it to me and wait for approval. It covers:
- API review from user feedback on v1alpha1: what changes, and the conversion path
  (`sloe migrate`, plus a CRD conversion webhook for the operator).
- Revisit the deferred items: a `custom` SLI with declared metrics/labels (Q6) and
  `matchExpressions` (Q5).
- Semconv upgrade process: how a GenAI semconv release is adopted (one file changes,
  goldens regenerate, the changelog notes metric renames), plus a compatibility mode that emits
  both old and new names for one release.
- Stability policy: what is covered by semver (CLI flags, output metric/rule names, the API).

Step 2, after approval:
- Fuzz tests for the YAML loader and validator (go test -fuzz) in CI on a short budget.
- A benchmark for compiling 1,000 SLOs.
- govulncheck in CI.
- Signed releases (cosign) and SBOM.
- A docs site proposal (ask before adding a generator).
- Security review of the label pipeline end to end: SDK -> connector -> rules -> dashboards.

Stop after Phase 7. Summarize what's done, what's left, and open questions.
