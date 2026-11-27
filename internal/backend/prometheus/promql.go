// Package prometheus renders IR into Prometheus rule files (recording
// rules, multi-window multi-burn-rate alerts) and promtool unit tests.
package prometheus

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abhinavcdev/sloeval/internal/ir"
	"github.com/abhinavcdev/sloeval/internal/semconv"
	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
)

// Options control rendering.
type Options struct {
	Naming semconv.Naming
}

func (o Options) naming() semconv.Naming {
	if o.Naming == "" {
		return semconv.NamingUnderscore
	}
	return o.Naming
}

var legacyName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
var legacyLabel = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func labelName(l string) string {
	if legacyLabel.MatchString(l) {
		return l
	}
	return strconv.Quote(l)
}

// selector renders name{matchers}, or {"name", matchers} for UTF-8 names.
func selector(name string, ms []string) string {
	if legacyName.MatchString(name) {
		return name + "{" + strings.Join(ms, ", ") + "}"
	}
	return "{" + strings.Join(append([]string{strconv.Quote(name)}, ms...), ", ") + "}"
}

func matchers(ms []ir.Matcher, n semconv.Naming) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = labelName(semconv.SpecLabel(m.Label, n)) + string(m.Op) + strconv.Quote(m.Value)
	}
	return out
}

func seriesSelector(s ir.Series, n semconv.Naming) string {
	return selector(s.Name(n), matchers(s.Matchers, n))
}

func window(d time.Duration) string { return v1alpha1.FormatDuration(d) }

func termExpr(t ir.Term, w time.Duration, n semconv.Naming) string {
	e := "sum(rate(" + seriesSelector(t.Series, n) + "[" + window(w) + "]))"
	if t.ZeroIfAbsent {
		e = "(" + e + " or vector(0))"
	}
	return e
}

func sumExpr(ts []ir.Term, w time.Duration, n semconv.Naming) string {
	var sb strings.Builder
	for i, t := range ts {
		switch {
		case i == 0 && t.Negative:
			sb.WriteString("-")
		case i > 0 && t.Negative:
			sb.WriteString(" - ")
		case i > 0:
			sb.WriteString(" + ")
		}
		sb.WriteString(termExpr(t, w, n))
	}
	return sb.String()
}

// ErrorRatioExpr renders the SLI error ratio over window w. Zero traffic
// yields no sample (not NaN), so rollups and alerts ignore idle periods.
func ErrorRatioExpr(s ir.SLI, w time.Duration, n semconv.Naming) string {
	bad := sumExpr(s.Bad, w, n)
	if s.ClampBad {
		bad = "clamp_min(" + bad + ", 0)"
	}
	return "(" + bad + ")\n/\n(" + sumExpr(s.Total, w, n) + " > 0)"
}

// Recording rule names.
const (
	RecordErrorRatio      = "sloe:sli_error:ratio_rate"
	RecordObjective       = "sloe:objective:ratio"
	RecordBudget          = "sloe:error_budget:ratio"
	RecordBudgetRemaining = "sloe:error_budget_remaining:ratio"
	RecordBurnRate        = "sloe:burn_rate:ratio_rate"
	RecordInfo            = "sloe:slo_info"
)

func idSelector(record string, slo *ir.SLO) string {
	return record + "{" + ir.LabelID + "=" + strconv.Quote(slo.ID) + "}"
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
