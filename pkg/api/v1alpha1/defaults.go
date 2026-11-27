package v1alpha1

import "time"

// DefaultWindow is the SLO window used when an SLO does not set one.
const DefaultWindow Duration = "30d"

// DefaultTarget is used when an SLI's target is not set.
const DefaultTarget = TargetInference

// TargetOrDefault returns t, or DefaultTarget if t is empty.
func TargetOrDefault(t Target) Target {
	if t == "" {
		return DefaultTarget
	}
	return t
}

// DefaultLatencySource returns the source used when latency.source is not
// set: client for inference, server (the agent runtime itself) for agents.
func DefaultLatencySource(t Target) LatencySource {
	if TargetOrDefault(t) == TargetAgent {
		return SourceServer
	}
	return SourceClient
}

// Resolve returns the value of the nearest level that sets a tri-state
// flag (levels ordered nearest first), or false if none does.
func Resolve(levels ...*bool) bool {
	for _, l := range levels {
		if l != nil {
			return *l
		}
	}
	return false
}

// BudgetWindow is a default burn-rate window expressed as the share of the
// error budget consumed, so it scales to any SLO window:
// burnRate = BudgetConsumed * window / Long.
type BudgetWindow struct {
	Long, Short    time.Duration
	BudgetConsumed float64
}

// Default page and ticket windows from the Google SRE Workbook. For a 30d
// window these give burn rates 14.4 and 6 (page), 3 and 1 (ticket).
var (
	DefaultPageWindows = []BudgetWindow{
		{Long: time.Hour, Short: 5 * time.Minute, BudgetConsumed: 0.02},
		{Long: 6 * time.Hour, Short: 30 * time.Minute, BudgetConsumed: 0.05},
	}
	DefaultTicketWindows = []BudgetWindow{
		{Long: 24 * time.Hour, Short: 2 * time.Hour, BudgetConsumed: 0.10},
		{Long: 3 * 24 * time.Hour, Short: 6 * time.Hour, BudgetConsumed: 0.10},
	}
)

// BurnRate returns the burn rate for this window given the SLO window.
func (w BudgetWindow) BurnRate(window time.Duration) float64 {
	return w.BudgetConsumed * float64(window) / float64(w.Long)
}

// AlertingDisabled reports whether all alerts of an SLO are off, given the
// SLO's and the spec's alerting (either may be nil). The SLO level wins.
func AlertingDisabled(slo, spec *Alerting) bool {
	var a, b *bool
	if slo != nil {
		a = slo.Disabled
	}
	if spec != nil {
		b = spec.Disabled
	}
	return Resolve(a, b)
}

// PolicyDisabled reports whether one severity is off, given the SLO's and the
// spec's policy for it (either may be nil). The SLO level wins. It does not
// consider Alerting.Disabled; see AlertingDisabled.
func PolicyDisabled(slo, spec *AlertPolicy) bool {
	var a, b *bool
	if slo != nil {
		a = slo.Disabled
	}
	if spec != nil {
		b = spec.Disabled
	}
	return Resolve(a, b)
}
