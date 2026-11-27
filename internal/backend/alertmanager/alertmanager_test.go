package alertmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abhinavcdev/sloeval/internal/golden"
	"github.com/abhinavcdev/sloeval/internal/ir"
	"github.com/abhinavcdev/sloeval/internal/validate"
)

func TestRoutesGolden(t *testing.T) {
	tests := []struct{ name, path string }{
		{"multi-team", "testdata/multi-team.yaml"},
		{"example-support-agent", "../../../examples/specs/support-agent.yaml"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var specs []*ir.Spec
			if tt.path != "" {
				data, err := os.ReadFile(tt.path)
				if err != nil {
					t.Fatal(err)
				}
				res, err := validate.Run([]validate.Input{{Name: tt.path, Data: data}})
				if err != nil || !res.OK() {
					t.Fatalf("invalid: %v %v", err, res.Diagnostics)
				}
				for _, d := range res.Docs {
					s, err := ir.Build(d.AISLO)
					if err != nil {
						t.Fatal(err)
					}
					specs = append(specs, s)
				}
			}
			out, err := Routes(specs)
			if err != nil {
				t.Fatal(err)
			}
			golden.Check(t, filepath.Join("testdata", tt.name+".golden.yaml"), out)
		})
	}
}
