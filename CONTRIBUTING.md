# Contributing to sloeval

Thanks for helping. This project compiles AISLO specs into Prometheus rules,
alerts, Alertmanager routing and Grafana dashboards. Correctness of the generated
output matters more than anything else, so most of the process below is about
keeping it reviewable and tested.

## Getting started

```sh
make build        # bin/sloe
make test         # unit + golden tests (with -race)
make lint         # golangci-lint v2
make promtool     # fetch pinned, checksum-verified promtool into .tools/
make promtool-check PROMTOOL=.tools/promtool
```

Go 1.24+ is required. Python is used only under `sdk/python` (later phases).

## Commits and pull requests

- Small, reviewable commits that each build and pass tests.
- Conventional prefixes: `feat:`, `fix:`, `docs:`, `test:`, `build:`, `ci:`, `refactor:`.
- Design changes (API, generated metric/rule names, new backends) start as a
  document in `docs/design/NNNN-title.md` and are agreed before code lands.

## Tests and golden files

- Tests are table-driven.
- Every generated output (diagnostics, rules, dashboards) has a golden file
  under the package's `testdata/`. Regenerate with `make golden`, then review
  the diff line by line before committing. CI fails if goldens are stale.
- Every generated rule file must pass `promtool check rules`, and every alert
  must have a `promtool test rules` unit test.

## Dependencies

Ask (in an issue or the PR description) before adding a Go or Python
dependency, and say why the standard library is not enough. Current direct
dependencies:

| Module | Purpose |
|---|---|
| `go.yaml.in/yaml/v3` | YAML parsing with line/column positions |
| `github.com/santhosh-tekuri/jsonschema/v6` | JSON Schema (Draft 2020-12) validation |

## Labels and privacy

Never put prompts, completions, user IDs, session IDs or other free text into
metric labels. The compiler enforces an allowlist in `internal/labels`; adding a
key there needs a justification of its cardinality in the PR.

## OpenTelemetry GenAI semantic conventions

All semconv names live in `internal/semconv/semconv.go`. Do not hard-code a
`gen_ai.*` name anywhere else.

## License

By contributing you agree that your contributions are licensed under the
Apache License 2.0.
