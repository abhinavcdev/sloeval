# 0001: Repo layout and AISLO v1alpha1

Status: **accepted** (2026-11-26). Open questions answered in section 8.

## 1. Example spec

The plain file and the Kubernetes CRD use the same document. A file that
`kubectl apply` accepts also works with `sloe validate`.

```yaml
apiVersion: sloeval.dev/v1alpha1      # API group: open question Q1
kind: AISLO
metadata:
  name: support-agent
  namespace: support                  # optional; used only as a CRD
spec:
  service: support-agent
  description: Tier-1 customer support agent
  labels:                             # static labels on every generated series and alert
    team: support                     # keys must be on the allowlist (section 5)
    tier: "1"
  selector:                           # matchers added to every SLI query
    matchLabels:
      gen_ai_agent_name: support-agent
      deployment_environment: prod
  alerting:                           # defaults for every SLO; each SLO can override
    annotations:
      runbook_url: https://runbooks.example.com/support-agent
    page:
      labels: {severity: critical}
    ticket:
      labels: {severity: warning}
  slos:
    - name: ttft
      objective: 99.0                 # percent
      window: 30d
      sli:
        latency:
          stage: ttft                 # ttft | end_to_end
          threshold: 2s
    - name: availability
      objective: 99.5
      sli:
        errorRatio:
          excludeErrorTypes: [cancelled]
    - name: cost-per-task
      objective: 95
      sli:
        cost:
          per: task                   # request | task
          thresholdUSD: 0.25
    - name: helpfulness
      objective: 90
      sli:
        evalScore:
          evaluator: helpfulness      # gen_ai.evaluation.name
          minScore: 0.7               # or passLabels: [pass]
    - name: eval-coverage
      objective: 98
      sli:
        evalCoverage:
          evaluator: helpfulness
    - name: tools
      objective: 99
      sli:
        toolSuccess:
          tools: [search_kb, create_ticket]   # optional; defaults to all tools
      alerting:
        page: {disabled: true}        # tickets only for this SLO
    - name: judge-vs-human
      objective: 85
      window: 28d
      sli:
        judgeAgreement:
          judge: helpfulness
          reference: human_review
```

## 2. Repo layout

```
cmd/sloe/                      main.go only; calls internal/cli
internal/cli/                  subcommands (validate now; generate later), exit codes
pkg/api/v1alpha1/              public Go types, defaults, Duration type
pkg/api/v1alpha1/schema/       aislo.schema.json (go:embed), used for validation, the CRD, and editors
internal/spec/                 YAML loading: multi-doc, keeps line/col for each field path
internal/validate/             JSON Schema pass + semantic pass -> []Diagnostic{Path, Line, Col, Msg, Hint, Severity}
internal/labels/               label allowlist and value rules (one source of truth)
internal/semconv/              THE single OTel GenAI semconv mapping file (names, attrs, Prom names, buckets)
internal/ir/                   (Phase 1) normalized SLOs, windows, burn rates
internal/backend/prometheus/   (Phase 1) recording rules + MWMB alerts
internal/backend/alertmanager/ (Phase 1) routing labels / route snippets
internal/backend/grafana/      (Phase 1) dashboards
connector/                     (Phase 2) OTel Collector connector, a separate Go module so CLI deps stay small
operator/                      (later) separate Go module (controller-runtime)
sdk/python/                    (later) Python SDK + judge runner
deploy/crd/                    CRD manifest generated from the JSON Schema
examples/specs/                support-agent.yaml, ...
testdata/                      golden inputs/outputs (per package testdata/ dirs too)
docs/design/                   this document and later ADRs
.github/workflows/             lint, test, promtool
```

Notes:
- Only `pkg/api` is public. Everything else is `internal/` until a backend
  needs a stable API.
- `connector/` and `operator/` are separate modules (own `go.mod`) so the
  CLI's `go.mod` does not pull in the Collector or controller-runtime.

## 3. Go types (`pkg/api/v1alpha1`)

