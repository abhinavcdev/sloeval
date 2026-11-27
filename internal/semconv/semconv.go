// Package semconv is the single place that names OpenTelemetry GenAI
// semantic-convention metrics, attributes and events, and the sloe-owned
// metrics derived from them, and that translates them to Prometheus names.
//
// The GenAI conventions are in Development status and change between
// releases. Nothing outside this file may hard-code a gen_ai.* name. To adopt
// a new semconv release: update Version, fix the names, attributes and
// buckets below, regenerate golden files, and note any renamed metric in the
// changelog. Only tagged releases are pinned, never main.
//
// Source: https://github.com/open-telemetry/semantic-conventions-genai
// (the GenAI conventions moved out of open-telemetry/semantic-conventions).
package semconv

import "strings"

// Version is the pinned semantic-conventions-genai release.
const Version = "v0.1.0"

// Unit is a UCUM unit as used by OTel instruments.
type Unit string

// Units used below.
const (
	UnitSeconds Unit = "s"
	UnitUSD     Unit = "{USD}" // annotation unit: no Prometheus suffix, so names carry "usd"
	UnitScore   Unit = "1"
	UnitCount   Unit = "{count}"
)

// InstrumentKind is the OTel instrument kind.
type InstrumentKind string

// Instrument kinds.
const (
	Histogram InstrumentKind = "histogram"
	Counter   InstrumentKind = "counter"
)

// Metric describes one OTel metric.
type Metric struct {
	Name string
	Kind InstrumentKind
	Unit Unit
	// Buckets are the advisory explicit bucket boundaries.
	Buckets []float64
	// Attrs are the attributes the metric carries, besides the promoted
	// resource attributes in ResourceAttrs (OTel names).
	Attrs []string
	// Owner is "semconv" for OTel-defined metrics and "sloe" for metrics
	// emitted by the sloe connector or judge runner.
	Owner string
}

var latencyBuckets = []float64{0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28, 2.56, 5.12, 10.24, 20.48, 40.96, 81.92}

var (
	inferenceAttrs = []string{AttrErrorType, AttrOperationName, AttrProviderName, AttrRequestModel, AttrResponseModel, AttrServerAddress, AttrServerPort}
	// ttftAttrs: TTFT is recorded for successful responses only, so no error.type.
	ttftAttrs = []string{AttrOperationName, AttrProviderName, AttrRequestModel, AttrResponseModel, AttrServerAddress, AttrServerPort}
	// operationAttrs are the bounded attributes the connector copies from the
	// evaluated or costed operation onto sloe-owned metrics.
	operationAttrs = []string{AttrOperationName, AttrProviderName, AttrRequestModel, AttrResponseModel, AttrAgentName}
)

// OTel GenAI metrics (semantic-conventions-genai v0.1.0).
var (
	// ClientInferenceDuration is end-to-end client latency of an inference call.
	// (gen_ai.client.operation.duration SHOULD NOT be used for inference in v0.1.0.)
	ClientInferenceDuration = Metric{
		Name: "gen_ai.client.inference.duration", Kind: Histogram, Unit: UnitSeconds,
		Buckets: latencyBuckets, Attrs: inferenceAttrs, Owner: "semconv",
	}
	// ClientTimeToFirstChunk is client-side time to first token/chunk (successful responses).
	ClientTimeToFirstChunk = Metric{
		Name: "gen_ai.client.inference.time_to_first_chunk", Kind: Histogram, Unit: UnitSeconds,
		Buckets: latencyBuckets, Attrs: ttftAttrs, Owner: "semconv",
	}
	// ServerRequestDuration is model-server request latency.
	ServerRequestDuration = Metric{
		Name: "gen_ai.server.request.duration", Kind: Histogram, Unit: UnitSeconds,
		Buckets: latencyBuckets, Attrs: inferenceAttrs, Owner: "semconv",
	}
	// ServerTimeToFirstToken is model-server time to first token (successful responses).
	ServerTimeToFirstToken = Metric{
		Name: "gen_ai.server.time_to_first_token", Kind: Histogram, Unit: UnitSeconds,
		Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.04, 0.06, 0.08, 0.1, 0.25, 0.5, 0.75, 1.0, 2.5, 5.0, 7.5, 10.0},
		Attrs:   ttftAttrs, Owner: "semconv",
	}
	// InvokeAgentDuration is an in-process agent invocation, measured by the agent runtime.
	InvokeAgentDuration = Metric{
		Name: "gen_ai.invoke_agent.duration", Kind: Histogram, Unit: UnitSeconds,
		Buckets: []float64{0.1, 0.2, 0.4, 0.8, 1.6, 3.2, 6.4, 12.8, 25.6, 51.2, 102.4, 204.8, 409.6, 900, 1800, 3600, 7200, 14400, 43200, 86400},
		Attrs:   []string{AttrErrorType, AttrAgentName, AttrOperationName, AttrRequestModel}, Owner: "semconv",
	}
	// ClientInvokeAgentDuration is a remote agent invocation, measured by the caller.
	ClientInvokeAgentDuration = Metric{
		Name: "gen_ai.client.invoke_agent.duration", Kind: Histogram, Unit: UnitSeconds, Buckets: latencyBuckets,
		Attrs: []string{AttrErrorType, AttrAgentName, AttrOperationName, AttrProviderName, AttrRequestModel, AttrServerAddress, AttrServerPort},
		Owner: "semconv",
	}
	// ExecuteToolDuration is the duration of one tool execution.
	ExecuteToolDuration = Metric{
		Name: "gen_ai.execute_tool.duration", Kind: Histogram, Unit: UnitSeconds, Buckets: latencyBuckets,
		Attrs: []string{AttrErrorType, AttrAgentName, AttrToolName, AttrToolType}, Owner: "semconv",
	}
)

