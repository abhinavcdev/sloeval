package labels

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		key         string
		ok          bool
		msgContains string
		hintContain string
	}{
		{key: "team", ok: true},
		{key: "gen_ai_request_model", ok: true},
		{key: "error_type", ok: true},
		{key: "deployment_environment_name", ok: true},
		{key: "alertname", msgContains: "reserved"},
		{key: "le", msgContains: "reserved"},
		{key: "__name__", msgContains: "reserved prefix"},
		{key: "sloe_slo", msgContains: "reserved prefix"},
		{key: "user_id", msgContains: "user identifiers"},
		{key: "prompt", msgContains: "prompts are free text"},
		{key: "gen_ai_prompt", msgContains: "prompts are free text"},
		{key: "session", msgContains: "session"},
		{key: "gen_ai_response_id", msgContains: "response identifiers"},
		{key: "customer_id", msgContains: "identifiers are unbounded"},
		{key: "teem", msgContains: "not on the allowlist", hintContain: `did you mean "team"`},
		{key: "region", msgContains: "not on the allowlist", hintContain: "allowed keys:"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			p := Check(tt.key)
			if tt.ok {
				if p != nil {
					t.Fatalf("Check(%q) = %+v, want nil", tt.key, p)
				}
				return
			}
			if p == nil {
				t.Fatalf("Check(%q) = nil, want a problem", tt.key)
			}
			if !strings.Contains(p.Message, tt.msgContains) {
				t.Errorf("message %q does not contain %q", p.Message, tt.msgContains)
			}
			if !strings.Contains(p.Hint, tt.hintContain) {
				t.Errorf("hint %q does not contain %q", p.Hint, tt.hintContain)
			}
		})
	}
}

func TestAllowlistHasNoUnboundedKeys(t *testing.T) {
	for _, k := range Allowed() {
		for _, u := range unbounded {
			if strings.Contains(k, u.substr) {
				t.Errorf("allowlisted key %q matches unbounded pattern %q", k, u.substr)
			}
		}
		for _, p := range reservedPrefixes {
			if strings.HasPrefix(k, p) {
				t.Errorf("allowlisted key %q has reserved prefix %q", k, p)
			}
		}
	}
}
