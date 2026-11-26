package v1alpha1

import "time"

// DefaultWindow is the SLO window used when an SLO does not set one.
const DefaultWindow Duration = "30d"

// DefaultLatencySource is used when latency.source is not set.
const DefaultLatencySource = SourceClient

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
