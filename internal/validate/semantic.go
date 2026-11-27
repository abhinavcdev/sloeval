package validate

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abhinavcdev/sloeval/internal/diag"
	"github.com/abhinavcdev/sloeval/internal/labels"
	"github.com/abhinavcdev/sloeval/internal/semconv"
	"github.com/abhinavcdev/sloeval/internal/sli"
	"github.com/abhinavcdev/sloeval/internal/spec"
	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
)

const (
	minWindow = 24 * time.Hour
	maxWindow = 365 * 24 * time.Hour
	day       = 24 * time.Hour
)

// semanticChecker runs checks the JSON Schema cannot express.
type semanticChecker struct {
	doc   *spec.Document
	diags []diag.Diagnostic
	seen  map[string]bool // dedupe key: path + message

	specSelector *v1alpha1.Selector
}

func checkSemantics(doc *spec.Document, obj *v1alpha1.AISLO) []diag.Diagnostic {
	c := &semanticChecker{doc: doc, seen: map[string]bool{}}
	c.run(obj)
	return c.diags
}

func (c *semanticChecker) report(sev diag.Severity, p spec.Path, pos spec.Pos, msg, hint string) {
	key := p.String() + "\x00" + msg
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	c.diags = append(c.diags, diag.Diagnostic{
		Severity: sev, File: c.doc.File, Document: c.doc.Index + 1,
		Path: p.String(), Line: pos.Line, Column: pos.Col, Message: msg, Hint: hint,
	})
}

func (c *semanticChecker) errorf(p spec.Path, hint, format string, args ...any) {
	c.report(diag.Error, p, c.doc.Pos(p), fmt.Sprintf(format, args...), hint)
}

func (c *semanticChecker) warnf(p spec.Path, hint, format string, args ...any) {
	c.report(diag.Warning, p, c.doc.Pos(p), fmt.Sprintf(format, args...), hint)
}

func (c *semanticChecker) run(obj *v1alpha1.AISLO) {
	specP := spec.Path{}.Key("spec")
	s := &obj.Spec
	c.specSelector = s.Selector

	c.labelMap(specP.Key("labels"), s.Labels)
	c.selector(specP.Key("selector"), s.Selector)
	c.alertingLabels(specP.Key("alerting"), s.Alerting)

	names := map[string]int{}
	for i := range s.SLOs {
		sloP := specP.Key("slos").Index(i)
		slo := &s.SLOs[i]
		if first, dup := names[slo.Name]; dup && slo.Name != "" {
			c.errorf(sloP.Key("name"), fmt.Sprintf("rename one of them; the first is spec.slos[%d]", first),
				"duplicate SLO name %q", slo.Name)
		} else {
			names[slo.Name] = i
		}
		c.slo(sloP, slo, s.Alerting)
	}
}

func (c *semanticChecker) labelMap(p spec.Path, m map[string]string) {
	for _, k := range sortedKeys(m) {
		if prob := labels.Check(k); prob != nil {
			kp := p.Key(k)
			c.report(diag.Error, kp, c.doc.KeyPos(kp), prob.Message, prob.Hint)
		}
	}
}

func (c *semanticChecker) selector(p spec.Path, s *v1alpha1.Selector) {
	if s != nil {
		c.labelMap(p.Key("matchLabels"), s.MatchLabels)
	}
}

func (c *semanticChecker) alertingLabels(p spec.Path, a *v1alpha1.Alerting) {
	if a == nil {
		return
	}
	c.labelMap(p.Key("labels"), a.Labels)
	if a.Page != nil {
		c.labelMap(p.Key("page").Key("labels"), a.Page.Labels)
	}
	if a.Ticket != nil {
		c.labelMap(p.Key("ticket").Key("labels"), a.Ticket.Labels)
	}
}