```go
package v1alpha1

const (
	Group      = "sloeval.dev" // Q1
	Version    = "v1alpha1"
	APIVersion = Group + "/" + Version
	Kind       = "AISLO"
)

// AISLO is the top-level document. Its JSON shape matches a Kubernetes object,
// so the operator can wrap it later without changing the format.
type AISLO struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       AISLOSpec  `json:"spec"`
}

// ObjectMeta is a small subset of metav1.ObjectMeta, so we don't depend on apimachinery yet.
type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type AISLOSpec struct {
	Service     string            `json:"service"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`   // allowlisted keys, safe values
	Selector    *Selector         `json:"selector,omitempty"` // merged into every SLI
	Alerting    *Alerting         `json:"alerting,omitempty"` // defaults for all SLOs
	SLOs        []SLO             `json:"slos"`
}

type SLO struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Objective   float64           `json:"objective"`        // percent, 0 < x < 100
	Window      Duration          `json:"window,omitempty"` // default 30d
	Labels      map[string]string `json:"labels,omitempty"`
	SLI         SLI               `json:"sli"`
	Alerting    *Alerting         `json:"alerting,omitempty"` // merged over spec.alerting
}

// Selector holds equality matchers only in v1alpha1 (regex: Q5).
// Keys are Prometheus label names and must be on the allowlist.
type Selector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// SLI must set exactly one field. A keyed one-of (rather than a `type:`
// discriminator) gives each kind its own schema and clearer errors.
type SLI struct {
	Latency        *LatencySLI        `json:"latency,omitempty"`
	ErrorRatio     *ErrorRatioSLI     `json:"errorRatio,omitempty"`
	Cost           *CostSLI           `json:"cost,omitempty"`
	EvalScore      *EvalScoreSLI      `json:"evalScore,omitempty"`
	EvalCoverage   *EvalCoverageSLI   `json:"evalCoverage,omitempty"`
	ToolSuccess    *ToolSuccessSLI    `json:"toolSuccess,omitempty"`
	JudgeAgreement *JudgeAgreementSLI `json:"judgeAgreement,omitempty"`
}

type LatencyStage string

const (
	StageTTFT      LatencyStage = "ttft"
	StageEndToEnd  LatencyStage = "end_to_end"
)

// good = requests with duration <= threshold. The threshold should be a
// histogram bucket boundary; otherwise validate warns.
type LatencySLI struct {
	Stage     LatencyStage `json:"stage"`
	Source    string       `json:"source,omitempty"` // client (default) | server; see Q7
	Threshold Duration     `json:"threshold"`
	Selector  *Selector    `json:"selector,omitempty"`
}

// good = operations with no error.type, except listed types.
type ErrorRatioSLI struct {
	ExcludeErrorTypes []string  `json:"excludeErrorTypes,omitempty"`
	Selector          *Selector `json:"selector,omitempty"`
}

type CostUnit string

const (
	CostPerRequest CostUnit = "request"
	CostPerTask    CostUnit = "task"
)

// good = requests/tasks whose computed cost (connector: tokens x price table)
// is <= ThresholdUSD. Only USD in v1alpha1.
type CostSLI struct {
	Per          CostUnit  `json:"per"`
	ThresholdUSD float64   `json:"thresholdUSD"`
	Selector     *Selector `json:"selector,omitempty"`
}

// good = gen_ai.evaluation.result events for Evaluator that pass.
// Set exactly one of MinScore (score.value >= MinScore) or PassLabels (score.label in set).
type EvalScoreSLI struct {
	Evaluator  string    `json:"evaluator"`
	MinScore   *float64  `json:"minScore,omitempty"`
	PassLabels []string  `json:"passLabels,omitempty"`
	Selector   *Selector `json:"selector,omitempty"`
}

// good = operations that have at least one Evaluator result; total = operations.
type EvalCoverageSLI struct {
	Evaluator string    `json:"evaluator"`
	Selector  *Selector `json:"selector,omitempty"`
}

// good = execute_tool operations with no error.type. Empty Tools means all tools.
type ToolSuccessSLI struct {
	Tools    []string  `json:"tools,omitempty"`
	Selector *Selector `json:"selector,omitempty"`
}

// good = items where the Judge evaluator's label matches the Reference
// evaluator's label (for example, human review). Emitted by the judge runner (Q4).
type JudgeAgreementSLI struct {
	Judge     string    `json:"judge"`
	Reference string    `json:"reference"`
	Selector  *Selector `json:"selector,omitempty"`
}

type Alerting struct {
	Disabled    bool              `json:"disabled,omitempty"`
	Name        string            `json:"name,omitempty"`        // alert name prefix; default derived
	Labels      map[string]string `json:"labels,omitempty"`      // routing labels; allowlisted
	Annotations map[string]string `json:"annotations,omitempty"` // free text OK (not series labels)
	Page        *AlertPolicy      `json:"page,omitempty"`
	Ticket      *AlertPolicy      `json:"ticket,omitempty"`
}

type AlertPolicy struct {
	Disabled bool              `json:"disabled,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	// Windows overrides the SRE Workbook defaults. If empty, the defaults are:
	//   page:   1h/5m @ 14.4, 6h/30m @ 6
	//   ticket: 1d/2h @ 3,    3d/6h  @ 1
	// (for a 30d window; scaled for other window lengths in the IR).
	Windows []BurnRateWindow `json:"windows,omitempty"`
}

