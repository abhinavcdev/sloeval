package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abhinavcdev/sloeval/internal/golden"
)

func run(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	env := OSEnv()
	env.Stdin = strings.NewReader(stdin)
	env.Stdout, env.Stderr = &out, &errOut
	code = Run(args, env)
	return code, out.String(), errOut.String()
}

const example = "../../examples/specs/support-agent.yaml"

func TestValidateGolden(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"example-text", []string{"validate", example}, ExitOK},
		{"example-json", []string{"validate", "--format=json", example}, ExitOK},
		{"invalid-text", []string{"validate", "testdata/invalid.yaml"}, ExitError},
		{"invalid-json", []string{"validate", "--format", "json", "testdata/invalid.yaml"}, ExitError},
		{"two-files-text", []string{"validate", example, "testdata/invalid.yaml"}, ExitError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(t, "", tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tt.wantCode, errOut)
			}
			if errOut != "" {
				t.Errorf("unexpected stderr: %s", errOut)
			}
			golden.Check(t, filepath.Join("testdata", tt.name+".golden"), []byte(out))
		})
	}
}

func TestValidateExitCodes(t *testing.T) {
	data, err := os.ReadFile(example)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		stdin      string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no command", args: nil, wantCode: ExitUsage, wantStderr: "Usage:"},
		{name: "unknown command", args: []string{"compile"}, wantCode: ExitUsage, wantStderr: `unknown command "compile"`},
		{name: "help", args: []string{"help"}, wantCode: ExitOK, wantStdout: "Commands:"},
		{name: "version", args: []string{"version"}, wantCode: ExitOK, wantStdout: "sloe dev"},
		{name: "no files", args: []string{"validate"}, wantCode: ExitUsage, wantStderr: "Usage: sloe validate"},
		{name: "validate help", args: []string{"validate", "-h"}, wantCode: ExitOK, wantStderr: "Exit codes"},
		{name: "bad flag", args: []string{"validate", "--nope", example}, wantCode: ExitUsage, wantStderr: "flag provided but not defined"},
		{name: "bad format", args: []string{"validate", "--format=xml", example}, wantCode: ExitUsage, wantStderr: "--format must be text or json"},
		{name: "missing file", args: []string{"validate", "does-not-exist.yaml"}, wantCode: ExitUsage, wantStderr: "does-not-exist.yaml"},
		{name: "empty dir", args: []string{"validate", t.TempDir()}, wantCode: ExitUsage, wantStderr: "no .yaml or .yml files"},
		{name: "directory", args: []string{"validate", "../../examples/specs"}, wantCode: ExitOK, wantStdout: "support-agent.yaml: ok"},
		{name: "stdin", stdin: string(data), args: []string{"validate", "-"}, wantCode: ExitOK, wantStdout: "<stdin>: ok (1 document, 10 SLOs)"},
		{name: "stdin invalid", stdin: "kind: AISLO\n", args: []string{"validate", "-"}, wantCode: ExitError, wantStdout: "<stdin>: invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(t, tt.stdin, tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.wantCode, out, errOut)
			}
			if !strings.Contains(out, tt.wantStdout) {
				t.Errorf("stdout %q does not contain %q", out, tt.wantStdout)
			}
			if !strings.Contains(errOut, tt.wantStderr) {
				t.Errorf("stderr %q does not contain %q", errOut, tt.wantStderr)
			}
		})
	}
}