func (c *semanticChecker) duration(p spec.Path, d v1alpha1.Duration) (time.Duration, bool) {
	v, err := d.Parse()
	if err != nil {
		c.errorf(p, `use a duration with units y, w, d, h, m, s, ms from largest to smallest, e.g. "1h30m" or "2.56s"`,
			"%s", err.Error())
		return 0, false
	}
	return v, true
}

func (c *semanticChecker) slo(p spec.Path, slo *v1alpha1.SLO, defaults *v1alpha1.Alerting) {
	c.labelMap(p.Key("labels"), slo.Labels)
	c.alertingLabels(p.Key("alerting"), slo.Alerting)

	// Objective.
	switch {
	case slo.Objective > 99.99 && slo.Objective < 100:
		c.warnf(p.Key("objective"), "at typical traffic the error budget is too small to alert on reliably; consider 99.9 or lower",
			"objective %v leaves an error budget of %.4f%%", slo.Objective, 100-slo.Objective)
	case slo.Objective > 0 && slo.Objective < 1:
		c.warnf(p.Key("objective"), fmt.Sprintf("did you mean %v?", trimFloat(slo.Objective*100)),
			"objective %v is a percentage, so this means %v%%", slo.Objective, slo.Objective)
	}

	// Window.
	windowP := p.Key("window")
	windowStr := slo.Window
	if windowStr == "" {
		windowStr = v1alpha1.DefaultWindow
	}
	window, ok := c.duration(windowP, windowStr)
	if ok {
		switch {
		case window < minWindow || window > maxWindow:
			c.errorf(windowP, "use 28d or 30d", "window %s must be between 1d and 365d", windowStr)
			ok = false
		case window != 28*day && window != 30*day:
			c.warnf(windowP, "28d and 30d are the usual choices; default burn rates are scaled to this window",
				"window %s is unusual", windowStr)
		}
	}

	c.sli(p.Key("sli"), &slo.SLI)

	if ok {
		c.alerting(p, slo, defaults, window)
	}
}

// alerting checks the effective burn-rate windows of one SLO.
func (c *semanticChecker) alerting(sloP spec.Path, slo *v1alpha1.SLO, defaults *v1alpha1.Alerting, window time.Duration) {
	if v1alpha1.AlertingDisabled(slo.Alerting, defaults) {
		return
	}
	type sev struct {
		name     string
		get      func(*v1alpha1.Alerting) *v1alpha1.AlertPolicy
		defaults []v1alpha1.BudgetWindow
	}
	sevs := []sev{
		{"page", func(a *v1alpha1.Alerting) *v1alpha1.AlertPolicy { return a.Page }, v1alpha1.DefaultPageWindows},
		{"ticket", func(a *v1alpha1.Alerting) *v1alpha1.AlertPolicy { return a.Ticket }, v1alpha1.DefaultTicketWindows},
	}
	for _, s := range sevs {
		var sloPol, defPol *v1alpha1.AlertPolicy
		if slo.Alerting != nil {
			sloPol = s.get(slo.Alerting)
		}
		if defaults != nil {
			defPol = s.get(defaults)
		}
		if v1alpha1.PolicyDisabled(sloPol, defPol) {
			continue
		}
		switch {
		case sloPol != nil && len(sloPol.Windows) > 0:
			c.windows(sloP.Key("alerting").Key(s.name).Key("windows"), sloPol.Windows, window, slo.Objective, "")
		case defPol != nil && len(defPol.Windows) > 0:
			c.windows(spec.Path{}.Key("spec").Key("alerting").Key(s.name).Key("windows"), defPol.Windows, window, slo.Objective,
				sloP.Key("name").String())
		default:
			for _, w := range s.defaults {
				if w.Long >= window {
					c.errorf(sloP.Key("window"),
						fmt.Sprintf("use a longer window, or set alerting.%s.windows explicitly", s.name),
						"window %s is not longer than the default %s long window %s",
						v1alpha1.FormatDuration(window), s.name, v1alpha1.FormatDuration(w.Long))
					break
				}
			}
		}
	}
}

