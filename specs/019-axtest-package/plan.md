# Implementation Plan: axtest — Full-Lifecycle Command Test Helper

**Branch**: `019-axtest-package` | **Date**: 2026-08-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/019-axtest-package/spec.md`

## Summary

Add a new public package `axtest` (module root, joining `config`, `contract`,
`id`, `logging`, `mcp`, `schema`) that wraps `ax.Execute` so a test can run a
command tree through the real startup lifecycle — agent-safety flags mounted,
mode resolved, context populated — and get back a `Result{Stdout, Stderr,
ExitCode}`, plus a generic `Decode[T]` that unwraps `ax.Envelope[T]`'s `Data`
field without a hand-declared wrapper struct, and a `RunAndDecode[T]`
convenience composing both for the success-path common case. `axtest` is
purely additive: it changes no existing exported signature and no runtime
behavior of `Execute` itself. Because it is designed to be imported only from
`_test.go` files, a new automated check (research.md) asserts no production
source file in the module imports it — a stricter guarantee than the
stdlib's own `httptest`/`fstest` precedent, matching this project's existing
bar for its public surfaces.

## Technical Context

**Language/Version**: Go 1.26.7 (matches `go.mod`; pinned in `mise.toml`)

**Primary Dependencies**: `github.com/spf13/cobra` (already a module
dependency; `*cobra.Command` is `Run`/`RunAndDecode`'s input type), the root
`ax` package (`ax.Execute`, `ax.ExecuteOption`, `ax.Envelope[T]`), and the
standard library `testing` package. No new third-party dependency.

**Storage**: N/A

**Testing**: `go test -race` with table-driven tests against a toy command
tree; a required, `// Output:`-verified `ExampleRun`/`ExampleRunAndDecode`
gated by `make doc-coverage`; a new import-direction test (`go list -json`
based) asserting no non-test source file imports `axtest` across all 4 build
configurations × 6 supported GOOS/GOARCH profiles; a second, non-gated
demonstration test added to `examples/integration/` against the real reference
command.

**Target Platform**: Cross-platform Go library. `axtest` has no
platform-specific code; its public surface and reverse import restriction are
both verified across the existing 6 GOOS/GOARCH × 4 build-tag matrix.

**Project Type**: Go library (single module, public packages at module
root).

**Performance Goals**: None claimed (spec Out of Scope: no performance
claim). No `internal/cmd/benchcheck` entry is added.

**Constraints**: None on `axtest`'s own dependency graph — it is not
size-isolated (research.md). The constraint this feature adds runs the other
direction: zero non-test source files elsewhere in the module may import
`axtest`, enforced automatically rather than left to convention.

**Scale/Scope**: One new public package (`axtest/`: `run.go`, `decode.go`,
`doc.go`, `example_test.go`, `import_isolation_test.go` — reversed-direction
isolation, see research.md); one new demonstration test file under
`examples/integration/`; edits to three gate allowlists
(`internal/cmd/surfacecheck/inventory.go`, `internal/cmd/apidiff-verdict/main.go`,
`internal/cmd/doccover/main.go`) plus their "seven public packages" prose;
one new `internal/cmd/covercheck` per-package floor entry; `README.md` and
`AGENTS.md` documentation updates.

**Governing ADR(s)**: N/A — see research.md's ADR governance note. No ADR in
`docs/adr/` addresses test tooling or execution-lifecycle testing.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design — no
design decision introduced a new violation, so the table is unchanged
post-design.*

