package prometheus

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abhinavcdev/sloeval/internal/ir"
	"github.com/abhinavcdev/sloeval/internal/semconv"
)

// The promtool test file format.
type testFile struct {
	RuleFiles          []string   `yaml:"rule_files"`
	EvaluationInterval string     `yaml:"evaluation_interval"`
	Tests              []testCase `yaml:"tests"`
}

type testCase struct {
	Name        string       `yaml:"name"`
	Interval    string       `yaml:"interval"`
	InputSeries []inputSerie `yaml:"input_series"`
	AlertTests  []alertTest  `yaml:"alert_rule_test,omitempty"`
	ExprTests   []exprTest   `yaml:"promql_expr_test,omitempty"`
}

type inputSerie struct {
	Series string `yaml:"series"`
	Values string `yaml:"values"`
}

type alertTest struct {
	EvalTime  string     `yaml:"eval_time"`
	AlertName string     `yaml:"alertname"`
	ExpAlerts []expAlert `yaml:"exp_alerts"`
}

type expAlert struct {
	ExpLabels      map[string]string `yaml:"exp_labels"`
	ExpAnnotations map[string]string `yaml:"exp_annotations,omitempty"`
}

type exprTest struct {
	Expr       string      `yaml:"expr"`
	EvalTime   string      `yaml:"eval_time"`
	ExpSamples []expSample `yaml:"exp_samples"`
}

type expSample struct {
	Labels string  `yaml:"labels"`
	Value  float64 `yaml:"value"`
}

const (
	valueTolerance = "1e-9"
	testInterval   = time.Minute
	// testEval is when every case is evaluated. The synthetic series have a
	// constant error ratio and start together, so every window's ratio is
	// exactly r once each window holds two samples; evaluating after the
	// full 3d window would only make the tests slower (design 0002 section 7).
	testEval = 70 * time.Minute
)

// Tests renders a promtool unit-test file for the rule file named ruleFile.
// Every alert gets a firing case per burn-rate window pair, a quiet case
// just below the lowest threshold and a no-traffic case.
func Tests(s *ir.Spec, ruleFile string, opt Options) ([]byte, error) {
	n := opt.naming()
	f := testFile{RuleFiles: []string{ruleFile}, EvaluationInterval: "1m"}
	for i := range s.SLOs {
		cases, err := sloTests(&s.SLOs[i], n)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.SLOs[i].ID, err)
		}
		f.Tests = append(f.Tests, cases...)
	}
	return marshal(f)
}

type pair struct {
	severity  string
	threshold float64
	long      time.Duration
	short     time.Duration
	burnRate  float64
}

