# Phase 0 (continue): scaffold, `sloe validate`, examples

Design 0001 (docs/design/0001-aislo-v1alpha1.md) is approved, including the
decisions in section 8. Mark its status "accepted". Approved dependencies:
`go.yaml.in/yaml/v3` and `github.com/santhosh-tekuri/jsonschema/v6`. Ask before adding any other.

Tasks, one or more small commits each:
2. Scaffold:
   - go.mod: module `github.com/abhinavcdev/sloeval`, go 1.24.
   - Makefile targets: build, test, lint, golden (regenerate with -update), promtool-check.
   - .golangci.yml.
   - GitHub Actions jobs: lint, test (with -race), promtool. The promtool job installs a
     pinned Prometheus release, verifies its checksum, and runs `promtool check rules` and
     `promtool test rules` over every golden rule/test file (a no-op while none exist).
   - LICENSE (Apache-2.0), CONTRIBUTING.md (commit style, golden workflow, dependency policy).
   - CLAUDE.md recording the full project brief plus the design 0001 decisions.
3. Implement `sloe validate <file>...`:
   - pkg/api/v1alpha1: types, Duration parser, defaults, embedded JSON Schema.
   - internal/spec: multi-doc YAML loader that keeps the line/col of every field path.
   - internal/labels: allowlist plus the unbounded-key denylist with explanations.
   - internal/semconv: the single file of OTel GenAI names (cite the semconv version pinned).
   - internal/validate: schema pass mapped to friendly messages with "did you mean" hints,
     then the semantic pass from design 0001 section 5.
   - CLI: `--format=text|json`; exit codes 0 = ok or warnings only, 1 = errors, 2 = usage/IO.
4. examples/specs/support-agent.yaml (valid, every SLI type), plus testdata/invalid/*.yaml
   with one file per diagnostic. Table-driven tests; golden files for text and JSON output.

Done when: `make lint test` is clean, CI is green on the branch, and every diagnostic has
a golden test.
Stop after Phase 0. Summarize what's done, what's left, and open questions.