func (c *semanticChecker) windows(p spec.Path, ws []v1alpha1.BurnRateWindow, window time.Duration, objective float64, forSLO string) {
	ctx := ""
	if forSLO != "" {
		ctx = " (for " + forSLO + ")"
	}
	for i, w := range ws {
		wp := p.Index(i)
		long, okL := c.duration(wp.Key("long"), w.Long)
		short, okS := c.duration(wp.Key("short"), w.Short)
		if !okL || !okS {
			continue
		}
		if short >= long {
			c.errorf(wp.Key("short"), "the short window is usually 1/12 of the long window, e.g. long: 1h, short: 5m",
				"short window %s must be shorter than long window %s", w.Short, w.Long)
		}
		if long > window {
			c.errorf(wp.Key("long"), "use a long window no longer than the SLO window",
				"long window %s is longer than the SLO window %s%s", w.Long, v1alpha1.FormatDuration(window), ctx)
			continue
		}
		budget := 1 - objective/100
		if budget > 0 && w.BurnRate*budget >= 1 {
			c.errorf(wp.Key("burnRate"),
				fmt.Sprintf("use a burn rate below %s for a %v%% objective", trimFloat(round6(1/budget)), objective),
				"burnRate %v can never fire%s: it needs an error ratio of %s, and the error ratio is at most 1",
				w.BurnRate, ctx, trimFloat(round6(w.BurnRate*budget)))
			continue
		}
		if maxRate := float64(window) / float64(long); w.BurnRate > maxRate {
			c.warnf(wp.Key("burnRate"),
				fmt.Sprintf("use a burn rate of at most %s, or a shorter long window", trimFloat(maxRate)),
				"burnRate %v only fires after more than the whole error budget of a %s window is spent within %s%s",
				w.BurnRate, v1alpha1.FormatDuration(window), w.Long, ctx)
		}
	}
}

func (c *semanticChecker) sli(p spec.Path, s *v1alpha1.SLI) {
	k := s.Kind()
	if k == "" {
		return // reported by the schema
	}
	kp := p.Key(k)
	c.selector(kp.Key("selector"), s.Selector())
	if s.Latency == nil {
		c.selectorCarried(kp.Key("selector"), s)
	}

	switch {
	case s.Latency != nil:
		l := s.Latency
		m, ok := sli.LatencyMetric(l)
		if !ok {
			c.errorf(kp.Key("target"), "use stage: end_to_end for agents, or target: inference for TTFT",
				"stage ttft is not available for target agent: semconv has no agent time-to-first-token metric")
			return
		}
		c.selectorCarried(kp.Key("selector"), s)
		thr, ok := c.duration(kp.Key("threshold"), l.Threshold)
		if !ok {
			return
		}
		c.bucket(kp.Key("threshold"), thr.Seconds(), m, func(f float64) string { return trimFloat(f) + "s" })
	case s.Cost != nil:
		m := semconv.CostPerRequest
		if s.Cost.Per == v1alpha1.CostPerTask {
			m = semconv.CostPerTask
		}
		c.bucket(kp.Key("thresholdUSD"), s.Cost.ThresholdUSD, m, trimFloat)
	case s.EvalScore != nil:
		if ms := s.EvalScore.MinScore; ms != nil {
			if lowest := semconv.EvaluationScore.Buckets[0]; *ms <= lowest {
				c.errorf(kp.Key("minScore"), fmt.Sprintf("use a minScore above %s, or passLabels for label-based evaluators", trimFloat(lowest)),
					"minScore %v must be above the lowest score bucket boundary %s: failures are counted in the bucket below minScore",
					*ms, trimFloat(lowest))
			} else {
				c.bucket(kp.Key("minScore"), *ms, semconv.EvaluationScore, trimFloat)
			}
		}
		c.unique(kp.Key("passLabels"), s.EvalScore.PassLabels)
	case s.ToolSuccess != nil:
		c.unique(kp.Key("tools"), s.ToolSuccess.Tools)
	case s.ErrorRatio != nil:
		c.unique(kp.Key("excludeErrorTypes"), s.ErrorRatio.ExcludeErrorTypes)
	case s.JudgeAgreement != nil:
		if j := s.JudgeAgreement; j.Judge != "" && j.Judge == j.Reference {
			c.errorf(kp.Key("reference"), "set reference to the evaluator that produces ground-truth labels, e.g. human_review",
				"judge and reference are both %q; a judge always agrees with itself", j.Judge)
		}
	}
}

