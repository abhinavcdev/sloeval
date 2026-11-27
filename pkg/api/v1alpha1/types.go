package v1alpha1

// API identity.
const (
	// Group is the API group. Placeholder until the domain is registered;
	// it must be final before v0.1.0 (design 0001, Q1).
	Group = "sloeval.dev"
	// Version is the API version.
	Version = "v1alpha1"
	// APIVersion is the value of the apiVersion field.
	APIVersion = Group + "/" + Version
	// Kind is the value of the kind field.
	Kind = "AISLO"
)

// AISLO is the top-level document. Its JSON shape matches a Kubernetes object
// so the operator can wrap it later without changing the format.
type AISLO struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       AISLOSpec  `json:"spec"`
}

// ObjectMeta is the subset of Kubernetes ObjectMeta that AISLO uses.
type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// AISLOSpec describes the SLOs of one service.
type AISLOSpec struct {
	// Service is the name of the service the SLOs belong to.
	Service     string `json:"service"`
	Description string `json:"description,omitempty"`
	// Labels are static labels added to every generated series and alert.
	// Keys must be on the label allowlist.
	Labels map[string]string `json:"labels,omitempty"`
	// Selector is merged into every SLI query.
	Selector *Selector `json:"selector,omitempty"`
	// Alerting holds defaults for every SLO; each SLO can override them.
	Alerting *Alerting `json:"alerting,omitempty"`
	SLOs     []SLO     `json:"slos"`
}

// SLO is a single service level objective.
type SLO struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Objective is the target percentage, 0 < objective < 100.
	Objective float64 `json:"objective"`
	// Window is the SLO window. Defaults to DefaultWindow.
	Window   Duration          `json:"window,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	SLI      SLI               `json:"sli"`
	Alerting *Alerting         `json:"alerting,omitempty"`
}

// Selector holds equality label matchers. Keys are Prometheus label names
// and must be on the label allowlist.
type Selector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// SLI must set exactly one field.
type SLI struct {
	Latency        *LatencySLI        `json:"latency,omitempty"`
	ErrorRatio     *ErrorRatioSLI     `json:"errorRatio,omitempty"`
	Cost           *CostSLI           `json:"cost,omitempty"`
	EvalScore      *EvalScoreSLI      `json:"evalScore,omitempty"`
	EvalCoverage   *EvalCoverageSLI   `json:"evalCoverage,omitempty"`
	ToolSuccess    *ToolSuccessSLI    `json:"toolSuccess,omitempty"`
	JudgeAgreement *JudgeAgreementSLI `json:"judgeAgreement,omitempty"`
}

// LatencyStage selects which latency is measured.
type LatencyStage string

// Latency stages.
const (
	StageTTFT     LatencyStage = "ttft"
	StageEndToEnd LatencyStage = "end_to_end"
)

// Target selects which operations an SLI measures.
type Target string

// Targets.
const (
	// TargetInference measures model inference calls (the default).
	TargetInference Target = "inference"
	// TargetAgent measures agent invocations (they carry gen_ai.agent.name).
	TargetAgent Target = "agent"
)

// LatencySource selects whether latency is measured by the client or the model server.
type LatencySource string

// Latency sources.
const (
	SourceClient LatencySource = "client"
	SourceServer LatencySource = "server"
)

// LatencySLI measures an SLI where good = requests with duration <= Threshold.
type LatencySLI struct {
	Stage LatencyStage `json:"stage"`
	// Target defaults to TargetInference. TTFT exists for inference only.
	Target Target `json:"target,omitempty"`
	// Source defaults to SourceClient for inference and SourceServer (the
	// agent runtime's own measurement) for agents.
	Source    LatencySource `json:"source,omitempty"`
	Threshold Duration      `json:"threshold"`
	Selector  *Selector     `json:"selector,omitempty"`
}

// ErrorRatioSLI measures an SLI where good = operations with no error.type, except listed types.
type ErrorRatioSLI struct {
	// Target defaults to TargetInference.
	Target            Target    `json:"target,omitempty"`
	ExcludeErrorTypes []string  `json:"excludeErrorTypes,omitempty"`
	Selector          *Selector `json:"selector,omitempty"`
}

// CostUnit is what a cost is measured per.
type CostUnit string

// Cost units.
const (
	CostPerRequest CostUnit = "request"
	CostPerTask    CostUnit = "task"
)

// CostSLI measures an SLI where good = requests or tasks whose cost is <= ThresholdUSD.
type CostSLI struct {
	Per          CostUnit  `json:"per"`
	ThresholdUSD float64   `json:"thresholdUSD"`
	Selector     *Selector `json:"selector,omitempty"`
}

// EvalScoreSLI measures an SLI where good = evaluation results for Evaluator that pass.
// Exactly one of MinScore or PassLabels is set.
type EvalScoreSLI struct {
	Evaluator  string    `json:"evaluator"`
	MinScore   *float64  `json:"minScore,omitempty"`
	PassLabels []string  `json:"passLabels,omitempty"`
	Selector   *Selector `json:"selector,omitempty"`
}

// EvalCoverageSLI measures an SLI where good = operations with at least one Evaluator result.
type EvalCoverageSLI struct {
	Evaluator string `json:"evaluator"`
	// Target defaults to TargetInference: the operations that should be evaluated.
	Target   Target    `json:"target,omitempty"`
	Selector *Selector `json:"selector,omitempty"`
}

// ToolSuccessSLI measures an SLI where good = tool executions with no error.type. Empty Tools means all tools.
type ToolSuccessSLI struct {
	Tools    []string  `json:"tools,omitempty"`
	Selector *Selector `json:"selector,omitempty"`
}

// JudgeAgreementSLI measures an SLI where good = items where Judge's label matches Reference's label.
type JudgeAgreementSLI struct {
	Judge     string    `json:"judge"`
	Reference string    `json:"reference"`
	Selector  *Selector `json:"selector,omitempty"`
}

// Alerting configures alerts for an SLO (or defaults for all SLOs).
type Alerting struct {
	// Disabled turns all alerts off. The nearest level that sets it wins, so
	// an SLO can set disabled: false to re-enable alerts disabled in spec.alerting.
	Disabled *bool `json:"disabled,omitempty"`
	// Name is the alert name prefix. Defaults to CamelCase(service + slo).
	Name string `json:"name,omitempty"`
	// Labels are routing labels; keys must be on the label allowlist.
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations may contain free text; they are not series labels.
	Annotations map[string]string `json:"annotations,omitempty"`
	Page        *AlertPolicy      `json:"page,omitempty"`
	Ticket      *AlertPolicy      `json:"ticket,omitempty"`
}

// AlertPolicy configures one alert severity.
type AlertPolicy struct {
	// Disabled turns this severity off; the nearest level that sets it wins.
	Disabled *bool             `json:"disabled,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	// Windows overrides the SRE Workbook default windows.
	Windows []BurnRateWindow `json:"windows,omitempty"`
}

