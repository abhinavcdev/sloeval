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

func TestNames(t *testing.T) {
	tests := []struct {
		m           Metric
		under, utf8 string
	}{
		{ClientInferenceDuration, "gen_ai_client_inference_duration_seconds", "gen_ai.client.inference.duration_seconds"},
		{ServerTimeToFirstToken, "gen_ai_server_time_to_first_token_seconds", "gen_ai.server.time_to_first_token_seconds"},
		{InvokeAgentDuration, "gen_ai_invoke_agent_duration_seconds", "gen_ai.invoke_agent.duration_seconds"},
		{CostPerRequest, "sloe_gen_ai_request_cost_usd", "sloe.gen_ai.request.cost.usd"},
		{EvaluationScore, "sloe_gen_ai_evaluation_score", "sloe.gen_ai.evaluation.score"},
		{EvaluationResults, "sloe_gen_ai_evaluation_results_total", "sloe.gen_ai.evaluation.results_total"},
		{JudgeAgreement, "sloe_judge_agreement_total", "sloe.judge.agreement_total"},
	}
	for _, tt := range tests {
		if got := tt.m.Base(NamingUnderscore); got != tt.under {
			t.Errorf("%s underscore = %q, want %q", tt.m.Name, got, tt.under)
		}
		if got := tt.m.Base(NamingUTF8); got != tt.utf8 {
			t.Errorf("%s utf8 = %q, want %q", tt.m.Name, got, tt.utf8)
		}
	}
	if got := ClientInferenceDuration.Bucket(NamingUnderscore); got != "gen_ai_client_inference_duration_seconds_bucket" {
		t.Errorf("bucket = %q", got)
	}
}

func TestLabels(t *testing.T) {
	if got := SpecLabel("gen_ai_request_model", NamingUTF8); got != "gen_ai.request.model" {
		t.Errorf("SpecLabel utf8 = %q", got)
	}
	if got := SpecLabel("team", NamingUTF8); got != "team" {
		t.Errorf("SpecLabel team = %q", got)
	}
	if !ClientInferenceDuration.Carries("gen_ai_request_model") || !ClientInferenceDuration.Carries("service_name") {
		t.Error("inference duration should carry request model and service_name")
	}
	if ClientInferenceDuration.Carries("gen_ai_agent_name") {
		t.Error("inference duration does not carry gen_ai.agent.name in v0.1.0")
	}
	if !JudgeAgreement.Carries("judge") {
		t.Error("judge agreement carries judge")
	}
}

func TestMetricsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range All {
		b := m.Base(NamingUnderscore)
		if seen[b] {
			t.Errorf("duplicate name %q", b)
		}
		seen[b] = true
		if m.Kind == Histogram && (len(m.Buckets) == 0 || !sort.Float64sAreSorted(m.Buckets)) {
			t.Errorf("%s: histogram buckets missing or unsorted", m.Name)
		}
		if m.Owner != "semconv" && m.Owner != "sloe" {
			t.Errorf("%s: bad owner %q", m.Name, m.Owner)
		}
		if len(m.Attrs) == 0 {
			t.Errorf("%s: no attributes", m.Name)
		}
	}
}