func sloTests(slo *ir.SLO, n semconv.Naming) ([]testCase, error) {
	var pairs []pair
	names := map[string]bool{}
	for _, a := range slo.Alerts {
		names[a.Name] = true
		for _, w := range a.Windows {
			pairs = append(pairs, pair{a.Severity, w.Threshold(slo.Budget), w.Long, w.Short, w.BurnRate})
		}
	}
	slices.SortStableFunc(pairs, func(a, b pair) int {
		switch {
		case a.threshold < b.threshold:
			return -1
		case a.threshold > b.threshold:
			return 1
		}
		return 0
	})
	// Total events per interval: large enough that bad counts stay well above
	// float noise even for tiny budgets.
	total := math.Round(1000 / slo.Budget)

	var out []testCase
	if len(pairs) == 0 {
		r := ir.Round(math.Min(0.5, 2*slo.Budget))
		tc, err := valueCase(slo, n, r, total, testEval, slo.ID+" records the error ratio")
		if err != nil {
			return nil, err
		}
		return []testCase{tc}, nil
	}

	seen := map[float64]bool{}
	for i, p := range pairs {
		upper := 1.0
		for _, q := range pairs[i+1:] {
			if q.threshold > p.threshold {
				upper = q.threshold
				break
			}
		}
		r := ir.Round(p.threshold * 1.1)
		if r >= upper {
			r = ir.Round((p.threshold + upper) / 2)
		}
		if r <= p.threshold || r > 1 || seen[r] {
			continue
		}
		seen[r] = true
		eval := testEval
		tc, err := valueCase(slo, n, r, total, eval, fmt.Sprintf("%s fires above the %s/%s threshold (%sx burn rate)",
			slo.ID, window(p.long), window(p.short), ir.FormatFloat(p.burnRate)))
		if err != nil {
			return nil, err
		}
		if tc.AlertTests, err = alertTests(slo, r, eval); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}

	// Quiet: just below the lowest threshold.
	r := ir.Round(pairs[0].threshold * 0.9)
	quiet, err := valueCase(slo, n, r, total, testEval, slo.ID+" stays quiet below the lowest burn-rate threshold")
	if err != nil {
		return nil, err
	}
	quiet.ExprTests = nil
	if quiet.AlertTests, err = alertTests(slo, r, testEval); err != nil {
		return nil, err
	}
	out = append(out, quiet)

	// No traffic: no SLI series at all.
	idle := testCase{
		Name:        slo.ID + " does not alert without traffic",
		Interval:    window(testInterval),
		InputSeries: []inputSerie{{Series: "sloe_test_unrelated_total", Values: "0+1x20"}},
	}
	for _, name := range sortedSet(names) {
		idle.AlertTests = append(idle.AlertTests, alertTest{EvalTime: "10m", AlertName: name, ExpAlerts: []expAlert{}})
	}
	out = append(out, idle)
	return out, nil
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// alertTests lists, per alert name, the alerts expected to fire for a
// constant error ratio r: a severity fires if any of its pairs' thresholds is below r.
func alertTests(slo *ir.SLO, r float64, eval time.Duration) ([]alertTest, error) {
	byName := map[string][]expAlert{}
	var order []string
	for _, a := range slo.Alerts {
		if _, ok := byName[a.Name]; !ok {
			order = append(order, a.Name)
			byName[a.Name] = []expAlert{}
		}
		fires := false
		for _, w := range a.Windows {
			if r > w.Threshold(slo.Budget) {
				fires = true
			}
		}
		if !fires {
			continue
		}
		labels := cloneWith(slo.SeriesLabels(), a.Labels)
		ann, err := expandAnnotations(a.Annotations, cloneWith(labels, map[string]string{"alertname": a.Name}))
		if err != nil {
			return nil, fmt.Errorf("alert %s: %w", a.Name, err)
		}
		byName[a.Name] = append(byName[a.Name], expAlert{ExpLabels: labels, ExpAnnotations: ann})
	}
	slices.Sort(order)
	var out []alertTest
	for _, name := range order {
		out = append(out, alertTest{EvalTime: window(eval), AlertName: name, ExpAlerts: byName[name]})
	}
	return out, nil
}

var labelsTemplate = regexp.MustCompile(`\{\{-?\s*\$labels\.([a-zA-Z_][a-zA-Z0-9_]*)\s*-?\}\}`)

// expandAnnotations expands {{ $labels.<name> }} the way Prometheus does.
// Validation rejects every other template.
func expandAnnotations(m, labels map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(m))
	for k, v := range m {
		var missing string
		out[k] = labelsTemplate.ReplaceAllStringFunc(v, func(act string) string {
			name := labelsTemplate.FindStringSubmatch(act)[1]
			val, ok := labels[name]
			if !ok {
				missing = name
			}
			return val
		})
		if missing != "" {
			return nil, fmt.Errorf("annotation %q references label %q, which the alert does not have", k, missing)
		}
	}
	return out, nil
}

// valueCase builds input series with constant error ratio r and asserts the
// 5m error-ratio recording rule.
func valueCase(slo *ir.SLO, n semconv.Naming, r, total float64, eval time.Duration, name string) (testCase, error) {
	series, err := inputSeries(slo, n, r, total)
	if err != nil {
		return testCase{}, err
	}
	steps := int(eval/testInterval) + 1
	tc := testCase{Name: name, Interval: window(testInterval)}
	for _, s := range series {
		tc.InputSeries = append(tc.InputSeries, inputSerie{
			Series: s.render(n),
			Values: fmt.Sprintf("0+%sx%d", ir.FormatFloat(s.inc), steps),
		})
	}
	// Compare within a tolerance: Prometheus' rate arithmetic differs from
	// r in the last bits, beyond what promtool's fuzzy_compare accepts.
	rec := RecordErrorRatio + "5m"
	tc.ExprTests = []exprTest{{
		Expr:     fmt.Sprintf("abs(%s - %s) < bool %s", idSelector(rec, slo), ir.FormatFloat(r), valueTolerance),
		EvalTime: window(eval),
		ExpSamples: []expSample{{
			Labels: labelSet(slo.SeriesLabels()),
			Value:  1,
		}},
	}}
	return tc, nil
}

