package validate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abhinavcdev/sloeval/internal/diag"
	"github.com/abhinavcdev/sloeval/internal/golden"
)

// TestCases validates every testdata/cases/*.yaml file and compares the
// diagnostics with its .golden file. The file name prefix states the
// expectation: err- (at least one error), warn- (warnings only), ok- (clean).
func TestCases(t *testing.T) {
	files, err := filepath.Glob("testdata/cases/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test cases: %v", err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			// Use a stable, short file name in output.
			res, err := Run([]Input{{Name: name + ".yaml", Data: data}})
			if err != nil {
				t.Fatal(err)
			}
			errs, warns := diag.Count(res.Diagnostics)
			switch {
			case strings.HasPrefix(name, "err-"):
				if errs == 0 {
					t.Errorf("expected at least one error, got none")
				}
			case strings.HasPrefix(name, "warn-"):
				if errs != 0 || warns == 0 {
					t.Errorf("expected warnings only, got %d errors, %d warnings", errs, warns)
				}
			case strings.HasPrefix(name, "ok-"):
				if errs != 0 || warns != 0 {
					t.Errorf("expected no diagnostics, got %d errors, %d warnings", errs, warns)
				}
			default:
				t.Fatalf("case name must start with err-, warn- or ok-")
			}
			for _, d := range res.Diagnostics {
				if d.Message == "" {
					t.Errorf("diagnostic without message: %+v", d)
				}
				if d.Severity == diag.Error && d.Hint == "" && !strings.HasPrefix(name, "err-yaml") {
					t.Errorf("error without a hint: %s", d)
				}
			}
			var buf bytes.Buffer
			if err := diag.WriteText(&buf, res.Diagnostics); err != nil {
				t.Fatal(err)
			}
			golden.Check(t, filepath.Join("testdata", "cases", name+".golden"), buf.Bytes())
		})
	}
}

func TestExamplesAreClean(t *testing.T) {
	files, err := filepath.Glob("../../examples/specs/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Run([]Input{{Name: f, Data: data}})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range res.Diagnostics {
			t.Errorf("%s", d)
		}
		for _, d := range res.Docs {
			if d.AISLO == nil {
				t.Errorf("%s: document %d not decoded", f, d.Source.Index)
			}
		}
	}
}
