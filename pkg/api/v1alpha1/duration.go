package v1alpha1

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// Duration is a Prometheus-style duration such as "5m", "1h30m" or "30d".
// It is stored as the string the user wrote; use Parse to get its value.
type Duration string

// Parse returns the duration value. Units, largest first and each at most
// once: y (365d), w, d, h, m, s, ms. Numbers may have a decimal fraction
// ("25.6s"), which is useful for histogram thresholds.
func (d Duration) Parse() (time.Duration, error) {
	return ParseDuration(string(d))
}

var durationUnits = []struct {
	name string
	d    time.Duration
}{
	{"y", 365 * 24 * time.Hour},
	{"w", 7 * 24 * time.Hour},
	{"d", 24 * time.Hour},
	{"h", time.Hour},
	{"m", time.Minute},
	{"s", time.Second},
	{"ms", time.Millisecond},
}

// ParseDuration parses a Prometheus-style duration. It rejects empty input,
// zero, units out of order and repeated units.
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	var total time.Duration
	next := 0 // index into durationUnits of the smallest unit still allowed
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i {
			return 0, fmt.Errorf("invalid duration %q: expected a number at position %d", s, i)
		}
		n, err := strconv.ParseInt(s[i:j], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		frac := ""
		if j < len(s) && s[j] == '.' {
			f := j + 1
			for f < len(s) && s[f] >= '0' && s[f] <= '9' {
				f++
			}
			if f == j+1 {
				return 0, fmt.Errorf("invalid duration %q: expected digits after '.'", s)
			}
			frac = s[j+1 : f]
			j = f
		}
		k := j
		for k < len(s) && (s[k] < '0' || s[k] > '9') {
			k++
		}
		unit := s[j:k]
		if unit == "" {
			return 0, fmt.Errorf("invalid duration %q: missing unit after %d", s, n)
		}
		idx := -1
		for u := range durationUnits {
			if durationUnits[u].name == unit {
				idx = u
				break
			}
		}
		if idx < 0 {
			return 0, fmt.Errorf("invalid duration %q: unknown unit %q", s, unit)
		}
		if idx < next {
			return 0, fmt.Errorf("invalid duration %q: units must go from largest to smallest, each once", s)
		}
		next = idx + 1
		ud := durationUnits[idx].d
		if n > int64((1<<63-1)/ud) {
			return 0, fmt.Errorf("invalid duration %q: too large", s)
		}
		add := time.Duration(n) * ud
		if frac != "" {
			fv, _ := strconv.ParseFloat("0."+frac, 64)
			add += time.Duration(math.Round(fv * float64(ud)))
		}
		if total > time.Duration(1<<63-1)-add {
			return 0, fmt.Errorf("invalid duration %q: too large", s)
		}
		total += add
		i = k
	}
	if total <= 0 {
		return 0, fmt.Errorf("invalid duration %q: must be greater than zero", s)
	}
	return total, nil
}

// FormatDuration renders d in Prometheus style, e.g. 90m -> "1h30m", 30d -> "30d".
// Weeks and years are not used, so the output round-trips predictably.
func FormatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	out := ""
	for _, u := range durationUnits {
		if u.name == "y" || u.name == "w" {
			continue
		}
		if n := d / u.d; n > 0 {
			out += strconv.FormatInt(int64(n), 10) + u.name
			d -= n * u.d
		}
	}
	return out
}
