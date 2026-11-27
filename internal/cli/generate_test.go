package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMatchesBackendGoldens(t *testing.T) {
	tests := []struct {
		args   []string
		golden string
	}{
		{nil, "example-support-agent"},
		{[]string{"--metric-naming=utf8"}, "example-support-agent-utf8"},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			out := t.TempDir()
			keep := filepath.Join(out, "prometheus", "unrelated.txt")
			if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"generate", "-o", out}, tt.args...)
			code, stdout, stderr := run(t, "", append(args, example)...)
			if code != ExitOK {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			for _, name := range []string{"support.support-agent.rules.yaml", "support.support-agent.test.yaml"} {
				got, err := os.ReadFile(filepath.Join(out, "prometheus", name))
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(filepath.Join("../backend/prometheus/testdata/golden", tt.golden, name))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s differs from the backend golden", name)
				}
			}
			got, _ := os.ReadFile(filepath.Join(out, "alertmanager", "routes.yaml"))
			want, _ := os.ReadFile("../backend/alertmanager/testdata/example-support-agent.golden.yaml")
			if !bytes.Equal(got, want) {
				t.Error("routes.yaml differs from the backend golden")
			}
			if _, err := os.Stat(keep); err != nil {
				t.Error("generate removed an unrelated file")
			}
			if !strings.Contains(stdout, "wrote ") {
				t.Errorf("stdout = %q", stdout)
			}
		})
	}
}

func TestGenerateErrors(t *testing.T) {
	out := t.TempDir()
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"no output dir", []string{"generate", example}, ExitUsage, "Usage: sloe generate"},
		{"no files", []string{"generate", "-o", out}, ExitUsage, "Usage: sloe generate"},
		{"bad naming", []string{"generate", "-o", out, "--metric-naming=dots", example}, ExitUsage, "--metric-naming must be"},
		{"invalid spec", []string{"generate", "-o", out, "testdata/invalid.yaml"}, ExitError, "3 errors; nothing written"},
		{"help", []string{"generate", "-h"}, ExitOK, "promtool unit tests"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(t, "", tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit = %d, want %d; stderr: %s", code, tt.wantCode, stderr)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("stderr %q does not contain %q", stderr, tt.wantStderr)
			}
		})
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 0 {
		t.Errorf("files written despite errors: %v", entries)
	}
}
