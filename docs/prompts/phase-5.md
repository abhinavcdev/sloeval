# Phase 5: Kubernetes operator

Read CLAUDE.md and docs/design/*. This lives in operator/ as a separate Go module.

Step 1: write docs/design/0006-operator.md, then show it to me and wait for approval. It covers:
- Deps: controller-runtime, apimachinery, prometheus-operator API types. List versions and
  ask before adding.
- CRD generation from pkg/api/v1alpha1/schema (structural schema conversion: $ref inlining,
  oneOf handling). Output to deploy/crd/. CI checks it is up to date.
- Reconcile AISLO -> PrometheusRule (rules from the Phase 1 backend; same code, no fork)
  plus a Grafana dashboard ConfigMap (sidecar label configurable) plus alert routing labels.
  Owner references and garbage collection.
- Status: conditions (Valid, Generated, Ready), validation diagnostics surfaced in status
  and events, observedGeneration.
- A validating admission webhook reusing internal/validate, or CEL rules only. State the
  trade-off.
- Multi-tenancy: namespace-scoped, with RBAC least privilege.

Step 2, after approval:
- Implementation.
- envtest-based tests.
- A kind-based e2e test: apply the example AISLO and assert the PrometheusRule content matches
  `sloe generate` golden output.
- Helm chart or kustomize. Propose which, then ask.
- Container image.

Stop after Phase 5. Summarize what's done, what's left, and open questions.