// sloe-owned metrics, emitted by the connector (cost, evaluations) and the
// judge runner (agreement). Buckets here are defaults; the connector can be
// configured with others, and validation warns when a threshold misses them.
var (
	// CostPerRequest is the computed USD cost of one inference request.
	CostPerRequest = Metric{
		Name: "sloe.gen_ai.request.cost.usd", Kind: Histogram, Unit: UnitUSD,
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		Attrs:   operationAttrs, Owner: "sloe",
	}
	// CostPerTask is the computed USD cost of one agent task (root invoke_agent span).
	CostPerTask = Metric{
		Name: "sloe.gen_ai.task.cost.usd", Kind: Histogram, Unit: UnitUSD,
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50},
		Attrs:   []string{AttrAgentName}, Owner: "sloe",
	}
	// EvaluationScore is the distribution of gen_ai.evaluation.score.value.
	EvaluationScore = Metric{
		Name: "sloe.gen_ai.evaluation.score", Kind: Histogram, Unit: UnitScore,
		Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.75, 0.8, 0.85, 0.9, 0.95, 1, 2, 3, 4, 5},
		Attrs:   append([]string{AttrEvaluationName}, operationAttrs...), Owner: "sloe",
	}
	// EvaluationResults counts gen_ai.evaluation.result events by name and score label.
	EvaluationResults = Metric{
		Name: "sloe.gen_ai.evaluation.results", Kind: Counter, Unit: UnitCount,
		Attrs: append([]string{AttrEvaluationName, AttrEvaluationScoreLabel}, operationAttrs...), Owner: "sloe",
	}
	// EvaluatedOperations counts operations with at least one result per evaluator.
	EvaluatedOperations = Metric{
		Name: "sloe.gen_ai.evaluation.evaluated_operations", Kind: Counter, Unit: UnitCount,
		Attrs: append([]string{AttrEvaluationName}, operationAttrs...), Owner: "sloe",
	}
	// JudgeAgreement counts judge-vs-reference comparisons (outcome=agree|disagree).
	JudgeAgreement = Metric{
		Name: "sloe.judge.agreement", Kind: Counter, Unit: UnitCount,
		Attrs: []string{LabelJudge, LabelReference, LabelOutcome}, Owner: "sloe",
	}
)

// All lists every metric above.
var All = []Metric{
	ClientInferenceDuration, ClientTimeToFirstChunk, ServerRequestDuration, ServerTimeToFirstToken,
	InvokeAgentDuration, ClientInvokeAgentDuration, ExecuteToolDuration,
	CostPerRequest, CostPerTask, EvaluationScore, EvaluationResults, EvaluatedOperations, JudgeAgreement,
}

// Event names.
const (
	EventEvaluationResult = "gen_ai.evaluation.result"
)

// Attribute names (OTel form).
const (
	AttrOperationName        = "gen_ai.operation.name"
	AttrProviderName         = "gen_ai.provider.name"
	AttrRequestModel         = "gen_ai.request.model"
	AttrResponseModel        = "gen_ai.response.model"
	AttrAgentName            = "gen_ai.agent.name"
	AttrToolName             = "gen_ai.tool.name"
	AttrToolType             = "gen_ai.tool.type"
	AttrEvaluationName       = "gen_ai.evaluation.name"
	AttrEvaluationScoreLabel = "gen_ai.evaluation.score.label"
	AttrEvaluationScoreValue = "gen_ai.evaluation.score.value"
	// AttrEvaluationExplanation is free text. It must never become a label.
	AttrEvaluationExplanation = "gen_ai.evaluation.explanation"
	AttrResponseID            = "gen_ai.response.id"
	AttrErrorType             = "error.type"
	AttrServerAddress         = "server.address"
	AttrServerPort            = "server.port"
	AttrServiceName           = "service.name"
	AttrDeploymentEnvironment = "deployment.environment.name"
	// AttrSloeTaskID overrides task-root detection for cost per task (sloe-owned).
	AttrSloeTaskID = "sloe.task.id"
)