| Principle | Verdict | Notes |
|---|---|---|
| I. Stream Separation | **PASS** | `Result` keeps `Stdout`/`Stderr` as distinct fields, mirroring `ax.Execute`'s own separation; `Decode` reads only `Stdout`. No new code writes to either stream except through `ax.Execute` itself. |
| II. Deterministic Output & Exit Codes | **N/A** | `axtest` produces no payload of its own; it passes through whatever `ax.Execute` already produces, unchanged. |
| III. `__schema` Discoverability | **N/A** | No command-tree or schema change. |
| IV. Agent-Safety Primitives | **PASS** | The entire point of `Run` is to exercise dry-run, no-prompt approval, format, and idempotency-key mounting exactly as production does (FR-001, FR-003, FR-004) — this feature makes that behavior *testable*, not different. |
| V. Asymmetric JSON I/O | **N/A** | `Decode` reads strict JSON `axtest`'s target commands already wrote per Principle V; it does not add a new input format. |
| VI. ADR-Governed Scope / No Second CLI Framework, No Persistent State | **PASS** | No new CLI framework, no persisted state, no `init()` side effects. New public package is authorized by this Spec Kit feature, as AGENTS.md requires. |
| VII. Test-First Discipline | **PASS (binding)** | Tests land before implementation (tasks.md, next phase). `ExampleRun`/`ExampleRunAndDecode` are required primary-API examples per doccover's tiering, added to `requiredSymbols()` in the same change that adds the functions. |
| VIII. Observability & ID Discipline | **N/A** | `axtest` does not log, trace, or mint IDs; it forwards whatever `ax.Execute` already does. |
| IX. Security & Resource Safety | **PASS** | No new I/O beyond capturing the streams `ax.Execute` already writes; no user input parsed beyond the command's own args (a test's own responsibility, same as today); no TLS surface. |
| X. Idiomatic Go & Dependency Minimalism | **PASS** | Zero new third-party dependencies (Technical Context above). New public package placed at module root per Principle X, justified by this feature. `Run` and `RunAndDecode` do I/O (they call `ax.Execute`), so both take `ctx context.Context` as their first parameter, forwarded unmodified to `ax.Execute` — matching this principle's ctx-first rule and this module's own `internal/testutil` precedent (`AssertNoForbiddenImports` and its callers already put `ctx` before `testing.TB`). `Decode` does no I/O (pure `json.Unmarshal` on an in-memory value) and correctly takes no context. |
| XI. Stability & SemVer | **PASS** | Additive only — no existing exported signature changes (spec Out of Scope, FR is additive throughout). `axtest` itself becomes governed by this policy as the 8th public package (research.md), same tier as `config`/`contract`/`id`/`logging`/`mcp`/`schema`. |
| XII. Deprecation Lifecycle | **N/A** | Nothing is deprecated or removed. |

**Gate result: PASS.** No violation requires justification; Complexity
Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/019-axtest-package/
├── spec.md                          # Feature specification
├── plan.md                          # This file
├── research.md                      # Phase 0 output
├── data-model.md                    # Phase 1 output
├── quickstart.md                    # Phase 1 output
├── contracts/
│   └── axtest-package.md            # Phase 1 output — public API contract
├── checklists/
│   └── requirements.md
└── tasks.md                         # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
axtest/                              # NEW — public test-helper package
├── run.go                           # Result, Run
├── decode.go                        # Decode, RunAndDecode
├── doc.go                           # package doc: test-only usage convention
├── example_test.go                  # ExampleRun, ExampleDecode, ExampleRunAndDecode (doccover-gated)
├── testhelpers_test.go              # shared newGreetCommand fixture used by every story's tests
├── run_test.go                      # table-driven: dry-run, approval granted/blocked, re-mount safety,
│                                     # concurrent use across independent trees
├── decode_test.go                   # table-driven: success decode, malformed-shape failure
├── runanddecode_test.go             # RunAndDecode matches Run+Decode composed by hand
└── import_isolation_test.go         # reverse-direction check: no non-test source imports axtest,
                                      # asserted across all 4 configurations × 6 profiles (see research.md)

examples/integration/                # MODIFIED — demonstration test against the real reference command
└── axtest_example_test.go           # NEW

internal/cmd/surfacecheck/
├── inventory.go                     # MODIFIED — PublicPackages() gains axtest; doc comment count updated
└── baseline.json                    # MODIFIED — reviewed baseline entries for the new package

internal/cmd/apidiff-verdict/
└── main.go                          # MODIFIED — allowedPackages() gains axtest

internal/cmd/doccover/
├── main.go                          # MODIFIED — scannedPackages()/requiredSymbols() gain axtest.Run, axtest.Decode, axtest.RunAndDecode
└── baseline.txt                     # MODIFIED — provisional entries only if examples land after the symbols are required (should not be needed if tests-first is followed)

internal/cmd/covercheck/
└── main.go                          # MODIFIED — new perPackage floor entry for github.com/rshade/ax-go/axtest

internal/testutil/
├── imports.go                       # MODIFIED — new reverse-direction helpers (ModulePackage,
│                                     # ResolveModulePackages, FindNonTestImporters,
│                                     # AssertNoProductionImport), profile- and tags-aware so the check
│                                     # can run under all 24 supported combinations, used by
│                                     # axtest/import_isolation_test.go
└── imports_test.go                  # MODIFIED — fixtures for importer detection and platform selection

README.md                            # MODIFIED — Public Import Surfaces section + "Testing a command built on ax-go" subsection
AGENTS.md                            # MODIFIED — "seven public packages" → "eight"; axtest listed in Repository Layout
```

**Structure Decision**: Single Go module, public packages at the module root
per Principle X. `axtest` joins the six existing public packages as the
eighth entry in the shared public-surface, apidiff, and doc-coverage
allowlists — organizational isolation (a stable, dedicated, discoverable
package), not size isolation (research.md). Implementation lives directly in
`axtest/` with no `internal/` counterpart, because — unlike `logging` over
`internal/logcore` — there is no swappable-backend concern to wall off; the
package is a thin, honest wrapper over the existing `ax.Execute` and
`ax.Envelope[T]`.

## Complexity Tracking

*No entries. The Constitution Check gate passed with no violation requiring
justification.*
