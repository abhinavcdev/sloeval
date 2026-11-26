// Package labels holds the metric label allowlist. It is the only place that
// decides which label keys a spec may use in static labels, selectors and
// alert routing labels.
//
// Labels become Prometheus series dimensions, so every allowed key must have
// bounded cardinality. Prompts, completions, user or session IDs and any
// other free text must never be labels.
package labels

import (
	"fmt"
	"slices"
	"strings"

	"github.com/abhinavcdev/sloeval/internal/semconv"
	"github.com/abhinavcdev/sloeval/internal/suggest"
)

// allowed is the label allowlist. Adding a key needs a cardinality
// justification in review.
var allowed = []string{
	// Ownership and routing (static labels, set by the spec author).
	"team",
	"tier",
	"env",
	"severity",
	// Resource attributes, as promoted by OTLP ingestion.
	semconv.PromLabel(semconv.AttrServiceName),
	semconv.PromLabel(semconv.AttrDeploymentEnvironment),
	// GenAI attributes with bounded cardinality.
	semconv.PromLabel(semconv.AttrOperationName),
	semconv.PromLabel(semconv.AttrProviderName),
	semconv.PromLabel(semconv.AttrRequestModel),
	semconv.PromLabel(semconv.AttrResponseModel),
	semconv.PromLabel(semconv.AttrAgentName),
	semconv.PromLabel(semconv.AttrToolName),
	semconv.PromLabel(semconv.AttrToolType),
	semconv.PromLabel(semconv.AttrEvaluationName),
	semconv.PromLabel(semconv.AttrEvaluationScoreLabel),
	semconv.PromLabel(semconv.AttrErrorType),
}

// Allowed returns a sorted copy of the allowlist.
func Allowed() []string {
	out := slices.Clone(allowed)
	slices.Sort(out)
	return out
}

// IsAllowed reports whether key is on the allowlist.
func IsAllowed(key string) bool { return slices.Contains(allowed, key) }

// reserved keys are set by sloe or Prometheus and may not be set by users.
var reservedExact = []string{"alertname", "le", "quantile"}

var reservedPrefixes = []string{"__", "sloe_"}

// unbounded lists substrings that identify keys we know carry unbounded or
// sensitive values, with the reason shown to the user.
var unbounded = []struct{ substr, why string }{
	{"prompt", "prompts are free text"},
	{"completion", "completions are free text"},
	{"content", "message content is free text"},
	{"message", "messages are free text"},
	{"explanation", "evaluation explanations are free text"},
	{"query", "queries are free text"},
	{"input", "inputs are free text"},
	{"output", "outputs are free text"},
	{"user", "user identifiers are unbounded and may be personal data"},
	{"email", "email addresses are unbounded personal data"},
	{"session", "session identifiers are unbounded"},
	{"conversation", "conversation identifiers are unbounded"},
	{"trace", "trace identifiers are unbounded"},
	{"span", "span identifiers are unbounded"},
	{"request_id", "request identifiers are unbounded"},
	{"response_id", "response identifiers are unbounded"},
}

// Problem explains why a key is not allowed.
type Problem struct {
	Message string
	Hint    string
}

// Check returns nil if key may be used as a label key, or a Problem saying why not.
func Check(key string) *Problem {
	if slices.Contains(reservedExact, key) {
		return &Problem{
			Message: fmt.Sprintf("label %q is reserved", key),
			Hint:    "remove it; sloe and Prometheus set this label themselves",
		}
	}
	for _, p := range reservedPrefixes {
		if strings.HasPrefix(key, p) {
			return &Problem{
				Message: fmt.Sprintf("label %q uses the reserved prefix %q", key, p),
				Hint:    "rename the label; keys starting with " + p + " are set by sloe or Prometheus",
			}
		}
	}
	if IsAllowed(key) {
		return nil
	}
	lower := strings.ToLower(key)
	for _, u := range unbounded {
		if strings.Contains(lower, u.substr) {
			return &Problem{
				Message: fmt.Sprintf("label %q is not allowed: %s", key, u.why),
				Hint:    "metric labels must have bounded cardinality and must never carry prompts, user IDs or free text; record this on spans or logs instead",
			}
		}
	}
	if strings.HasSuffix(lower, "_id") || lower == "id" {
		return &Problem{
			Message: fmt.Sprintf("label %q is not allowed: identifiers are unbounded", key),
			Hint:    "metric labels must have bounded cardinality; record identifiers on spans or logs instead",
		}
	}
	hint := "allowed keys: " + strings.Join(Allowed(), ", ")
	if s := suggest.Closest(key, allowed); s != "" {
		hint = fmt.Sprintf("did you mean %q? %s", s, hint)
	}
	return &Problem{
		Message: fmt.Sprintf("label %q is not on the allowlist", key),
		Hint:    hint,
	}
}
