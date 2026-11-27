package prometheus

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/abhinavcdev/sloeval/internal/labels"
	"github.com/abhinavcdev/sloeval/internal/semconv"
)

// matcherRe finds label matchers in PromQL and promtool series notation:
// name="v", name=~"v", "dotted.name"!="v" ...
var matcherRe = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*|"[^"]+")\s*(=~|!~|!=|=)\s*"`)

// allowedLabel reports whether a label may appear in generated output: the
// spec allowlist, sloe's own labels, le, and the judge runner's labels.
func allowedLabel(name string) bool {
	name = semconv.PromLabel(strings.Trim(name, `"`)) // utf8 mode: compare in underscore form
	switch name {
	case "le", semconv.LabelJudge, semconv.LabelReference, semconv.LabelOutcome:
		return true
	}
	return labels.IsAllowed(name) || strings.HasPrefix(name, "sloe_")
}

// TestGoldensUseOnlyAllowedLabels scans every generated golden file: no
// spec may smuggle a label past the allowlist into rules, alerts or tests.
func TestGoldensUseOnlyAllowedLabels(t *testing.T) {
	files, _ := filepath.Glob("testdata/golden/*/*.yaml")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		walk(doc, "", func(key, path string, v any) {
			switch val := v.(type) {
			case map[string]any:
				if key == "labels" || key == "exp_labels" {
					for k := range val {
						if !allowedLabel(k) {
							t.Errorf("%s: %s: label %q is not allowed", f, path, k)
						}
					}
				}
			case string:
				if key == "expr" || key == "series" || key == "labels" {
					for _, m := range matcherRe.FindAllStringSubmatch(val, -1) {
						if !allowedLabel(m[1]) {
							t.Errorf("%s: %s: matcher label %s is not allowed", f, path, m[1])
						}
					}
				}
			}
		})
	}
}

func walk(v any, path string, fn func(key, path string, v any)) {
	switch val := v.(type) {
	case map[string]any:
		for k, x := range val {
			p := path + "." + k
			fn(k, p, x)
			walk(x, p, fn)
		}
	case []any:
		for _, x := range val {
			walk(x, path+"[]", fn)
		}
	}
}