// ResourceAttrs are resource attributes users are expected to promote to
// labels in their OTLP ingestion config. They are treated as present on every metric.
var ResourceAttrs = []string{AttrServiceName, AttrDeploymentEnvironment}

// Operation names (values of gen_ai.operation.name) used by sloe.
const (
	OpInvokeAgent = "invoke_agent"
	OpExecuteTool = "execute_tool"
)

// Label names of sloe-owned series that are not OTel attributes. They are
// the same in every naming mode.
const (
	LabelOutcome   = "outcome"   // judge agreement: agree|disagree
	LabelJudge     = "judge"     // judge evaluator name
	LabelReference = "reference" // reference evaluator name
)

// Naming selects how OTel names are translated to Prometheus names.
type Naming string

// Naming modes, matching Prometheus OTLP translation strategies.
const (
	// NamingUnderscore matches UnderscoreEscapingWithSuffixes (the default).
	NamingUnderscore Naming = "underscore"
	// NamingUTF8 matches NoUTF8EscapingWithSuffixes. Experimental: suffix
	// rules are checked end to end in Phase 3.
	NamingUTF8 Naming = "utf8"
)

// ParseNaming parses a --metric-naming value.
func ParseNaming(s string) (Naming, bool) {
	switch Naming(s) {
	case NamingUnderscore, NamingUTF8:
		return Naming(s), true
	}
	return "", false
}

func escape(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			b[i] = '_'
		}
	}
	return string(b)
}

// PromLabel translates an OTel attribute name to a Prometheus label name in
// underscore mode (non [a-zA-Z0-9_] -> '_').
func PromLabel(attr string) string { return escape(attr) }

// Label returns the label name for an OTel attribute in the given mode.
func Label(attr string, n Naming) string {
	if n == NamingUTF8 {
		return attr
	}
	return escape(attr)
}

var labelToAttr = func() map[string]string {
	m := map[string]string{}
	for _, a := range []string{
		AttrOperationName, AttrProviderName, AttrRequestModel, AttrResponseModel, AttrAgentName,
		AttrToolName, AttrToolType, AttrEvaluationName, AttrEvaluationScoreLabel, AttrErrorType,
		AttrServerAddress, AttrServerPort, AttrServiceName, AttrDeploymentEnvironment,
	} {
		m[escape(a)] = a
	}
	return m
}()

// AttrForLabel maps an underscore label name (as written in specs) back to
// its OTel attribute, if it is one.
func AttrForLabel(label string) (string, bool) {
	a, ok := labelToAttr[label]
	return a, ok
}

// SpecLabel translates a label key written in a spec (always underscore form)
// into the label name for mode n.
func SpecLabel(label string, n Naming) string {
	if a, ok := AttrForLabel(label); ok {
		return Label(a, n)
	}
	return label
}

// Carries reports whether the metric carries the spec label key (underscore
// form), counting promoted resource attributes.
func (m Metric) Carries(label string) bool {
	a, ok := AttrForLabel(label)
	if !ok {
		a = label // sloe-owned label names such as judge
	}
	for _, x := range m.Attrs {
		if x == a {
			return true
		}
	}
	for _, x := range ResourceAttrs {
		if x == a {
			return true
		}
	}
	return false
}

func (m Metric) unitSuffix() string {
	if m.Unit == UnitSeconds {
		return "_seconds"
	}
	return ""
}

// Base returns the Prometheus base name (without _bucket/_count/_sum; with
// _total for counters).
func (m Metric) Base(n Naming) string {
	name := m.Name
	if n != NamingUTF8 {
		name = escape(name)
	}
	name += m.unitSuffix()
	if m.Kind == Counter && !strings.HasSuffix(name, "_total") {
		name += "_total"
	}
	return name
}

// Bucket returns the histogram _bucket series name.
func (m Metric) Bucket(n Naming) string { return m.Base(n) + "_bucket" }

// Count returns the histogram _count series name.
func (m Metric) Count(n Naming) string { return m.Base(n) + "_count" }
