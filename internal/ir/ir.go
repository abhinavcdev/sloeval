// Package ir builds the normalized intermediate representation of validated
// AISLO specs: resolved SLIs as ratios of rates, objectives as ratios,
// merged alerting, and burn-rate windows scaled to each SLO window.
//
// Backends consume only this package's types (design 0002 section 3).
package ir

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abhinavcdev/sloeval/internal/semconv"
	"github.com/abhinavcdev/sloeval/internal/sli"
	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
)

// Labels set by sloe on every generated series.
const (
	LabelID        = "sloe_id"
	LabelService   = "sloe_service"
	LabelSLO       = "sloe_slo"
	LabelWindow    = "sloe_window"
	LabelSeverity  = "sloe_severity"
	LabelSLIKind   = "sloe_sli_kind"
	LabelObjective = "sloe_objective"
)

// Severities.
const (
	SeverityPage   = "page"
	SeverityTicket = "ticket"
)

// Spec is one AISLO document.
type Spec struct {
	// ID is "<name>" or "<namespace>.<name>"; it names output files.
	ID        string
	Namespace string
	Name      string
	Service   string
	SLOs      []SLO
}

// SLO is one normalized SLO.
type SLO struct {
	// ID is the sloe_id label value: "<namespace>/<name>/<slo>" or "<name>/<slo>".
	ID          string
	Service     string
	Name        string
	Description string
	Objective   float64 // ratio, e.g. 0.995
	Budget      float64 // 1 - Objective
	Window      time.Duration
	// Labels are the static labels (spec.labels merged with slo.labels).
	Labels map[string]string
	SLI    SLI
	Alerts []Alert
	// RateWindows are the windows for which an error ratio is recorded, sorted.
	RateWindows []time.Duration
}

// SeriesLabels returns the labels on every series generated for the SLO.
func (s *SLO) SeriesLabels() map[string]string {
	out := make(map[string]string, len(s.Labels)+4)
	for k, v := range s.Labels {
		out[k] = v
	}
	out[LabelID] = s.ID
	out[LabelService] = s.Service
	out[LabelSLO] = s.Name
	out[LabelWindow] = v1alpha1.FormatDuration(s.Window)
	return out
}

// SLI is an error ratio: sum(Bad) / sum(Total), both rates.
type SLI struct {
	Kind  string
	Bad   []Term
	Total []Term
	// ClampBad clamps the bad rate at zero (coverage: total - covered).
	ClampBad bool
	// Matchers is the merged selector, for reference by backends and tests.
	Matchers []Matcher
}

// Term is one signed sum(rate(...)) term.
type Term struct {
	Negative bool
	Series   Series
	// ZeroIfAbsent renders "or vector(0)": absence of the series means zero
	// (e.g. no error series yet), not "no data".
	ZeroIfAbsent bool
}

// Suffix selects a histogram series.
type Suffix string

// Series suffixes.
const (
	SuffixNone   Suffix = ""
	SuffixBucket Suffix = "_bucket"
	SuffixCount  Suffix = "_count"
)

// Series is a metric selector.
type Series struct {
	Metric   semconv.Metric
	Suffix   Suffix
	Matchers []Matcher
}

// Name returns the Prometheus series name in naming mode n.
func (s Series) Name(n semconv.Naming) string {
	switch s.Suffix {
	case SuffixBucket:
		return s.Metric.Bucket(n)
	case SuffixCount:
		return s.Metric.Count(n)
	}
	return s.Metric.Base(n)
}

// MatchOp is a PromQL label matcher operator.
type MatchOp string

// Matcher operators.
const (
	Eq  MatchOp = "="
	Neq MatchOp = "!="
	Re  MatchOp = "=~"
	Nre MatchOp = "!~"
)

// Matcher is one label matcher. Label is in spec (underscore) form; backends
// translate it with semconv.SpecLabel.
type Matcher struct {
	Label string
	Op    MatchOp
	Value string
}

// Alert is one alerting rule (one severity of one SLO).
type Alert struct {
	Severity    string
	Name        string
	Windows     []BurnWindow
	Labels      map[string]string
	Annotations map[string]string
}

// BurnWindow is one multi-window burn-rate condition.
type BurnWindow struct {
	Long, Short time.Duration
	BurnRate    float64
}

// Threshold is the error-ratio threshold of the window for budget b.
func (w BurnWindow) Threshold(budget float64) float64 { return Round(w.BurnRate * budget) }

// Round removes float noise (10 decimal places) so generated numbers are stable.
func Round(f float64) float64 { return math.Round(f*1e10) / 1e10 }

// FormatFloat renders f without float noise or exponent.
func FormatFloat(f float64) string { return strconv.FormatFloat(Round(f), 'f', -1, 64) }

