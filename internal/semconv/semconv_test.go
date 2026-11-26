package semconv

import (
	"sort"
	"testing"
)

func TestPromLabel(t *testing.T) {
	tests := map[string]string{
		AttrRequestModel:          "gen_ai_request_model",
		AttrErrorType:             "error_type",
		AttrDeploymentEnvironment: "deployment_environment_name",
		"already_ok":              "already_ok",
	}
	for in, want := range tests {
		if got := PromLabel(in); got != want {
			t.Errorf("PromLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMetricsAreWellFormed(t *testing.T) {
	all := []Metric{
		ClientInferenceDuration, ClientTimeToFirstChunk, ServerRequestDuration, ServerTimeToFirstToken,
		ExecuteToolDuration, CostPerRequest, CostPerTask, EvaluationScore, EvaluationResults,
		EvaluatedOperations, JudgeAgreement,
	}
	seen := map[string]bool{}
	for _, m := range all {
		if m.Name == "" || m.PromName == "" {
			t.Errorf("metric %+v has empty name", m)
		}
		if seen[m.PromName] {
			t.Errorf("duplicate PromName %q", m.PromName)
		}
		seen[m.PromName] = true
		if m.Kind == Histogram && !sort.Float64sAreSorted(m.Buckets) {
			t.Errorf("%s: buckets not sorted", m.Name)
		}
		if m.Kind == Histogram && len(m.Buckets) == 0 {
			t.Errorf("%s: histogram without buckets", m.Name)
		}
		if m.Owner != "semconv" && m.Owner != "sloe" {
			t.Errorf("%s: bad owner %q", m.Name, m.Owner)
		}
	}
}
