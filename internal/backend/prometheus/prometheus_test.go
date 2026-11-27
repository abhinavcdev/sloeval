package prometheus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abhinavcdev/sloeval/internal/golden"
	"github.com/abhinavcdev/sloeval/internal/ir"
	"github.com/abhinavcdev/sloeval/internal/semconv"
	"github.com/abhinavcdev/sloeval/internal/validate"
)

func load(t *testing.T, path string) []*ir.Spec {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := validate.Run([]validate.Input{{Name: path, Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("spec is invalid: %v", res.Diagnostics)
	}
	var out []*ir.Spec
	for _, d := range res.Docs {
		s, err := ir.Build(d.AISLO)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// TestGolden renders every testdata/specs/*.yaml (and the examples) into
// testdata/golden/<case>/<id>.{rules,test}.yaml. CI runs promtool check and
// test over all of them.
func TestGolden(t *testing.T) {
	type tc struct {
		name, path string
		naming     semconv.Naming
	}
	var cases []tc
	specs, _ := filepath.Glob("testdata/specs/*.yaml")
	for _, p := range specs {
		cases = append(cases, tc{strings.TrimSuffix(filepath.Base(p), ".yaml"), p, semconv.NamingUnderscore})
	}
	examples, _ := filepath.Glob("../../../examples/specs/*.yaml")
	for _, p := range examples {
		base := "example-" + strings.TrimSuffix(filepath.Base(p), ".yaml")
		cases = append(cases, tc{base, p, semconv.NamingUnderscore}, tc{base + "-utf8", p, semconv.NamingUTF8})
	}
	if len(cases) < 10 {
		t.Fatalf("expected test specs, found %d", len(cases))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opt := Options{Naming: c.naming}
			for _, s := range load(t, c.path) {
				rules, err := Rules(s, opt)
				if err != nil {
					t.Fatal(err)
				}
				ruleFile := s.ID + ".rules.yaml"
				tests, err := Tests(s, ruleFile, opt)
				if err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join("testdata", "golden", c.name)
				golden.Check(t, filepath.Join(dir, ruleFile), rules)
				golden.Check(t, filepath.Join(dir, s.ID+".test.yaml"), tests)
			}
		})
	}
}

func TestEveryAlertHasATest(t *testing.T) {
	for _, s := range load(t, "../../../examples/specs/support-agent.yaml") {
		tests, err := Tests(s, "x.rules.yaml", Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, slo := range s.SLOs {
			for _, a := range slo.Alerts {
				if !strings.Contains(string(tests), "alertname: "+a.Name) {
					t.Errorf("alert %s has no test", a.Name)
				}
			}
		}
	}
}