type BurnRateWindow struct {
	Long     Duration `json:"long"`
	Short    Duration `json:"short"`
	BurnRate float64  `json:"burnRate"`
}

// Duration is a Prometheus-style duration ("5m", "1h", "30d"). It is kept as a
// string in YAML/JSON and parsed by our own small parser (no prometheus/common dependency).
type Duration string
```

Defaults (applied after validation, in `defaults.go`): `window: 30d`,
alerting page+ticket enabled with Workbook windows, `alert name =
<Service><SLO>` in CamelCase.

## 4. JSON Schema (`pkg/api/v1alpha1/schema/aislo.schema.json`)

Draft 2020-12. The same file feeds the CRD's `openAPIV3Schema`. When I
generate the CRD, I'll drop or convert the keywords that k8s structural
schemas don't support (`$defs`/`$ref`, `oneOf` with `required`).

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://sloeval.dev/schemas/aislo/v1alpha1.json",
  "title": "AISLO",
  "type": "object",
  "additionalProperties": false,
  "required": ["apiVersion", "kind", "metadata", "spec"],
  "properties": {
    "apiVersion": { "const": "sloeval.dev/v1alpha1" },
    "kind":       { "const": "AISLO" },
    "metadata": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name"],
      "properties": {
        "name":        { "$ref": "#/$defs/dnsName" },
        "namespace":   { "$ref": "#/$defs/dnsName" },
        "labels":      { "type": "object", "additionalProperties": { "type": "string" } },
        "annotations": { "type": "object", "additionalProperties": { "type": "string" } }
      }
    },
    "spec": {
      "type": "object",
      "additionalProperties": false,
      "required": ["service", "slos"],
      "properties": {
        "service":     { "$ref": "#/$defs/labelValue" },
        "description": { "type": "string", "maxLength": 1024 },
        "labels":      { "$ref": "#/$defs/labelMap" },
        "selector":    { "$ref": "#/$defs/selector" },
        "alerting":    { "$ref": "#/$defs/alerting" },
        "slos": { "type": "array", "minItems": 1, "items": { "$ref": "#/$defs/slo" } }
      }
    }
  },
  "$defs": {
    "dnsName":    { "type": "string", "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$", "maxLength": 63 },
    "labelName":  { "type": "string", "pattern": "^[a-zA-Z_][a-zA-Z0-9_]*$", "maxLength": 64 },
    "labelValue": { "type": "string", "pattern": "^[A-Za-z0-9_.:/-]+$", "minLength": 1, "maxLength": 128 },
    "labelMap": {
      "type": "object",
      "propertyNames": { "$ref": "#/$defs/labelName" },
      "additionalProperties": { "$ref": "#/$defs/labelValue" }
    },
    "duration": { "type": "string", "pattern": "^([0-9]+(ms|s|m|h|d|w|y))+$" },
    "selector": {
      "type": "object",
      "additionalProperties": false,
      "properties": { "matchLabels": { "$ref": "#/$defs/labelMap" } }
    },
    "slo": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name", "objective", "sli"],
      "properties": {
        "name":        { "$ref": "#/$defs/dnsName" },
        "description": { "type": "string", "maxLength": 1024 },
        "objective":   { "type": "number", "exclusiveMinimum": 0, "exclusiveMaximum": 100 },
        "window":      { "$ref": "#/$defs/duration" },
        "labels":      { "$ref": "#/$defs/labelMap" },
        "sli":         { "$ref": "#/$defs/sli" },
        "alerting":    { "$ref": "#/$defs/alerting" }
      }
    },
    "sli": {
      "type": "object",
      "additionalProperties": false,
      "minProperties": 1,
      "maxProperties": 1,
      "properties": {
        "latency": {
          "type": "object", "additionalProperties": false,
          "required": ["stage", "threshold"],
          "properties": {
            "stage":     { "enum": ["ttft", "end_to_end"] },
            "source":    { "enum": ["client", "server"] },
            "threshold": { "$ref": "#/$defs/duration" },
            "selector":  { "$ref": "#/$defs/selector" }
          }
        },
        "errorRatio": {
          "type": "object", "additionalProperties": false,
          "properties": {
            "excludeErrorTypes": { "type": "array", "items": { "$ref": "#/$defs/labelValue" } },
            "selector":          { "$ref": "#/$defs/selector" }
          }
        },
        "cost": {
          "type": "object", "additionalProperties": false,
          "required": ["per", "thresholdUSD"],
          "properties": {
            "per":          { "enum": ["request", "task"] },
            "thresholdUSD": { "type": "number", "exclusiveMinimum": 0 },
            "selector":     { "$ref": "#/$defs/selector" }
          }
        },
        "evalScore": {
          "type": "object", "additionalProperties": false,
          "required": ["evaluator"],
          "oneOf": [ { "required": ["minScore"] }, { "required": ["passLabels"] } ],
          "properties": {
            "evaluator":  { "$ref": "#/$defs/labelValue" },
            "minScore":   { "type": "number" },
            "passLabels": { "type": "array", "minItems": 1, "items": { "$ref": "#/$defs/labelValue" } },
            "selector":   { "$ref": "#/$defs/selector" }
          }
        },
        "evalCoverage": {
          "type": "object", "additionalProperties": false,
          "required": ["evaluator"],
          "properties": {
            "evaluator": { "$ref": "#/$defs/labelValue" },
            "selector":  { "$ref": "#/$defs/selector" }
          }
        },
        "toolSuccess": {
          "type": "object", "additionalProperties": false,
          "properties": {
            "tools":    { "type": "array", "items": { "$ref": "#/$defs/labelValue" } },
            "selector": { "$ref": "#/$defs/selector" }
          }
        },
        "judgeAgreement": {
          "type": "object", "additionalProperties": false,
          "required": ["judge", "reference"],
          "properties": {
            "judge":     { "$ref": "#/$defs/labelValue" },
            "reference": { "$ref": "#/$defs/labelValue" },
            "selector":  { "$ref": "#/$defs/selector" }
          }
        }
      }
    },
    "alerting": {
      "type": "object", "additionalProperties": false,
      "properties": {
        "disabled":    { "type": "boolean" },
        "name":        { "type": "string", "pattern": "^[A-Za-z][A-Za-z0-9]*$", "maxLength": 128 },
        "labels":      { "$ref": "#/$defs/labelMap" },
        "annotations": { "type": "object", "additionalProperties": { "type": "string" } },
        "page":        { "$ref": "#/$defs/alertPolicy" },
        "ticket":      { "$ref": "#/$defs/alertPolicy" }
      }
    },
    "alertPolicy": {
      "type": "object", "additionalProperties": false,
      "properties": {
        "disabled": { "type": "boolean" },
        "labels":   { "$ref": "#/$defs/labelMap" },
        "windows": {
          "type": "array",
          "items": {
            "type": "object", "additionalProperties": false,
            "required": ["long", "short", "burnRate"],
            "properties": {
              "long":     { "$ref": "#/$defs/duration" },
              "short":    { "$ref": "#/$defs/duration" },
              "burnRate": { "type": "number", "exclusiveMinimum": 0 }
            }
          }
        }
      }
    }
  }
}
```

