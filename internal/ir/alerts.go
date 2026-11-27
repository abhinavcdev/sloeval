package ir

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
)

// Camel converts "support-agent" to "SupportAgent" and "ttft" to "Ttft".
func Camel(s string) string {
	var sb strings.Builder
	upper := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			upper = true
			continue
		}
		if upper {
			sb.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func policy(a *v1alpha1.Alerting, severity string) *v1alpha1.AlertPolicy {
	if a == nil {
		return nil
	}
	if severity == SeverityPage {
		return a.Page
	}
	return a.Ticket
}

func buildAlerts(spec *v1alpha1.AISLOSpec, in *v1alpha1.SLO, slo *SLO) ([]Alert, error) {
	if v1alpha1.AlertingDisabled(in.Alerting, spec.Alerting) {
		return nil, nil
	}
	name := Camel(spec.Service) + Camel(in.Name) + "ErrorBudgetBurn"
	switch {
	case in.Alerting != nil && in.Alerting.Name != "":
		name = in.Alerting.Name
	case spec.Alerting != nil && spec.Alerting.Name != "":
		name = spec.Alerting.Name + Camel(in.Name)
	}
	var routing, annotations map[string]string
	{
		var sl, sa, il, ia map[string]string
		if spec.Alerting != nil {
			sl, sa = spec.Alerting.Labels, spec.Alerting.Annotations
		}
		if in.Alerting != nil {
			il, ia = in.Alerting.Labels, in.Alerting.Annotations
		}
		routing, annotations = merge(sl, il), merge(sa, ia)
	}

	var out []Alert
	for _, sev := range []struct {
		name     string
		defaults []v1alpha1.BudgetWindow
	}{
		{SeverityPage, v1alpha1.DefaultPageWindows},
		{SeverityTicket, v1alpha1.DefaultTicketWindows},
	} {
		sloPol, specPol := policy(in.Alerting, sev.name), policy(spec.Alerting, sev.name)
		if v1alpha1.PolicyDisabled(sloPol, specPol) {
			continue
		}
		var custom []v1alpha1.BurnRateWindow
		switch {
		case sloPol != nil && len(sloPol.Windows) > 0:
			custom = sloPol.Windows
		case specPol != nil && len(specPol.Windows) > 0:
			custom = specPol.Windows
		}
		var windows []BurnWindow
		if len(custom) > 0 {
			for _, w := range custom {
				long, err := w.Long.Parse()
				if err != nil {
					return nil, err
				}
				short, err := w.Short.Parse()
				if err != nil {
					return nil, err
				}
				windows = append(windows, BurnWindow{Long: long, Short: short, BurnRate: Round(w.BurnRate)})
			}
		} else {
			for _, w := range sev.defaults {
				windows = append(windows, BurnWindow{Long: w.Long, Short: w.Short, BurnRate: Round(w.BurnRate(slo.Window))})
			}
		}
		var pl, il map[string]string
		if specPol != nil {
			pl = specPol.Labels
		}
		if sloPol != nil {
			il = sloPol.Labels
		}
		labels := merge(routing, pl, il)
		labels[LabelSeverity] = sev.name
		ann := merge(annotations)
		if _, ok := ann["summary"]; !ok {
			ann["summary"] = fmt.Sprintf("%s / %s is burning its error budget too fast (%s)", slo.Service, slo.Name, sev.name)
		}
		if _, ok := ann["description"]; !ok {
			ann["description"] = fmt.Sprintf("Objective %s%% over %s. Burn-rate windows (long/short at rate): %s.",
				FormatFloat(slo.Objective*100), v1alpha1.FormatDuration(slo.Window), describeWindows(windows))
		}
		out = append(out, Alert{Severity: sev.name, Name: name, Windows: windows, Labels: labels, Annotations: ann})
	}
	return out, nil
}

func describeWindows(ws []BurnWindow) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = fmt.Sprintf("%s/%s at %sx", fmtDur(w.Long), fmtDur(w.Short), FormatFloat(w.BurnRate))
	}
	return strings.Join(parts, ", ")
}

func fmtDur(d time.Duration) string { return v1alpha1.FormatDuration(d) }
