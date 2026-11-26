package suggest

import "testing"

func TestClosest(t *testing.T) {
	cands := []string{"objective", "window", "name", "description", "sli", "labels", "alerting"}
	tests := []struct{ in, want string }{
		{"objectve", "objective"},
		{"objetcive", "objective"},
		{"windwo", "window"},
		{"naem", "name"},
		{"lables", "labels"},
		{"sl", "sli"},
		{"completely_different", ""},
		{"x", ""},
	}
	for _, tt := range tests {
		if got := Closest(tt.in, cands); got != tt.want {
			t.Errorf("Closest(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"a", "", 1}, {"abc", "abc", 0}, {"abc", "acb", 1}, {"kitten", "sitting", 3},
	}
	for _, tt := range tests {
		if got := Distance(tt.a, tt.b); got != tt.want {
			t.Errorf("Distance(%q,%q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