func labelSet(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, labelName(k)+"="+strconv.Quote(m[k]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// synth is one synthetic counter series: increment per interval.
type synth struct {
	name   string            // Prometheus series name
	labels map[string]string // spec-form label names
	inc    float64
}

func (s synth) render(n semconv.Naming) string {
	ms := make([]string, 0, len(s.labels))
	for _, k := range sortedKeys(s.labels) {
		ms = append(ms, labelName(semconv.SpecLabel(k, n))+"="+strconv.Quote(s.labels[k]))
	}
	return selector(s.name, ms)
}

func eqLabels(ms []ir.Matcher) map[string]string {
	out := map[string]string{}
	for _, m := range ms {
		if m.Op == ir.Eq {
			out[m.Label] = m.Value
		}
	}
	return out
}

func set(m map[string]string, kv ...string) map[string]string {
	out := cloneWith(m, nil)
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func leValue(ms []ir.Matcher) (string, bool) {
	for _, m := range ms {
		if m.Label == "le" {
			v := m.Value
			if i := strings.IndexByte(v, '|'); i >= 0 {
				v = v[:i] // integer boundary: emit the OTLP form "1"
			}
			return v, true
		}
	}
	return "", false
}

var (
	errorType  = semconv.PromLabel(semconv.AttrErrorType)
	scoreLabel = semconv.PromLabel(semconv.AttrEvaluationScoreLabel)
	toolName   = semconv.PromLabel(semconv.AttrToolName)
)

// inputSeries synthesizes source series for the SLO's SLI with error ratio
// r and total events per interval, plus a distractor series that the
// selector must exclude (it is all bad, so a broken selector changes the ratio).
func inputSeries(slo *ir.SLO, n semconv.Naming, r, total float64) ([]synth, error) {
	sli := slo.SLI
	base := eqLabels(sli.Matchers)
	bad, good := ir.Round(total*r), ir.Round(total*(1-r))
	var out []synth
	totalTerm := sli.Total[0].Series
	m := totalTerm.Metric
	lbl := eqLabels(totalTerm.Matchers) // includes kind-specific equality matchers

	switch sli.Kind {
	case "latency", "cost":
		le, ok := leValue(sli.Bad[1].Series.Matchers)
		if !ok {
			return nil, fmt.Errorf("no le matcher")
		}
		out = append(out,
			synth{m.Count(n), lbl, total},
			synth{m.Bucket(n), set(lbl, "le", le), good},
			synth{m.Bucket(n), set(lbl, "le", "+Inf"), total})
		if m.Carries(errorType) {
			// Failed requests are not part of the latency SLI.
			failed := set(lbl, errorType, "sloe_test_error")
			out = append(out,
				synth{m.Count(n), failed, total},
				synth{m.Bucket(n), set(failed, "le", le), 0},
				synth{m.Bucket(n), set(failed, "le", "+Inf"), total})
		}
	case "errorRatio", "toolSuccess":
		if sli.Kind == "toolSuccess" {
			if _, ok := lbl[toolName]; !ok {
				lbl = set(lbl, toolName, firstAlt(totalTerm.Matchers, toolName, "sloe_test_tool"))
			}
		}
		out = append(out,
			synth{m.Count(n), lbl, good},
			synth{m.Count(n), set(lbl, errorType, "sloe_test_error"), bad})
		if ex := excluded(totalTerm.Matchers); ex != "" {
			out = append(out, synth{m.Count(n), set(lbl, errorType, ex), total})
		}
		if sli.Kind == "toolSuccess" && hasRegex(totalTerm.Matchers, toolName) {
			out = append(out, synth{m.Count(n), set(lbl, toolName, "sloe_test_other_tool", errorType, "sloe_test_error"), total})
		}
	case "evalScore":
		if le, ok := leValue(sli.Bad[0].Series.Matchers); ok {
			out = append(out,
				synth{m.Count(n), lbl, total},
				synth{m.Bucket(n), set(lbl, "le", le), bad},
				synth{m.Bucket(n), set(lbl, "le", "+Inf"), total})
		} else {
			pass := firstAlt(sli.Bad[0].Series.Matchers, scoreLabel, "")
			if pass == "" {
				return nil, fmt.Errorf("no pass label")
			}
			out = append(out,
				synth{m.Base(n), set(lbl, scoreLabel, pass), good},
				synth{m.Base(n), set(lbl, scoreLabel, "sloe_test_fail"), bad})
		}
	case "evalCoverage":
		covered := sli.Bad[1].Series
		out = append(out,
			synth{m.Count(n), lbl, total},
			synth{covered.Metric.Base(n), eqLabels(covered.Matchers), good})
	case "judgeAgreement":
		out = append(out,
			synth{m.Base(n), set(lbl, semconv.LabelOutcome, "agree"), good},
			synth{m.Base(n), set(lbl, semconv.LabelOutcome, "disagree"), bad})
	default:
		return nil, fmt.Errorf("unsupported SLI kind %q", sli.Kind)
	}

	// Distractor: same series with the first selector label changed, all bad.
	if len(base) > 0 {
		key := sortedKeys(base)[0]
		var extra []synth
		for _, s := range out {
			d := synth{s.name, set(s.labels, key, "sloe_test_other"), s.inc}
			extra = append(extra, d)
		}
		out = append(out, extra...)
	}
	return out, nil
}

// firstAlt returns the first alternative of a =~ or !~ matcher on label.
func firstAlt(ms []ir.Matcher, label, def string) string {
	for _, m := range ms {
		if m.Label == label && (m.Op == ir.Re || m.Op == ir.Nre) {
			v := strings.SplitN(m.Value, "|", 2)[0]
			return strings.ReplaceAll(v, `\`, "")
		}
	}
	return def
}

func hasRegex(ms []ir.Matcher, label string) bool {
	for _, m := range ms {
		if m.Label == label && m.Op == ir.Re {
			return true
		}
	}
	return false
}

// excluded returns one excluded error type, if errorRatio excludes any.
func excluded(ms []ir.Matcher) string {
	for _, m := range ms {
		if m.Label == errorType && m.Op == ir.Nre {
			return strings.ReplaceAll(strings.SplitN(m.Value, "|", 2)[0], `\`, "")
		}
	}
	return ""
}