## 5. Validation

Two passes. Both produce `Diagnostic{Severity, Path, Line, Col, Message, Hint}`:

```
examples/specs/bad.yaml:23:20: error: spec.slos[0].sli.latency.threshold: "2 seconds" is not a duration
    hint: use a Prometheus duration such as "2s", "500ms" or "1m30s"
examples/specs/bad.yaml:31:9: error: spec.labels.user_id: label "user_id" is not on the allowlist
    hint: metric labels must have bounded cardinality; allowed keys are team, tier, env, ... (see docs/labels.md)
```

**Schema pass:** maps JSON Schema error kinds (required, enum, pattern,
additionalProperties, oneOf...) to our own messages and hints. For
`additionalProperties`, it suggests the closest valid key by edit distance
("unknown field `objectve`; did you mean `objective`?").

**Semantic pass** (things the schema can't express well):
- Every SLO name is unique within the spec.
- Label **keys** (spec.labels, slo.labels, selectors, alerting labels) must be
  on the allowlist in `internal/labels`. The allowlist starts as:
  `team, tier, env, severity, service_name, deployment_environment,
  gen_ai_operation_name, gen_ai_provider_name, gen_ai_request_model,
  gen_ai_response_model, gen_ai_agent_name, gen_ai_tool_name,
  gen_ai_evaluation_name, error_type`. Keys we know are unbounded
  (`*prompt*`, `*user*`, `*session*`, `*message*`, `*content*`,
  `*input*`, `*output*`, `*query*`, `*conversation*`) get a specific error
  that explains why.
- Label **values** match `^[A-Za-z0-9_.:/-]{1,128}$` (no whitespace, so
  free text can't get in).
- Reserved label keys (`alertname`, `__*`, `sloe_*`, `le`) can't be set.
- Durations parse. Burn-rate windows: `short < long <= window`, and
  `burnRate <= window/long` (a higher rate can never fire).
- Objective `> 99.99` gives a warning: the error budget is too small to
  alert on reliably at typical traffic.
- Latency threshold not on the semconv advisory bucket boundaries gives a
  warning: the `le` matcher would miss.
- A window other than 28d/30d gives a warning (fine, but the default burn
  rates are scaled).

`sloe validate` exits 0 when there are no errors (warnings are printed), 1
when there are errors, and 2 on a usage or IO error. `--format=json` gives
CI-friendly output.

## 6. Dependencies to approve

| Module | Why | License |
|---|---|---|
| `go.yaml.in/yaml/v3` | YAML with node positions (line/col in errors); the maintained successor of the archived `gopkg.in/yaml.v3` | MIT / Apache-2.0 |
| `github.com/santhosh-tekuri/jsonschema/v6` | Draft 2020-12 validator with structured errors (instance paths); only transitive dep is `golang.org/x/text` | Apache-2.0 |

No CLI framework: stdlib `flag` with a small subcommand switch until there
are enough commands to justify cobra. No assertion libraries: golden files
use stdlib with a `-update` flag.

CI tools (installed as binaries, not Go deps): `golangci-lint`, `promtool`
(pinned Prometheus release).

## 7. Open questions

- **Q1 API group/domain.** `sloeval.dev` is a placeholder. A CRD group
  should be a domain you control. What should it be?
- **Q2 Metric naming.** Prometheus 3 / OTLP ingestion can keep UTF-8 names
  (`gen_ai.client.operation.duration`) or translate them
  (`gen_ai_client_operation_duration_seconds`). Should we support both via a
  spec- or CLI-level `--metric-naming=underscore|utf8`, with underscore as
  the default?
- **Q3 Cost per task.** What is a "task" boundary? My proposal is the root
  `invoke_agent` span (or a configurable span attribute); the connector sums
  the cost of its child spans.
- **Q4 judge_agreement source.** I assume the Python judge runner emits
  `sloe_judge_agreement_total{judge,reference,outcome}`. The alternative is
  for the connector to join two evaluation events by response ID, which is
  stateful and harder. OK to go with the runner?
- **Q5 Selectors.** Equality-only in v1alpha1, or allow regex now?
- **Q6 Raw escape hatch.** Should there be a `custom` SLI (good/total
  PromQL, like Sloth `raw`)? It's useful, but the compiler couldn't enforce
  the label allowlist inside arbitrary PromQL. I suggest deferring it.
- **Q7 TTFT metric.** Semconv defines server-side
  `gen_ai.server.time_to_first_token`. The client-side TTFT metric is still
  in flux. I'll keep both candidates in `internal/semconv` and pin a semconv
  version. Which side do your users usually instrument?

## 8. Decisions on open questions

- **Q1 API group:** `sloeval.dev`, provided the owner registers that domain
  before v0.1.0. If they don't, change the single `Group` constant and the
  schema `const` before the first release. After the release it's a breaking change.
- **Q2 Metric names:** both are supported. This is a CLI/compile option,
  `--metric-naming=underscore|utf8` (default `underscore`), not a spec field:
  naming depends on how the backend ingests, not on the SLO. Both forms live
  in `internal/semconv`.
- **Q3 Cost per task:** a task is the outermost `invoke_agent` span (no
  `invoke_agent` ancestor). A span attribute `sloe.task.id` overrides this
  for frameworks that don't nest spans that way. The connector buffers spans
  per trace, bounded by time and count. Late or overflowing spans are
  dropped and counted in a self-metric.
- **Q4 judgeAgreement:** the Python judge runner emits
  `sloe_judge_agreement_total{judge,reference,outcome}`. The connector stays
  stateless for evaluations.
- **Q5 Selectors:** equality-only in v1alpha1. If more is needed later, add
  `matchExpressions` with `In`/`NotIn` (bounded and allowlist-checkable)
  rather than raw regex.
- **Q6 Raw escape hatch:** deferred. Revisit in v1alpha2 as a `custom` SLI
  that must declare its metrics and labels, so the allowlist can still be checked.
- **Q7 TTFT:** application teams mostly instrument the client, while
  server-side TTFT exists only for teams running their own model servers.
  `latency.source: client|server` defaults to `client`:
  - `end_to_end` + `client` -> `gen_ai.client.operation.duration`.
  - `ttft` + `server` -> `gen_ai.server.time_to_first_token`.
  - `ttft` + `client`: the client metric name is still changing, so it is
    the name the SDK/connector emits, pinned in `internal/semconv`.