// Build converts a validated AISLO into IR. It returns an error only for
// inputs validation should have rejected.
func Build(obj *v1alpha1.AISLO) (*Spec, error) {
	s := &Spec{
		Namespace: obj.Metadata.Namespace,
		Name:      obj.Metadata.Name,
		Service:   obj.Spec.Service,
		ID:        obj.Metadata.Name,
	}
	idPrefix := obj.Metadata.Name
	if s.Namespace != "" {
		s.ID = s.Namespace + "." + s.Name
		idPrefix = s.Namespace + "/" + s.Name
	}
	for i := range obj.Spec.SLOs {
		slo, err := buildSLO(&obj.Spec, &obj.Spec.SLOs[i], idPrefix)
		if err != nil {
			return nil, fmt.Errorf("spec.slos[%d] (%s): %w", i, obj.Spec.SLOs[i].Name, err)
		}
		s.SLOs = append(s.SLOs, *slo)
	}
	// promtool tests and silences address alerts by name, so each SLO needs its own.
	owner := map[string]string{}
	for _, slo := range s.SLOs {
		for _, a := range slo.Alerts {
			if o, ok := owner[a.Name]; ok && o != slo.Name {
				return nil, fmt.Errorf("alert name %q is used by SLOs %q and %q; set a distinct alerting.name", a.Name, o, slo.Name)
			}
			owner[a.Name] = slo.Name
		}
	}
	return s, nil
}

func buildSLO(spec *v1alpha1.AISLOSpec, in *v1alpha1.SLO, idPrefix string) (*SLO, error) {
	windowStr := in.Window
	if windowStr == "" {
		windowStr = v1alpha1.DefaultWindow
	}
	window, err := windowStr.Parse()
	if err != nil {
		return nil, err
	}
	obj := Round(in.Objective / 100)
	out := &SLO{
		ID:          idPrefix + "/" + in.Name,
		Service:     spec.Service,
		Name:        in.Name,
		Description: in.Description,
		Objective:   obj,
		Budget:      Round(1 - obj),
		Window:      window,
		Labels:      merge(spec.Labels, in.Labels),
	}
	out.SLI, err = buildSLI(&in.SLI, spec.Selector)
	if err != nil {
		return nil, err
	}
	out.Alerts, err = buildAlerts(spec, in, out)
	if err != nil {
		return nil, err
	}
	ws := map[time.Duration]bool{5 * time.Minute: true, time.Hour: true}
	for _, a := range out.Alerts {
		for _, w := range a.Windows {
			ws[w.Long], ws[w.Short] = true, true
		}
	}
	for w := range ws {
		out.RateWindows = append(out.RateWindows, w)
	}
	slices.Sort(out.RateWindows)
	return out, nil
}

