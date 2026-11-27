// Package sli resolves which semconv metrics an SLI reads. Validation and
// the IR both use it, so they can never disagree.
package sli

import (
	"github.com/abhinavcdev/sloeval/internal/semconv"
	"github.com/abhinavcdev/sloeval/pkg/api/v1alpha1"
)

// LatencyMetric returns the histogram a latency SLI reads, or false for an
// unsupported combination (ttft with target agent).
func LatencyMetric(l *v1alpha1.LatencySLI) (semconv.Metric, bool) {
	target := v1alpha1.TargetOrDefault(l.Target)
	source := l.Source
	if source == "" {
		source = v1alpha1.DefaultLatencySource(target)
	}
	server := source == v1alpha1.SourceServer
	switch {
	case target == v1alpha1.TargetAgent && l.Stage == v1alpha1.StageTTFT:
		return semconv.Metric{}, false
	case target == v1alpha1.TargetAgent && server:
		return semconv.InvokeAgentDuration, true
	case target == v1alpha1.TargetAgent:
		return semconv.ClientInvokeAgentDuration, true
	case l.Stage == v1alpha1.StageTTFT && server:
		return semconv.ServerTimeToFirstToken, true
	case l.Stage == v1alpha1.StageTTFT:
		return semconv.ClientTimeToFirstChunk, true
	case server:
		return semconv.ServerRequestDuration, true
	default:
		return semconv.ClientInferenceDuration, true
	}
}

// OperationMetric returns the histogram whose _count counts the operations
// of a target (its end-to-end duration metric).
func OperationMetric(t v1alpha1.Target) semconv.Metric {
	if v1alpha1.TargetOrDefault(t) == v1alpha1.TargetAgent {
		return semconv.InvokeAgentDuration
	}
	return semconv.ClientInferenceDuration
}

// Metrics returns every metric the SLI reads (for selector checks).
func Metrics(s *v1alpha1.SLI) []semconv.Metric {
	switch {
	case s.Latency != nil:
		if m, ok := LatencyMetric(s.Latency); ok {
			return []semconv.Metric{m}
		}
		return nil
	case s.ErrorRatio != nil:
		return []semconv.Metric{OperationMetric(s.ErrorRatio.Target)}
	case s.Cost != nil:
		if s.Cost.Per == v1alpha1.CostPerTask {
			return []semconv.Metric{semconv.CostPerTask}
		}
		return []semconv.Metric{semconv.CostPerRequest}
	case s.EvalScore != nil:
		if s.EvalScore.MinScore != nil {
			return []semconv.Metric{semconv.EvaluationScore}
		}
		return []semconv.Metric{semconv.EvaluationResults}
	case s.EvalCoverage != nil:
		return []semconv.Metric{semconv.EvaluatedOperations, OperationMetric(s.EvalCoverage.Target)}
	case s.ToolSuccess != nil:
		return []semconv.Metric{semconv.ExecuteToolDuration}
	case s.JudgeAgreement != nil:
		return []semconv.Metric{semconv.JudgeAgreement}
	}
	return nil
}
