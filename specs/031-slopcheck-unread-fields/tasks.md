---

description: "Task list for the unread struct-field gate (slopcheck)"
---

# Tasks: Unread struct-field gate (slopcheck)

**Input**: Design documents from `specs/031-slopcheck-unread-fields/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/,
quickstart.md

**Tests**: REQUIRED. Constitution VII (test-first) is non-negotiable here.
Every test task precedes the implementation task it covers and must be
observed failing for the right reason before that implementation starts.

**Organization**: by user story (spec.md), so each story is an independently
testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1–US4 from spec.md
- All paths are repository-relative

---

## Phase 1: Setup

- [X] T001 Confirm `golang.org/x/tools` is selected at `v0.50.0` (`go list -m golang.org/x/tools`) and record it. Do **not** edit `go.mod` yet: `go mod tidy` drops a requirement nothing imports. The promotion to a direct requirement happens in T021, at the first import (research R10).
- [X] T002 Create `internal/unreadfield/doc.go` with the package doc comment as a contract: what it classifies, that it is harness-agnostic and stdlib-only, that it keeps no state between calls, and a pointer to the gate (`internal/cmd/slopcheck`) as the enforcing front end.

---

## Phase 2: Foundational (blocks every story)

**Purpose**: the result types and the loader that every story consumes.

- [X] T003 Define `FieldKey`, `Position`, `Finding`, `Result`, `Report`, `Configuration`, `BuildConfigurations()` (the four-configuration policy, research R3) and `AnalysisError` (with `Error`/`Unwrap`) exactly per `data-model.md` and `contracts/core-api.md`, plus the `Finding` cause renderer, in `internal/unreadfield/types.go`.
- [X] T004 [P] Create the `Run` fixture modules, each with its own `go.mod`:
  - `internal/unreadfield/testdata/testonlyread/`: field assigned in `a.go`, read only in `a_test.go`, plus an `export_test.go` alias read from an `a_x_test.go` external test;
  - `internal/unreadfield/testdata/taggedread/`: field read only in a `//go:build ax_no_grpc` file;
  - `internal/unreadfield/testdata/broken/`: a type error.
- [X] T005 [P] Write failing loader tests in `internal/unreadfield/load_test.go`. Over the `testonlyread` module (created in T004), the loader must:
  - select `p [p.test]` instead of `p`;
  - load `p_test [p.test]` as its own unit keyed `p_test`;
  - skip the synthetic `p.test` main;
  - resolve imports through `ImportMap`;
  - fail closed with `*AnalysisError` on the `broken` module (T004);
  - fail closed when output exceeds the byte ceiling (inject a tiny ceiling);
  - `FuzzDecodeListing` over the `go list -json` stream decoder, seeded with a real listing captured from `testonlyread`: never panics, returns units or a wrapped error, honours the ceiling (Constitution VII, research R12).
- [X] T006 Implement the loader (research R2) in `internal/unreadfield/load.go`:
  - run `go list -deps -export -test -json [-tags=…] ./...` via `exec.CommandContext` with `Dir` set and a byte-capped stdout;
  - decode `ImportPath`, `ForTest`, `Name`, `Dir`, `GoFiles`, `CgoFiles`, `Export`, `ImportMap`, `DepOnly`;
  - parse with `parser.ParseComments`;
  - type-check with `importer.ForCompiler(fset, "gc", lookup)`, where `lookup` rewrites through the unit's `ImportMap`, and with `types.Info` populating `Types`, `Defs`, `Uses`, `Selections`;
  - collect the first type error and fail closed;
  - wrap every error with `%w`;
  - after any failed `go list`, wrap `ctx.Err()` when it is non-nil, because a context-killed child returns `*exec.ExitError` ("signal: killed"), not the context error (research R9).

**Checkpoint**: units load per configuration; failures are typed.

---

## Phase 3: User Story 1 — A field nothing reads fails the gate (P1) 🎯 MVP

**Goal**: the core classifies every acceptance shape correctly and `Run`
intersects the results across the four build configurations.

**Independent test**: `go test -race ./internal/unreadfield/` passes, with every
spec acceptance shape covered by its own fixture package.

