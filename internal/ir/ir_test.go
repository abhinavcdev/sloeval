package ir

import (
	"reflect"
	"testing"
	"time"

	"github.com/abhinavcdev/sloeval/internal/spec"
)

// build parses YAML through the same decoding path as validation.
func build(t *testing.T, src string) *Spec {
	t.Helper()
	docs, diags := spec.Load("t.yaml", []byte(src))
	if len(diags) > 0 {
		t.Fatalf("load: %v", diags)
	}
	obj, err := decodeForTest(docs[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(obj)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const head = `apiVersion: sloeval.dev/v1alpha1
kind: AISLO
metadata: {name: demo}
spec:
  service: demo-svc
`

func TestObjectiveWindowAndIDs(t *testing.T) {
	s := build(t, `apiVersion: sloeval.dev/v1alpha1
kind: AISLO
metadata: {name: demo, namespace: team-a}
spec:
  service: demo-svc
  labels: {team: a, tier: "1"}
  slos:
    - name: errors
      objective: 99.5
      labels: {tier: "2"}
      sli: {errorRatio: {}}
`)
	if s.ID != "team-a.demo" {
		t.Errorf("spec ID = %q", s.ID)
	}
	slo := s.SLOs[0]
	if slo.ID != "team-a/demo/errors" || slo.Objective != 0.995 || slo.Budget != 0.005 || slo.Window != 30*24*time.Hour {
		t.Errorf("slo = %+v", slo)
	}
	if want := map[string]string{"team": "a", "tier": "2"}; !reflect.DeepEqual(slo.Labels, want) {
		t.Errorf("labels = %v, want %v", slo.Labels, want)
	}
	if got := slo.SeriesLabels()[LabelWindow]; got != "30d" {
		t.Errorf("sloe_window = %q", got)
	}
}

func TestDefaultWindowsScale(t *testing.T) {
	tests := []struct {
		window string
		want   []float64
	}{
		{"30d", []float64{14.4, 6, 3, 1}},
		{"28d", []float64{13.44, 5.6, 2.8, 0.9333333333}},
	}
	for _, tt := range tests {
		s := build(t, head+"  slos:\n    - name: e\n      objective: 99\n      window: "+tt.window+"\n      sli: {errorRatio: {}}\n")
		var got []float64
		for _, a := range s.SLOs[0].Alerts {
			for _, w := range a.Windows {
				got = append(got, w.BurnRate)
			}
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: burn rates = %v, want %v", tt.window, got, tt.want)
		}
		wantWindows := []time.Duration{5 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 6 * time.Hour, 24 * time.Hour, 72 * time.Hour}
		if !reflect.DeepEqual(s.SLOs[0].RateWindows, wantWindows) {
			t.Errorf("rate windows = %v", s.SLOs[0].RateWindows)
		}
	}
}

func TestAlertMerge(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		severities []string
		alertName  string
		labels     map[string]string
		windows    int
	}{
		{
			name:       "defaults",
			yaml:       head + "  slos:\n    - name: ttft-p\n      objective: 99\n      sli: {errorRatio: {}}\n",
			severities: []string{"page", "ticket"}, alertName: "DemoSvcTtftPErrorBudgetBurn",
			labels: map[string]string{"sloe_severity": "page"}, windows: 2,
		},
		{
			name:       "spec disabled, slo re-enabled",
			yaml:       head + "  alerting: {disabled: true}\n  slos:\n    - name: e\n      objective: 99\n      alerting: {disabled: false}\n      sli: {errorRatio: {}}\n",
			severities: []string{"page", "ticket"}, alertName: "DemoSvcEErrorBudgetBurn",
			labels: map[string]string{"sloe_severity": "page"}, windows: 2,
		},
		{
			name:       "spec disabled",
			yaml:       head + "  alerting: {disabled: true}\n  slos:\n    - name: e\n      objective: 99\n      sli: {errorRatio: {}}\n",
			severities: nil,
		},
		{
			name: "page disabled in spec, labels merged, slo wins",
			yaml: head + `  alerting:
    name: Demo
    labels: {team: a}
    page: {disabled: true}
    ticket: {labels: {severity: warning}}
  slos:
    - name: e
      objective: 99
      alerting:
        labels: {team: b}
        ticket:
          windows: [{long: 1d, short: 2h, burnRate: 3}]
      sli: {errorRatio: {}}
`,
			severities: []string{"ticket"}, alertName: "DemoE",
			labels: map[string]string{"sloe_severity": "ticket", "team": "b", "severity": "warning"}, windows: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alerts := build(t, tt.yaml).SLOs[0].Alerts
			var sev []string
			for _, a := range alerts {
				sev = append(sev, a.Severity)
			}
			if !reflect.DeepEqual(sev, tt.severities) {
				t.Fatalf("severities = %v, want %v", sev, tt.severities)
			}
			if len(alerts) == 0 {
				return
			}
			a := alerts[0]
			if a.Name != tt.alertName || !reflect.DeepEqual(a.Labels, tt.labels) || len(a.Windows) != tt.windows {
				t.Errorf("alert = %+v", a)
			}
			if a.Annotations["summary"] == "" || a.Annotations["description"] == "" {
				t.Errorf("missing default annotations: %v", a.Annotations)
			}
		})
	}
}

func TestLeMatcher(t *testing.T) {
	tests := []struct {
		in   float64
		want Matcher
	}{
		{0.64, Matcher{"le", Eq, "0.64"}},
		{2.56, Matcher{"le", Eq, "2.56"}},
		{1, Matcher{"le", Re, `1|1\.0`}},
		{10, Matcher{"le", Re, `10|10\.0`}},
	}
	for _, tt := range tests {
		if got := LeMatcher(tt.in); got != tt.want {
			t.Errorf("LeMatcher(%v) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestCamel(t *testing.T) {
	for in, want := range map[string]string{"support-agent": "SupportAgent", "ttft": "Ttft", "cost-per-task": "CostPerTask", "a1-b2": "A1B2"} {
		if got := Camel(in); got != want {
			t.Errorf("Camel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEvalScoreBucketBelow(t *testing.T) {
	s := build(t, head+"  slos:\n    - name: h\n      objective: 90\n      sli: {evalScore: {evaluator: helpfulness, minScore: 0.7}}\n")
	bad := s.SLOs[0].SLI.Bad[0].Series.Matchers
	found := false
	for _, m := range bad {
		if m.Label == "le" && m.Value == "0.6" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected le=0.6 (bucket below 0.7), got %v", bad)
	}
}
