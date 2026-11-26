// Package semconv is the single place that names OpenTelemetry GenAI
// semantic-convention metrics, attributes and events, and the sloe-owned
// metrics derived from them.
//
// The GenAI conventions are in Development status and change between
// releases. Nothing outside this file may hard-code a gen_ai.* name. To adopt
// a new semconv release: update Version, fix the names and buckets below,
// regenerate golden files, and note any renamed metric in the changelog.
//
// Source: https://github.com/open-telemetry/semantic-conventions-genai
// (the GenAI conventions moved out of open-telemetry/semantic-conventions).
package semconv

// Version is the pinned semantic-conventions-genai release.
const Version = "v0.1.0"

// Unit is a UCUM unit as used by OTel instruments.
type Unit string

// Units used below.
const (
	UnitSeconds Unit = "s"
	UnitUSD     Unit = "{USD}"
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

// Metric describes one OTel metric. PromName is the underscore-translated
// base name (as produced by Prometheus OTLP ingestion with unit suffixes);
// backends add _bucket/_count/_sum/_total as appropriate.
type Metric struct {
	Name     string
	PromName string
	Kind     InstrumentKind
	Unit     Unit
	// Buckets are the advisory explicit bucket boundaries.
	Buckets []float64
	// Owner is "semconv" for OTel-defined metrics and "sloe" for metrics
	// emitted by the sloe connector or judge runner.
	Owner string
}

var latencyBuckets = []float64{0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28, 2.56, 5.12, 10.24, 20.48, 40.96, 81.92}

// OTel GenAI metrics (semantic-conventions-genai v0.1.0).
var (
	// ClientInferenceDuration is end-to-end client latency of an inference call.
	// (gen_ai.client.operation.duration SHOULD NOT be used for inference in v0.1.0.)
	ClientInferenceDuration = Metric{
		Name: "gen_ai.client.inference.duration", PromName: "gen_ai_client_inference_duration_seconds",
		Kind: Histogram, Unit: UnitSeconds, Buckets: latencyBuckets, Owner: "semconv",
	}
	// ClientTimeToFirstChunk is client-side time to first token/chunk.
	ClientTimeToFirstChunk = Metric{
		Name: "gen_ai.client.inference.time_to_first_chunk", PromName: "gen_ai_client_inference_time_to_first_chunk_seconds",
		Kind: Histogram, Unit: UnitSeconds, Buckets: latencyBuckets, Owner: "semconv",
	}
	// ServerRequestDuration is model-server request latency.
	ServerRequestDuration = Metric{
		Name: "gen_ai.server.request.duration", PromName: "gen_ai_server_request_duration_seconds",
		Kind: Histogram, Unit: UnitSeconds, Buckets: latencyBuckets, Owner: "semconv",
	}
	// ServerTimeToFirstToken is model-server time to first token.
	ServerTimeToFirstToken = Metric{
		Name: "gen_ai.server.time_to_first_token", PromName: "gen_ai_server_time_to_first_token_seconds",
		Kind: Histogram, Unit: UnitSeconds, Owner: "semconv",
		Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.04, 0.06, 0.08, 0.1, 0.25, 0.5, 0.75, 1.0, 2.5, 5.0, 7.5, 10.0},
	}
	// ExecuteToolDuration is the duration of one tool execution (carries error.type).
	ExecuteToolDuration = Metric{
		Name: "gen_ai.execute_tool.duration", PromName: "gen_ai_execute_tool_duration_seconds",
		Kind: Histogram, Unit: UnitSeconds, Buckets: latencyBuckets, Owner: "semconv",
	}
)

// sloe-owned metrics, emitted by the connector (cost, evaluations) and the
// judge runner (agreement). Buckets here are defaults; the connector can be
// configured with others, and validation warns when a threshold misses them.
var (
	// CostPerRequest is the computed USD cost of one inference request.
	CostPerRequest = Metric{
		Name: "sloe.gen_ai.request.cost", PromName: "sloe_gen_ai_request_cost_usd",
		Kind: Histogram, Unit: UnitUSD, Owner: "sloe",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}
	// CostPerTask is the computed USD cost of one agent task (root invoke_agent span).
	CostPerTask = Metric{
		Name: "sloe.gen_ai.task.cost", PromName: "sloe_gen_ai_task_cost_usd",
		Kind: Histogram, Unit: UnitUSD, Owner: "sloe",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50},
	}
	// EvaluationScore is the distribution of gen_ai.evaluation.score.value.
	EvaluationScore = Metric{
		Name: "sloe.gen_ai.evaluation.score", PromName: "sloe_gen_ai_evaluation_score",
		Kind: Histogram, Unit: UnitScore, Owner: "sloe",
		Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.75, 0.8, 0.85, 0.9, 0.95, 1, 2, 3, 4, 5},
	}
	// EvaluationResults counts gen_ai.evaluation.result events by name and score label.
	EvaluationResults = Metric{
		Name: "sloe.gen_ai.evaluation.results", PromName: "sloe_gen_ai_evaluation_results_total",
		Kind: Counter, Unit: UnitCount, Owner: "sloe",
	}
	// EvaluatedOperations counts operations with at least one result per evaluator.
	EvaluatedOperations = Metric{
		Name: "sloe.gen_ai.evaluation.evaluated_operations", PromName: "sloe_gen_ai_evaluation_evaluated_operations_total",
		Kind: Counter, Unit: UnitCount, Owner: "sloe",
	}
	// JudgeAgreement counts judge-vs-reference comparisons (outcome=agree|disagree).
	JudgeAgreement = Metric{
		Name: "sloe.judge.agreement", PromName: "sloe_judge_agreement_total",
		Kind: Counter, Unit: UnitCount, Owner: "sloe",
	}
)

// Event names.
const (
	EventEvaluationResult = "gen_ai.evaluation.result"
)

// Attribute names (OTel form). Use PromLabel for the Prometheus label name.
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
	AttrServiceName           = "service.name"
	AttrDeploymentEnvironment = "deployment.environment.name"
	// AttrSloeTaskID overrides task-root detection for cost per task (sloe-owned).
	AttrSloeTaskID = "sloe.task.id"
)

// Operation names (values of gen_ai.operation.name) used by sloe.
const (
	OpInvokeAgent = "invoke_agent"
	OpExecuteTool = "execute_tool"
)

// PromLabel translates an OTel attribute name to a Prometheus label name the
// way OTLP ingestion does in underscore mode (non [a-zA-Z0-9_] -> '_').
func PromLabel(attr string) string {
	b := []byte(attr)
	for i, c := range b {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			b[i] = '_'
		}
	}
	return string(b)
}

// Label names of sloe-owned series that are not OTel attributes.
const (
	LabelOutcome   = "outcome"   // judge agreement: agree|disagree
	LabelJudge     = "judge"     // judge evaluator name
	LabelReference = "reference" // reference evaluator name
)