### Tests for User Story 1 (write first, watch them fail)

- [X] T007 [P] [US1] Create the shared fixture module `internal/unreadfield/testdata/fixtures/go.mod` (`module fixtures`, `go 1.27`, no requirements). It must contain no `_test.go` files (research R12).
- [X] T008 [P] [US1] Add fixture packages for the seven spec acceptance shapes. Each one annotates the field's declaration line with `// want "struct field <Type>.<Field> is assigned but never read"` when the field must be reported, and carries no annotation otherwise. `want` patterns are regular expressions, so escape `{`, `}` and `.` in anonymous-type messages (`struct\{\.\.\.\}`). Because one escaping use marks every field of a type read package-wide, a package that mixes verdicts MUST use a **distinct struct type per verdict** (applies to T008–T010). Fixtures may use `_ = x` to satisfy "declared and not used":
  - `keyed/` (reported)
  - `readloop/` (not reported)
  - `addrof/` (`&x.F`, not reported)
  - `selfassign/` (`x.F = x.F`, reported)
  - `compound/` (`+=` and `++`, not reported)
  - `positional/` (every unread field reported)
  - `storeforms/` (R4 rows: `x.F[i] = v` and `x.F.G = 1` count as reads of `F`, so not reported; `for x.F = range` is a store only, so reported)
  - `promoted/` (embedded field read through the outer type, not reported)

  Files: `internal/unreadfield/testdata/fixtures/<case>/<case>.go`.
- [X] T009 [P] [US1] Add whole-value fixture packages (research R5), each in `internal/unreadfield/testdata/fixtures/<case>/<case>.go`:
  - `escapeeq/` (`==` comparison, not reported)
  - `escapereflect/` (passed to `reflect.DeepEqual`, `fmt.Sprintf`, `json.Marshal`, not reported)
  - `escapeiface/` (sent on `chan any`, stored in `map[string]any`, not reported)
  - `escapenested/` (a nested struct-typed field of an escaping value, not reported)
  - `samepkgcall/` (passed to a same-package function with a concrete parameter that never reads the field, **reported**)
  - `escapecontainer/` (each with its own type: a `[]A` passed to `fmt.Sprintf`, a `map[string]B` sent on a `chan any`, a `map[C]int` passed to `fmt.Sprint` (map key), and a `chan D` passed to an out-of-package function: not reported; `range` over a `[]T` whose rows never read the field: **reported**)
  - `dynamiccall/` (passed to a same-package function through a func value: not reported, per research R5)
  - `assignrhs/` (`x := T{F: 1}`, `y := x`, `tc := tc`, and a value returned by an unexported same-package constructor and assigned to a variable, plus `_ = x`, with `F` never read: **reported**, because RHS-of-assignment is harmless)
