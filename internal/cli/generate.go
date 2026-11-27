package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abhinavcdev/sloeval/internal/backend/alertmanager"
	"github.com/abhinavcdev/sloeval/internal/backend/prometheus"
	"github.com/abhinavcdev/sloeval/internal/diag"
	"github.com/abhinavcdev/sloeval/internal/ir"
	"github.com/abhinavcdev/sloeval/internal/semconv"
	"github.com/abhinavcdev/sloeval/internal/validate"
)

func runGenerate(args []string, env Env) int {
	fl := flag.NewFlagSet("generate", flag.ContinueOnError)
	fl.SetOutput(env.Stderr)
	outDir := fl.String("o", "", "output directory (required)")
	naming := fl.String("metric-naming", string(semconv.NamingUnderscore),
		"Prometheus metric naming: underscore (UnderscoreEscapingWithSuffixes) or utf8 (NoUTF8EscapingWithSuffixes, experimental)")
	fl.Usage = func() {
		fmt.Fprint(env.Stderr, `Usage: sloe generate -o <dir> [flags] <file|dir|->...

Validates the specs, then writes:
  <dir>/prometheus/<id>.rules.yaml   recording rules and alerts
  <dir>/prometheus/<id>.test.yaml    promtool unit tests for every alert
  <dir>/alertmanager/routes.yaml     routing snippet with placeholder receivers

<id> is metadata.name, or <namespace>.<name>. Nothing is written if any
spec has errors. Other files in <dir> are left alone.

Exit codes: 0 generated, 1 specs have errors, 2 usage or I/O error.

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
	n, ok := semconv.ParseNaming(*naming)
	if !ok {
		fmt.Fprintf(env.Stderr, "sloe generate: --metric-naming must be underscore or utf8, got %q\n", *naming)
		return ExitUsage
	}
	if *outDir == "" || fl.NArg() == 0 {
		fl.Usage()
		return ExitUsage
	}
	inputs, err := readInputs(fl.Args(), env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sloe generate: %v\n", err)
		return ExitUsage
	}
	res, err := validate.Run(inputs)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sloe generate: %v\n", err)
		return ExitUsage
	}
	_ = diag.WriteText(env.Stderr, res.Diagnostics)
	if !res.OK() {
		errs, _ := diag.Count(res.Diagnostics)
		fmt.Fprintf(env.Stderr, "sloe generate: %s; nothing written\n", plural(errs, "error"))
		return ExitError
	}
	if n == semconv.NamingUTF8 {
		fmt.Fprintln(env.Stderr, "sloe generate: note: --metric-naming=utf8 is experimental")
	}

	type file struct {
		path string
		data []byte
	}
	var files []file
	var specs []*ir.Spec
	opt := prometheus.Options{Naming: n}
	for _, d := range res.Docs {
		s, err := ir.Build(d.AISLO)
		if err != nil {
			fmt.Fprintf(env.Stderr, "%s: error: %v\n", d.Source.File, err)
			return ExitError
		}
		specs = append(specs, s)
		ruleFile := s.ID + ".rules.yaml"
		rules, err := prometheus.Rules(s, opt)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sloe generate: %s: %v\n", s.ID, err)
			return ExitUsage
		}
		tests, err := prometheus.Tests(s, ruleFile, opt)
		if err != nil {
			fmt.Fprintf(env.Stderr, "sloe generate: %s: %v\n", s.ID, err)
			return ExitError
		}
		dir := filepath.Join(*outDir, "prometheus")
		files = append(files,
			file{filepath.Join(dir, ruleFile), rules},
			file{filepath.Join(dir, s.ID+".test.yaml"), tests})
	}
	routes, err := alertmanager.Routes(specs)
	if err != nil {
		fmt.Fprintf(env.Stderr, "sloe generate: %v\n", err)
		return ExitUsage
	}
	files = append(files, file{filepath.Join(*outDir, "alertmanager", "routes.yaml"), routes})

	for _, f := range files {
		if err := env.WriteFile(f.path, f.data); err != nil {
			fmt.Fprintf(env.Stderr, "sloe generate: %v\n", err)
			return ExitUsage
		}
		fmt.Fprintf(env.Stdout, "wrote %s\n", f.path)
	}
	return ExitOK
}

// writeFileAtomic writes data to a temp file next to path and renames it.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