// BurnRateWindow is one multi-window burn-rate condition.
type BurnRateWindow struct {
	Long     Duration `json:"long"`
	Short    Duration `json:"short"`
	BurnRate float64  `json:"burnRate"`
}

// Kind returns the JSON name of the SLI kind that is set, or "" if none.
// If several are set, it returns the first in declaration order.
func (s SLI) Kind() string {
	switch {
	case s.Latency != nil:
		return "latency"
	case s.ErrorRatio != nil:
		return "errorRatio"
	case s.Cost != nil:
		return "cost"
	case s.EvalScore != nil:
		return "evalScore"
	case s.EvalCoverage != nil:
		return "evalCoverage"
	case s.ToolSuccess != nil:
		return "toolSuccess"
	case s.JudgeAgreement != nil:
		return "judgeAgreement"
	}
	return ""
}

// Selector returns the SLI-level selector of whichever kind is set.
func (s SLI) Selector() *Selector {
	switch {
	case s.Latency != nil:
		return s.Latency.Selector
	case s.ErrorRatio != nil:
		return s.ErrorRatio.Selector
	case s.Cost != nil:
		return s.Cost.Selector
	case s.EvalScore != nil:
		return s.EvalScore.Selector
	case s.EvalCoverage != nil:
		return s.EvalCoverage.Selector
	case s.ToolSuccess != nil:
		return s.ToolSuccess.Selector
	case s.JudgeAgreement != nil:
		return s.JudgeAgreement.Selector
	}
	return nil
}
