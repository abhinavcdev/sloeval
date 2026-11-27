package sli

import (
	"testing"

	"github.com/abhinavcdev/sloeval/internal/semconv"
	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
)

func TestLatencyMetric(t *testing.T) {
	tests := []struct {
		stage  v1alpha1.LatencyStage
		target v1alpha1.Target
		source v1alpha1.LatencySource
		want   string
		ok     bool
	}{
		{v1alpha1.StageTTFT, "", "", semconv.ClientTimeToFirstChunk.Name, true},
		{v1alpha1.StageTTFT, "", v1alpha1.SourceServer, semconv.ServerTimeToFirstToken.Name, true},
		{v1alpha1.StageEndToEnd, "", "", semconv.ClientInferenceDuration.Name, true},
		{v1alpha1.StageEndToEnd, v1alpha1.TargetInference, v1alpha1.SourceServer, semconv.ServerRequestDuration.Name, true},
		{v1alpha1.StageEndToEnd, v1alpha1.TargetAgent, "", semconv.InvokeAgentDuration.Name, true},
		{v1alpha1.StageEndToEnd, v1alpha1.TargetAgent, v1alpha1.SourceClient, semconv.ClientInvokeAgentDuration.Name, true},
		{v1alpha1.StageTTFT, v1alpha1.TargetAgent, "", "", false},
	}
	for _, tt := range tests {
		m, ok := LatencyMetric(&v1alpha1.LatencySLI{Stage: tt.stage, Target: tt.target, Source: tt.source})
		if ok != tt.ok || m.Name != tt.want {
			t.Errorf("%s/%s/%s = %q,%v want %q,%v", tt.stage, tt.target, tt.source, m.Name, ok, tt.want, tt.ok)
		}
	}
}
