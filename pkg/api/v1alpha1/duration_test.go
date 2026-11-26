package v1alpha1

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "5m", want: 5 * time.Minute},
		{in: "500ms", want: 500 * time.Millisecond},
		{in: "1h30m", want: 90 * time.Minute},
		{in: "30d", want: 30 * 24 * time.Hour},
		{in: "2w", want: 14 * 24 * time.Hour},
		{in: "1y", want: 365 * 24 * time.Hour},
		{in: "1m30s500ms", want: 90*time.Second + 500*time.Millisecond},
		{in: "", wantErr: true},
		{in: "0s", wantErr: true},
		{in: "2", wantErr: true},
		{in: "2 seconds", wantErr: true},
		{in: "2sec", wantErr: true},
		{in: "30m1h", wantErr: true},
		{in: "1h1h", wantErr: true},
		{in: "-5m", wantErr: true},
		{in: "1.5h", wantErr: true},
		{in: "99999999999999999999s", wantErr: true},
		{in: "9999999999y", wantErr: true},
		{in: "292y52w", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDuration(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDuration(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h30m"},
		{30 * 24 * time.Hour, "30d"},
		{1500 * time.Millisecond, "1s500ms"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.in); got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDefaultBurnRates(t *testing.T) {
	window := 30 * 24 * time.Hour
	want := []float64{14.4, 6, 3, 1}
	all := append(append([]BudgetWindow{}, DefaultPageWindows...), DefaultTicketWindows...)
	for i, w := range all {
		got := w.BurnRate(window)
		if diff := got - want[i]; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("window %d (%v/%v): burn rate = %v, want %v", i, w.Long, w.Short, got, want[i])
		}
	}
}
