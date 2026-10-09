# Implementation Plan: Unread struct-field gate (slopcheck)

**Branch**: `031-slopcheck-unread-fields` | **Date**: 2026-10-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/031-slopcheck-unread-fields/spec.md`

## Summary

Add an eighth `internal/cmd` gate, `slopcheck`. It fails CI when a struct field
is assigned in a composite literal and never read: the table-test
`expectError` fingerprint that `unused` cannot see. One stdlib-only core
(`internal/unreadfield`) classifies field loads, stores and whole-value uses
over `go/types` information. Two front ends consume it:

- the gate (`internal/cmd/slopcheck`), which loads every package with its tests
  through `go list -e -deps -export -test -json` under all four build-tag
  configurations, intersects the results, and speaks the repository's
  stdout-JSON / stderr-`ax.Error` contract;
- a `go/analysis` adapter (`internal/unreadfield/analyzer`), tested with
  `analysistest` over the same fixtures, plus a parity test.

`make slop-check` joins `make ci` and the CI `validate` job.

## Technical Context

**Language/Version**: Go 1.27.1 (pinned in `mise.toml`, matching `go.mod`)

**Primary Dependencies**: stdlib `go/ast`, `go/parser`, `go/token`,
`go/types`, `go/importer`, `os/exec`; `github.com/rshade/ax-go/contract`
(envelope); `golang.org/x/tools/go/analysis` and `.../analysistest` in the
adapter only. `x/tools` (already in the module graph via the MCP SDK's
tests, with no `go.mod` line today) becomes a direct requirement at the
MVS-selected `v0.50.0` (research R10).

**Storage**: N/A

**Testing**: `go test -race`, table-driven core tests over a fixture module,
`analysistest` with `// want`, a parity test, and golden byte comparison for
gate streams (research R12)

**Target Platform**: Host `GOOS`/`GOARCH` (CI: linux/amd64), four build-tag
configurations

**Project Type**: Internal CLI gate inside a Go library module

**Performance Goals**: Whole-repository run completes within the five-minute
per-configuration timeout on the CI runner (SC-005). No hot-path benchmark is
involved, so `bench-check` is not triggered.

**Constraints**: stdout is reserved for the pass document. No package-level
mutable state (`gochecknoglobals`). No public surface change. No
`.golangci.yml` edit (research R10). The isolated `logging` binary is unchanged.

**Scale/Scope**: About 60 analyzable units across 4 configurations. The core
is roughly 400 lines, the gate about 200, the adapter about 40.

**Governing ADR(s)**: N/A

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Verdict |
| --- | --- |
| I. Stream separation | Pass. Pass document on stdout only, one envelope on stderr only (contracts/gate-cli.md). |
| II. Deterministic output and exit codes | Pass. Sorted findings, struct-shaped pass document, exits `0/1/2/3/4` per research R8, two runs byte-identical (SC-004). |
| III. `__schema` | N/A. Internal gate, not an adopting CLI. |
| IV. Agent-safety primitives | N/A. Read-only gate with no side effects to dry-run. |
| V. Asymmetric JSON I/O | Pass. Emits strict minified JSON and reads no Hujson. |
| VI. Scope: library, not application | Pass. Lives under `internal/`, adds no public package (FR-013). |
| VII. Test-first | Pass. Fixtures and goldens are written before the core (tasks order). The one parser surface, the `go list -json` stream decoder, gets `FuzzDecodeListing` (research R12), as every sibling gate fuzzes its tool-output parser. Doc comments are contracts. |
| VIII. Observability and IDs | N/A. |
| IX. Security and resource safety | Pass. `go list` output is byte-capped; per-configuration timeout; no network (the fixture module has no requirements; `analysistest` sets `GOPROXY=off`). |
| X. Idiomatic Go and dependency minimalism | Pass with justification. `ctx` is the first parameter of `Run`; no globals (`New()` constructor, policy as functions). `x/tools` is justified in research R10: no new module in the graph, and confined to one internal package. |
| XI. Stability and SemVer | Pass. No exported-surface or payload change; `surface-check` baseline untouched. |
| XII. Deprecation | N/A. |

**ADR absorption gate**: N/A, no governing ADR.

**Post-design re-check (after Phase 1)**: unchanged. The design introduces no
new principle exposure. The one judgement call, the `x/tools` direct
requirement, is recorded in Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/031-slopcheck-unread-fields/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── gate-cli.md
│   └── core-api.md
├── checklists/requirements.md
└── tasks.md            # /speckit-tasks
```

### Source Code (repository root)

```text
internal/unreadfield/
├── doc.go                    # package contract
├── analyze.go                # Analyze: candidates, assignments, reads (R4–R7)
├── scope.go                  # candidate scope rules (R6)
├── escape.go                 # whole-value allow-list (R5)
├── load.go                   # go list -e -deps -export -test -json loader (R2)
├── run.go                    # Run: per-configuration load + intersection (R3, R9)
├── analyze_test.go           # fixture-driven table tests
├── run_test.go               # intersection, failure classification, BUILD_TAG_MATRIX sync
├── load_test.go              # loader units + FuzzDecodeListing
├── testdata/
│   ├── fixtures/             # shared: go.mod + one package per case, // want annotated, no _test.go
│   ├── testonlyread/         # module: field read only in a _test.go file
│   ├── taggedread/           # module: field read only under //go:build ax_no_grpc
│   └── broken/               # module: does not type-check
└── analyzer/
    ├── analyzer.go           # New() *analysis.Analyzer
    └── analyzer_test.go      # analysistest + parity

internal/cmd/slopcheck/
├── main.go                   # header doc (FR-016), flags, contract, exit
├── main_test.go              # goldens, stream assertions, injected-runner exit mapping
└── testdata/
    ├── clean/  failing/  broken/   # fixture modules for the stream goldens
    ├── pass.stdout.golden
    ├── fail.stderr.golden
    └── analysis.stderr.golden

Makefile                      # slop-check target; added to ci; help text
.github/workflows/ci.yml      # validate job: make slop-check step
go.mod / go.sum               # golang.org/x/tools direct
AGENTS.md, CONTEXT.md         # gate inventory, pre-flight checklist
```

**Structure Decision**: Follow the `internal/cmd/<tool>` gate convention
(`deadcheck`, `sizecheck`) for the front end that owns the contract. The
reusable classification lives in a sibling `internal/unreadfield` package so
that both front ends import it and neither can drift (FR-011).

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
| --- | --- | --- |
| `golang.org/x/tools` becomes a direct requirement | The maintainer chose a `go/analysis` front end (analysistest fixtures, a future lint-plugin path) | A stdlib-only build drops the chosen adapter. The module is already in the graph at `v0.50.0`, so no new module is added, and only one internal package imports it. |
| Two front ends over one core | Maintainer decision, spec FR-011 | A single harness would drop one maintainer requirement. The shared core keeps the duplication to about 40 lines of adapter. |
