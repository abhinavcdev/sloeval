// Package cli implements the sloe command line.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/abhinavcdev/sloeval/internal/diag"
	"github.com/abhinavcdev/sloeval/internal/validate"
)

// Exit codes.
const (
	ExitOK    = 0
	ExitError = 1 // the input has errors
	ExitUsage = 2 // bad usage or I/O failure
)

// Version is set at build time with -ldflags "-X .../internal/cli.Version=...".
var Version = "dev"

const usage = `sloe compiles AISLO specs into Prometheus rules, alerts and dashboards.

Usage:
  sloe <command> [flags]

Commands:
  validate   Check AISLO spec files and report problems
  version    Print the sloe version

Run "sloe <command> -h" for command flags.
`

// Env is the process environment the CLI runs in.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	ReadFile       func(string) ([]byte, error)
	Stat           func(string) (fs.FileInfo, error)
	WalkDir        func(string, fs.WalkDirFunc) error
}

// OSEnv returns an Env backed by the real process.
func OSEnv() Env {
	return Env{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		ReadFile: os.ReadFile, Stat: os.Stat, WalkDir: filepath.WalkDir,
	}
}

// Run executes the CLI with args (without the program name) and returns the exit code.
func Run(args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "validate":
		return runValidate(args[1:], env)
	case "version", "--version":
		fmt.Fprintln(env.Stdout, "sloe", Version)
		return ExitOK
	case "help", "-h", "--help":
		fmt.Fprint(env.Stdout, usage)
		return ExitOK
	default:
		fmt.Fprintf(env.Stderr, "sloe: unknown command %q\n\n%s", args[0], usage)
		return ExitUsage
	}
}

func runValidate(args []string, env Env) int {
	fl := flag.NewFlagSet("validate", flag.ContinueOnError)
	fl.SetOutput(env.Stderr)
	format := fl.String("format", "text", "output format: text or json")
	fl.Usage = func() {
		fmt.Fprint(env.Stderr, `Usage: sloe validate [flags] <file|dir|->...

Checks AISLO spec files. Directories are searched for *.yaml and *.yml files;
"-" reads from standard input.

Exit codes: 0 valid (warnings allowed), 1 errors found, 2 usage or I/O error.

Flags:
`)
		fl.PrintDefaults()
	}
	if err := fl.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(env.Stderr, "sloe validate: --format must be text or json, got %q\n", *format)
		return ExitUsage
	}
	if fl.NArg() == 0 {
		fl.Usage()
		return ExitUsage
	}
	inputs, err := readInputs(fl.Args(), env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sloe validate: %v\n", err)
		return ExitUsage
	}
	res, err := validate.Run(inputs)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sloe validate: %v\n", err)
		return ExitUsage
	}

	report := buildReport(inputs, res)
	switch *format {
	case "json":
		if err := diag.WriteJSON(env.Stdout, report); err != nil {
			return ExitUsage
		}
	default:
		writeText(env.Stdout, report)
	}
	if report.Errors > 0 {
		return ExitError
	}
	return ExitOK
}

func readInputs(args []string, env Env) ([]validate.Input, error) {
	var inputs []validate.Input
	for _, a := range args {
		if a == "-" {
			b, err := io.ReadAll(env.Stdin)
			if err != nil {
				return nil, fmt.Errorf("reading stdin: %w", err)
			}
			inputs = append(inputs, validate.Input{Name: "<stdin>", Data: b})
			continue
		}
		info, err := env.Stat(a)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			b, err := env.ReadFile(a)
			if err != nil {
				return nil, err
			}
			inputs = append(inputs, validate.Input{Name: a, Data: b})
			continue
		}
		var files []string
		err = env.WalkDir(a, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ext := strings.ToLower(filepath.Ext(path)); !d.IsDir() && (ext == ".yaml" || ext == ".yml") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("%s: no .yaml or .yml files found", a)
		}
		slices.Sort(files)
		for _, f := range files {
			b, err := env.ReadFile(f)
			if err != nil {
				return nil, err
			}
			inputs = append(inputs, validate.Input{Name: f, Data: b})
		}
	}
	return inputs, nil
}

func buildReport(inputs []validate.Input, res validate.Result) diag.Report {
	r := diag.Report{Diagnostics: res.Diagnostics}
	idx := map[string]int{}
	for _, in := range inputs {
		if _, ok := idx[in.Name]; ok {
			continue
		}
		idx[in.Name] = len(r.Files)
		r.Files = append(r.Files, diag.FileReport{File: in.Name})
	}
	for _, d := range res.Docs {
		fr := &r.Files[idx[d.Source.File]]
		fr.Documents++
		if d.AISLO != nil {
			fr.SLOs += len(d.AISLO.Spec.SLOs)
		}
	}
	for _, d := range res.Diagnostics {
		fr := &r.Files[idx[d.File]]
		switch d.Severity {
		case diag.Error:
			fr.Errors++
		case diag.Warning:
			fr.Warnings++
		}
	}
	r.Errors, r.Warnings = diag.Count(res.Diagnostics)
	r.Valid = r.Errors == 0
	return r
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func writeText(w io.Writer, r diag.Report) {
	_ = diag.WriteText(w, r.Diagnostics)
	for _, f := range r.Files {
		status := "ok"
		if f.Errors > 0 {
			status = "invalid"
		}
		parts := []string{plural(f.Documents, "document"), plural(f.SLOs, "SLO")}
		if f.Errors > 0 {
			parts = append(parts, plural(f.Errors, "error"))
		}
		if f.Warnings > 0 {
			parts = append(parts, plural(f.Warnings, "warning"))
		}
		fmt.Fprintf(w, "%s: %s (%s)\n", f.File, status, strings.Join(parts, ", "))
	}
}