func merge(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// selectorMatchers merges the spec and SLI selectors (SLI wins) into sorted
// equality matchers.
func selectorMatchers(specSel, sliSel *v1alpha1.Selector) []Matcher {
	m := map[string]string{}
	if specSel != nil {
		m = merge(m, specSel.MatchLabels)
	}
	if sliSel != nil {
		m = merge(m, sliSel.MatchLabels)
	}
	out := make([]Matcher, 0, len(m))
	for k, v := range m {
		out = append(out, Matcher{Label: k, Op: Eq, Value: v})
	}
	sortMatchers(out)
	return out
}

func sortMatchers(ms []Matcher) {
	slices.SortFunc(ms, func(a, b Matcher) int {
		if c := strings.Compare(a.Label, b.Label); c != 0 {
			return c
		}
		if c := strings.Compare(string(a.Op), string(b.Op)); c != 0 {
			return c
		}
		return strings.Compare(a.Value, b.Value)
	})
}

func with(base []Matcher, extra ...Matcher) []Matcher {
	out := append(slices.Clone(base), extra...)
	sortMatchers(out)
	return out
}

// regexAlt renders values as an anchored-by-PromQL alternation, escaped.
func regexAlt(values []string) string {
	q := make([]string, len(values))
	for i, v := range values {
		q[i] = regexp.QuoteMeta(v)
	}
	return strings.Join(q, "|")
}

// LeMatcher matches a histogram bucket boundary. Integer boundaries match
// both "1" (OTLP) and "1.0" (Prometheus 3 scrape normalization).
func LeMatcher(b float64) Matcher {
	s := FormatFloat(b)
	if b == math.Trunc(b) {
		return Matcher{Label: "le", Op: Re, Value: s + `|` + regexp.QuoteMeta(s+".0")}
	}
	return Matcher{Label: "le", Op: Eq, Value: s}
}

var (
	errorTypeLabel  = semconv.PromLabel(semconv.AttrErrorType)
	evalNameLabel   = semconv.PromLabel(semconv.AttrEvaluationName)
	scoreLabelLabel = semconv.PromLabel(semconv.AttrEvaluationScoreLabel)
	toolNameLabel   = semconv.PromLabel(semconv.AttrToolName)
)

func term(m semconv.Metric, suffix Suffix, ms []Matcher) Term {
	return Term{Series: Series{Metric: m, Suffix: suffix, Matchers: ms}}
}

func neg(t Term) Term          { t.Negative = true; return t }
func zeroIfAbsent(t Term) Term { t.ZeroIfAbsent = true; return t }

// bucketBelow returns the largest bucket boundary strictly below v.
func bucketBelow(m semconv.Metric, v float64) (float64, bool) {
	best, ok := 0.0, false
	for _, b := range m.Buckets {
		if b < v-1e-12 {
			best, ok = b, true
		}
	}
	return best, ok
}

func buildSLI(in *v1alpha1.SLI, specSel *v1alpha1.Selector) (SLI, error) {
	base := selectorMatchers(specSel, in.Selector())
	out := SLI{Kind: in.Kind(), Matchers: base}
	switch {
	case in.Latency != nil:
		m, ok := sli.LatencyMetric(in.Latency)
		if !ok {
			return out, fmt.Errorf("unsupported latency stage/target combination")
		}
		thr, err := in.Latency.Threshold.Parse()
		if err != nil {
			return out, err
		}
		ms := base
		if m.Carries(errorTypeLabel) {
			ms = with(base, Matcher{Label: errorTypeLabel, Op: Eq, Value: ""})
		}
		out.Bad = []Term{
			term(m, SuffixCount, ms),
			neg(term(m, SuffixBucket, with(ms, LeMatcher(thr.Seconds())))),
		}
		out.Total = []Term{term(m, SuffixCount, ms)}

	case in.ErrorRatio != nil:
		m := sli.OperationMetric(in.ErrorRatio.Target)
		ms := base
		if ex := in.ErrorRatio.ExcludeErrorTypes; len(ex) > 0 {
			ms = with(base, Matcher{Label: errorTypeLabel, Op: Nre, Value: regexAlt(ex)})
		}
		out.Bad = []Term{zeroIfAbsent(term(m, SuffixCount, with(ms, Matcher{Label: errorTypeLabel, Op: Neq, Value: ""})))}
		out.Total = []Term{term(m, SuffixCount, ms)}

	case in.Cost != nil:
		m := semconv.CostPerRequest
		if in.Cost.Per == v1alpha1.CostPerTask {
			m = semconv.CostPerTask
		}
		out.Bad = []Term{
			term(m, SuffixCount, base),
			neg(term(m, SuffixBucket, with(base, LeMatcher(in.Cost.ThresholdUSD)))),
		}
		out.Total = []Term{term(m, SuffixCount, base)}

	case in.EvalScore != nil:
		e := in.EvalScore
		ms := with(base, Matcher{Label: evalNameLabel, Op: Eq, Value: e.Evaluator})
		if e.MinScore != nil {
			m := semconv.EvaluationScore
			below, ok := bucketBelow(m, *e.MinScore)
			if !ok {
				return out, fmt.Errorf("minScore %v must be above the lowest bucket boundary of %s", *e.MinScore, m.Name)
			}
			out.Bad = []Term{zeroIfAbsent(term(m, SuffixBucket, with(ms, LeMatcher(below))))}
			out.Total = []Term{term(m, SuffixCount, ms)}
		} else {
			m := semconv.EvaluationResults
			labeled := with(ms, Matcher{Label: scoreLabelLabel, Op: Neq, Value: ""})
			out.Bad = []Term{zeroIfAbsent(term(m, SuffixNone, with(labeled, Matcher{Label: scoreLabelLabel, Op: Nre, Value: regexAlt(e.PassLabels)})))}
			out.Total = []Term{term(m, SuffixNone, labeled)}
		}

	case in.EvalCoverage != nil:
		op := sli.OperationMetric(in.EvalCoverage.Target)
		// Only successful operations are expected to be evaluated.
		opMs := with(base, Matcher{Label: errorTypeLabel, Op: Eq, Value: ""})
		covered := with(base, Matcher{Label: evalNameLabel, Op: Eq, Value: in.EvalCoverage.Evaluator})
		out.Bad = []Term{
			term(op, SuffixCount, opMs),
			zeroIfAbsent(neg(term(semconv.EvaluatedOperations, SuffixNone, covered))),
		}
		out.Total = []Term{term(op, SuffixCount, opMs)}
		out.ClampBad = true

	case in.ToolSuccess != nil:
		m := semconv.ExecuteToolDuration
		ms := base
		switch tools := in.ToolSuccess.Tools; len(tools) {
		case 0:
		case 1:
			ms = with(base, Matcher{Label: toolNameLabel, Op: Eq, Value: tools[0]})
		default:
			ms = with(base, Matcher{Label: toolNameLabel, Op: Re, Value: regexAlt(tools)})
		}
		out.Bad = []Term{zeroIfAbsent(term(m, SuffixCount, with(ms, Matcher{Label: errorTypeLabel, Op: Neq, Value: ""})))}
		out.Total = []Term{term(m, SuffixCount, ms)}

	case in.JudgeAgreement != nil:
		m := semconv.JudgeAgreement
		ms := with(base,
			Matcher{Label: semconv.LabelJudge, Op: Eq, Value: in.JudgeAgreement.Judge},
			Matcher{Label: semconv.LabelReference, Op: Eq, Value: in.JudgeAgreement.Reference})
		out.Bad = []Term{zeroIfAbsent(term(m, SuffixNone, with(ms, Matcher{Label: semconv.LabelOutcome, Op: Eq, Value: "disagree"})))}
		out.Total = []Term{term(m, SuffixNone, ms)}

	default:
		return out, fmt.Errorf("no SLI kind set")
	}
	return out, nil
}