- [X] T010 [P] [US1] Add scope fixture packages (research R6) at `internal/unreadfield/testdata/fixtures/<case>/`:
  - `exportedtype/` (exported field of an exported type: skipped)
  - `unexportedtype/` (exported field of an unexported type: reported)
  - `blank/` (`_` field: skipped)
  - `generated/` (field in a `// Code generated ... DO NOT EDIT.` file: skipped)
  - `alias/` (unexported type behind an exported package-level alias: exported fields skipped)
  - `generic/` (unread field of a generic struct: reported once, at the declared field)
  - `anontable/` (the issue's `expectError` table-test shape inside a function body: reported)
- [X] T011 [US1] Write failing table-driven tests in `internal/unreadfield/analyze_test.go`. Load each fixture package with the T006 loader, call `Analyze`, and compare the reported `FieldKey`s against a table derived from the spec. Also assert that `Assigned ⊇ Findings`, that output is sorted, and that two calls give identical results.
- [X] T012 [US1] Write failing `Run` tests in `internal/unreadfield/run_test.go`:
  - `TestBuildTagMatrix`: `BuildConfigurations()` equals the Makefile `BUILD_TAG_MATRIX` (port `deadcheck`'s test). This is the only copy of the policy; the gate imports it;
  - the `testonlyread` module reports nothing;
  - the `taggedread` module reports nothing under the four configurations, but reports the field when run with the default configuration alone (proves the R3 intersection);
  - `broken` returns `*AnalysisError` naming the configuration;
  - an already-expired context returns an error wrapping `context.DeadlineExceeded`;
  - a per-configuration timeout that fires **during** `go list` (a tiny timeout against a fixture large enough to outlast it, or an injected command that sleeps) also returns an error wrapping `context.DeadlineExceeded`, not a plain `*AnalysisError`;
  - `Report.Findings` positions are module-relative.

### Implementation for User Story 1

- [X] T013 [US1] Implement the candidate scope rules (research R6) in `internal/unreadfield/scope.go`: declared-in-unit, not generated (`ast.IsGenerated`), not blank, and the exported-and-externally-visible skip (lexical package-level exported declaration, or the target of an exported alias). Canonicalize through `(*types.Var).Origin()`.
- [X] T014 [US1] Implement `Analyze` in `internal/unreadfield/analyze.go`:
  - assignment sites from keyed and positional composite literals (R7);
  - the R4 load/store table, including promoted-path marking and the self-assignment rule (same field, identical `types.ExprString` base);
  - sorted `Result` output.
- [X] T015 [US1] Implement the whole-value allow-list (research R5) in `internal/unreadfield/escape.go`. Every use outside the allow-list marks all fields of the value's struct type as read, Carriers and recursion follow research R5 exactly: `T` and any pointer, array, slice, map key or element, or channel element type that contains `T`, recursing through nested struct field types with a cycle guard. The blank destination (`_ = x`) is special-cased as harmless before any interface check.
- [X] T016 [US1] Implement `Run` in `internal/unreadfield/run.go`: per-configuration timeout (R9), loop over configurations and units, relativize positions against `dir`, compute the R3 intersection, and union the assignment positions. Check `ctx.Err()` between units.

**Checkpoint**: T011 and T012 pass; US1 is demonstrable through the core.

---

## Phase 4: User Story 2 — The gate speaks the machine contract (P1)

**Goal**: `go run ./internal/cmd/slopcheck` meets `contracts/gate-cli.md`
byte for byte.

**Independent test**: `go test -race ./internal/cmd/slopcheck/` passes its
golden and stream assertions.

### Tests for User Story 2 (write first, watch them fail)

- [X] T017 [P] [US2] Create gate fixture modules: `internal/cmd/slopcheck/testdata/clean/` (every assigned field read), `internal/cmd/slopcheck/testdata/failing/` (two unread fields in one file, which pins sort order), and `internal/cmd/slopcheck/testdata/broken/` (a type error).
- [X] T018 [US2] Write failing tests in `internal/cmd/slopcheck/main_test.go`:
  - `run(ctx, analyze runner, args, stdout, stderr) int`, where `runner` is an injected `func(context.Context, string, []unreadfield.Configuration, time.Duration) (unreadfield.Report, error)` (the `deadcheck` `analyzer` seam). Production passes `unreadfield.Run`. Exercise it with the real runner against `clean`, `failing` and `broken`, byte-compared to `testdata/pass.stdout.golden`, `testdata/fail.stderr.golden` and `testdata/analysis.stderr.golden`, with the opposite stream asserted empty;
  - exit codes per the contract table;
  - invalid-flag and positional-argument cases (`invalid_slopcheck_artifact`, exit 2);
  - error classification through stub runners: timeout → 3, `fs.ErrPermission` → 4, `*AnalysisError` → 2, other → 1, plus precedence (an `*AnalysisError` wrapping `context.DeadlineExceeded` → 3, wrapping `context.Canceled` → 1, wrapping `fs.ErrPermission` → 4), per `contracts/gate-cli.md`;
  - `-dir` pointing at a directory with no `go.mod` → `invalid_slopcheck_artifact`, exit 2;
  - two consecutive runs byte-identical;

### Implementation for User Story 2

- [X] T019 [US2] Implement `internal/cmd/slopcheck/main.go`:
  - the FR-016 header doc comment: what it checks, why `unused`/`structcheck`/ast-grep cannot, what it cannot prove (reflection readers outside allow-listed escapes, other platforms, fields assigned only by statements), the harness decision and its reasoning, the build-configuration rule, exit codes, and `go run ./internal/cmd/slopcheck`;
  - the `-dir` flag and positional rejection;
  - configurations from `unreadfield.BuildConfigurations()`; no local copy;
  - a five-minute per-configuration timeout;
  - the `result` pass document;
  - `emitFailure` via `contract.NewError` with `WithErrorTool("slopcheck")`, `WithErrorVersion(toolVersion())`, `WithErrorExitCode`, `WithRetryable(false)`, `WithErrorCause`, and `WithSuggestions` for findings;
  - `contract.WriteError`.
- [X] T020 [US2] Generate the three golden files with a one-off `-update` test flag, review every byte against `contracts/gate-cli.md`, then confirm T018 passes without `-update`. Files: `internal/cmd/slopcheck/testdata/*.golden`.

**Checkpoint**: the gate runs end to end on fixtures with the exact contract.

---

## Phase 5: User Story 3 — The same check as a standard analyzer (P2)

**Goal**: `analyzer.New()` reports exactly the fixture `// want` set, and agrees
with the core.

**Independent test**: `go test -race ./internal/unreadfield/analyzer/` passes.

- [X] T021 [US3] Write a failing test in `internal/unreadfield/analyzer/analyzer_test.go` that runs `analysistest.Run(t, "../testdata/fixtures", analyzer.New(), "./...")`, so every `// want` from T008–T010 is matched and nothing else is reported. This is the first import of `golang.org/x/tools`: run `go mod tidy` here. It adds `golang.org/x/tools v0.50.0` as a direct requirement and `golang.org/x/mod v0.41.0 // indirect`, plus `go.sum` entries (research R10). Confirm `go mod tidy -diff` is clean and the selected version did not move, so that the test fails because `analyzer.New` is missing, not because `go.mod` needs updating.
- [X] T022 [US3] Write a failing parity test in the same file. Collect `(file, line, message)` from the `analysistest` results, and the same triples from `unreadfield.Run` over the same fixture module with the default configuration. Assert equal sets (FR-012, SC-003).
- [X] T023 [US3] Implement `internal/unreadfield/analyzer/analyzer.go`: `New() *analysis.Analyzer` with name `unreadfield`, a contract doc string, and a `Run` that calls `unreadfield.Analyze(pass.Fset, pass.Files, pass.Pkg, pass.TypesInfo)` and reports each finding with `pass.Reportf` at the declaration position. No classification logic (FR-011).

**Checkpoint**: both front ends agree on every fixture.

---

## Phase 6: User Story 4 — The gate is enforced (P2)

**Goal**: `make slop-check` exists and runs in `make ci` and the CI `validate`
job. This phase **starts with a stop point**.

- [X] T024 [US4] **STOP POINT (research R13, spec Assumptions)**: run `go run ./internal/cmd/slopcheck` from the repository root and record the finding count and a verdict for each finding (real defect, or false positive with its reason) in a scratch note for the PR description (FR-018).
  - **Zero findings**: continue.
  - **False positives only**: they are core defects. Add a fixture reproducing each to T008–T010, fix the core, re-run T024.
  - **Any real finding**: halt implementation and ask the maintainer how to proceed before T025. Do not fix it here, and do not weaken the gate.
- [X] T025 [US4] Add a `slop-check` target to `Makefile` (`@go run ./internal/cmd/slopcheck`, beside `dead-check`), add it to the `ci:` prerequisites, add help text matching the `dead-check` line, and add `slop-check` to the `ci` help line that lists its members.
- [X] T026 [US4] Add a `make slop-check` step to the `validate` job in `.github/workflows/ci.yml`, right after the `make dead-check` step, with a one-line comment that slopcheck owns the four-tag intersection and counts tests as readers. Then run `actionlint -shellcheck= .github/workflows/ci.yml` locally (snap `shellcheck` hangs here; CI runs the full check).
- [X] T027 [US4] SC-002 manual check: plant a throwaway `expectError bool` field in a real table test and confirm `make slop-check` exits 2 and names it. Remove it, confirm a pass, and record the result for the PR description.

---

## Phase 7: Polish & cross-cutting

- [X] T028 [P] Document the gate in `AGENTS.md`. Add an "Unread Field Gate" subsection under Development Workflow, beside "Internal Reachability Gate", covering what it checks, the stream and exit contract, the four-configuration intersection, that tests count as readers, the no-allowlist policy, and `make slop-check`. State in one line how it differs from `make slop`: that is the non-blocking, type-free ast-grep report, and `make slop-check` is the blocking, type-aware gate.
- [X] T029 [P] Add a pre-flight checklist item after the `make dead-check` item in `CONTEXT.md`. Add a bullet beside the "Internal reachability" bullet in `README.md`. Add `make slop-check` beside `make dead-check` in the gate list at `CONTRIBUTING.md:82`.
- [X] T030 Run the full gate set and fix anything red: `make validate`, `make test`, `golangci-lint run` under all four build-tag combinations (`make lint` dies at actionlint here), `make cover-check` (new packages clear the 25% default floor), `make doc-coverage`, `make surface-check` (baseline must be unchanged, FR-013), `make size-check` (unchanged, SC-007), `make dead-check` (every new internal function reachable), `make security`, `make slop-check`, `npx markdownlint-cli2` on every changed Markdown file.
- [X] T031 Draft `PR_MESSAGE.md` as a Conventional Commit:
  - subject `feat(gates): add slopcheck unread struct-field gate`;
  - body with the harness decision, the T024 finding count and per-finding triage, the T027 manual result, and the `x/tools` justification;
  - `Implements specs/031-slopcheck-unread-fields/` and `Closes #232`.

  Validate it with `cat PR_MESSAGE.md | npx commitlint`.

- [X] T032 Remediate the PR #286 review. It supersedes the rules recorded in T013–T015 and the loader flags above; research R2, R4, R5 and R6 are authoritative. Each fix lands behind a fixture in `internal/unreadfield/testdata/fixtures/` that failed first:
  - physical positions, ignoring `//line` (`linedirective/`);
  - promoted method calls read the embedded field (`promotedmethod/`);
  - self-assignment exempt only for a call- and receive-free base (`effectfulbase/`);
  - one same-field-object copy rule at every copy site (`anoncopy/`, which also covers `genericparam/`);
  - `append` judged by element type, `delete` keys escape (`builtins/`);
  - function signatures carry structs, and the callee position is harmless (`funcvalue/`);
  - exposure by reachability from exported declarations (`aliasforms/`);
  - `go list -e`, so an unreadable source file returns a typed `fs.ErrPermission` and exits `4` (`load_test.go`, `main_test.go`).

---

## Dependencies & execution order

- **Setup (T001–T002)** → **Foundational (T003–T006)** → every story.
- T004 (the `Run` fixture modules) sits in Foundational because the loader tests (T005) and the `Run` tests (T012) both need it.
- **US1 (T007–T016)** blocks US2 (the gate consumes `Run`) and US3 (the adapter consumes `Analyze` and the shared fixtures).
- **US2** and **US3** are independent of each other after US1.
- **US4** needs US2. T024 is a hard gate in front of T025–T026.
- **Polish** runs last; T030 must be green before T031. T032 follows PR review and reruns T030's gates.

Within each story, tests come before implementation, and implementation does
not start until its tests have failed for the right reason.

## Parallel opportunities

- Phase 2: T004 and T005 alongside T003.
- US1 fixtures: T007, T008, T009 and T010 touch disjoint directories.
- After US1: US2 (T017–T020) and US3 (T021–T023) in parallel.
- Polish: T028 and T029 in parallel.

## Implementation strategy

1. **MVP = Phases 1–3 (US1)**: the core is correct on every acceptance shape
   and intersects configurations. This is where the issue's correctness risk
   lives.
2. Add US2: the gate contract, the thing CI consumes.
3. Add US3: the analyzer adapter and parity.
4. US4 behind the T024 stop point, then polish.

## Format validation

All 32 tasks use `- [ ] T### [P?] [US?] description with path`. Story labels
appear only in Phases 3–6. Setup, Foundational and Polish tasks carry none.