// selectorCarried warns about selector keys (spec-level and SLI-level)
// that a metric the SLI reads does not carry: the SLI would match nothing.
func (c *semanticChecker) selectorCarried(sliSelP spec.Path, s *v1alpha1.SLI) {
	check := func(p spec.Path, sel *v1alpha1.Selector) {
		if sel == nil {
			return
		}
		for _, k := range sortedKeys(sel.MatchLabels) {
			if labels.Check(k) != nil {
				continue // already an error
			}
			for _, m := range sli.Metrics(s) {
				if m.Carries(k) {
					continue
				}
				hint := fmt.Sprintf("remove %q from this selector; %s carries: %s", k, m.Name, strings.Join(carried(m), ", "))
				if k == semconv.PromLabel(semconv.AttrAgentName) {
					hint = "set target: agent to measure agent invocations, which carry gen_ai.agent.name"
				}
				kp := p.Key("matchLabels").Key(k)
				c.report(diag.Warning, kp, c.doc.KeyPos(kp),
					fmt.Sprintf("label %q is not an attribute of %s, so this SLI would match no series", k, m.Name), hint)
			}
		}
	}
	check(spec.Path{}.Key("spec").Key("selector"), c.specSelector)
	check(sliSelP, s.Selector())
}

func carried(m semconv.Metric) []string {
	var out []string
	for _, a := range append(append([]string{}, m.Attrs...), semconv.ResourceAttrs...) {
		if labels.IsAllowed(semconv.PromLabel(a)) || a == semconv.LabelJudge || a == semconv.LabelReference || a == semconv.LabelOutcome {
			out = append(out, semconv.PromLabel(a))
		}
	}
	slices.Sort(out)
	return out
}

// bucket warns when v is not a bucket boundary of m: a histogram "le"
// matcher only works at boundaries.
func (c *semanticChecker) bucket(p spec.Path, v float64, m semconv.Metric, format func(float64) string) {
	for _, b := range m.Buckets {
		if math.Abs(b-v) <= 1e-9*math.Max(1, math.Abs(b)) {
			return
		}
	}
	lo, hi := math.NaN(), math.NaN()
	for _, b := range m.Buckets {
		if b < v {
			lo = b
		} else if math.IsNaN(hi) {
			hi = b
		}
	}
	var near []string
	if !math.IsNaN(lo) {
		near = append(near, format(lo))
	}
	if !math.IsNaN(hi) {
		near = append(near, format(hi))
	}
	hint := fmt.Sprintf("use a bucket boundary (nearest: %s), or configure your instrumentation to emit a bucket at %s",
		strings.Join(near, " or "), format(v))
	c.warnf(p, hint, "%s is not a bucket boundary of %s, so the SLI would be measured at the next bucket",
		format(v), m.Name)
}

func (c *semanticChecker) unique(p spec.Path, items []string) {
	seen := map[string]bool{}
	for i, it := range items {
		if seen[it] {
			c.errorf(p.Index(i), "remove the duplicate", "duplicate value %q", it)
		}
		seen[it] = true
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// round6 rounds to 6 decimal places, hiding float noise in messages.
func round6(f float64) float64 { return math.Round(f*1e6) / 1e6 }

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
