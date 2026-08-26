---
description: "Task list for 019-axtest-package"
---

# Tasks: axtest — Full-Lifecycle Command Test Helper

**Input**: Design documents from `/specs/019-axtest-package/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/axtest-package.md, quickstart.md

**Tests**: Test tasks ARE included and are mandatory — plan.md's Constitution Check marks Principle VII (Test-First Discipline) **PASS (binding)**, and every functional requirement in spec.md maps to a test assertion below. Every test task lands before the implementation task it constrains and must fail for the right reason first (a compile error for a not-yet-implemented function, or a named gate failure for a not-yet-registered doc-coverage requirement — never an unrelated failure).

**Organization**: Grouped by user story so each is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no unresolved same-phase dependency)
- **[Story]**: `[US1]`–`[US3]` map to spec.md user stories
- Exact file paths included in every task

## Path Conventions

Go library at the module root (`github.com/rshade/ax-go`). The new public package `axtest` lives at `/axtest`, joining `config`, `contract`, `id`, `logging`, `mcp`, `schema` at the module root (no `pkg/` or `src/`). Its own tests live in the external test package `axtest_test`, matching the `logging_test`/`mcp_test` convention.

**Governing ADR**: none (plan.md — no ADR addresses test tooling or execution-lifecycle testing). **No ADR-retirement task is generated.**

**Build configurations**: `axtest` adds no build-tag-conditional code (research.md), so no per-configuration task loop is needed the way the logging feature required. It is still exercised by the existing `make test`/`make validate` four-configuration matrix with no feature-specific addition.

**Shared toy command fixture**: Several stories need a small confirmation-gated, dry-run-aware command tree to exercise `axtest` against. T005 mirrors the proven pattern already in this repository's own `execute_test.go` (`TestExecuteApprovalAndDryRunAreOrthogonal`: `ax.Confirm`, `DryRunFromContext`, `--format=json`) rather than inventing a new one, and every later story reuses it.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the new package root so test files have something to compile against.

- [X] T001 Create `axtest/doc.go` declaring `package axtest` with a package doc comment stating: this package is designed to be imported only from `_test.go` files; it depends on the full root `ax` package and Cobra without restriction (organizational isolation, not size isolation — research.md); and a non-test import anywhere else in this module is caught automatically (names the enforcement mechanism built in Phase 2, FR-009)

**Checkpoint**: `axtest` exists and compiles empty; `go build ./...` is green.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the FR-009 test-only enforcement mechanism and the shared test fixture every story's tests depend on. Both are infrastructure no single user story owns, and both must exist before any story's tests can be written meaningfully.

**⚠️ CRITICAL**: No user story test-writing can begin until T005 (the shared fixture) lands; the FR-009 guard (T002–T004) has no ordering dependency on the fixture and can proceed in parallel with it.

### Tests for Foundational (write first, verify they fail for the right reason)

- [X] T002 [P] Write fixture-based unit tests in `internal/testutil/imports_test.go` for a not-yet-existing pure function `FindNonTestImporters(packages []testutil.ModulePackage, targetImportPath string) []string`: given literal `[]ModulePackage` fixtures — one package whose `Imports` field contains the target (a violation) and one package that has no reference at all — assert only the violator is reported. Add a throwaway module with production imports in `_windows.go` and `_arm64.go` files and assert `ResolveModulePackages` selects them only for matching `BuildProfile` values. Each test must fail to compile first because its function or profile type does not exist yet

### Implementation for Foundational

- [X] T003 Implement the `internal/testutil/imports.go` additions that make T002 pass: `type ModulePackage struct { ImportPath string; Imports []string }`; `type BuildProfile struct { GOOS string; GOARCH string }`; `func ResolveModulePackages(ctx context.Context, moduleDir string, profile BuildProfile, tags ...string) ([]ModulePackage, error)`, which runs `go list -json ./...` from `moduleDir` with `GOOS`, `GOARCH`, and `CGO_ENABLED=0` pinned and passes `-tags=<joined tags>` when tags are non-empty, then decodes the concatenated JSON objects with `json.NewDecoder` in a loop, keeping only `ImportPath`/`Imports`; `FindNonTestImporters`; and a profile-aware `AssertNoProductionImport` wiring them together with `t.Helper()` and `t.Errorf` naming every violating package. Deliberately read only each package's plain `Imports` field, never `TestImports`/`XTestImports`, so Go itself separates test-file imports from production ones
- [X] T004 [P] Write `axtest/import_isolation_test.go` (package `axtest_test`) with the exhaustive four build configurations and six supported GOOS/GOARCH profiles mirrored from `internal/cmd/surfacecheck.DefaultProfiles`. `TestAxtestIsOnlyImportedFromTests` loops over all 24 combinations and calls `testutil.AssertNoProductionImport` with each profile and tag set. A host-only check would miss imports in platform-specific production files such as `_windows.go`; a default-tag-only check would miss `ax_no_grpc`/`ax_no_otlp` files. This standing regression guard stays green only while FR-009 holds everywhere in the supported matrix
- [X] T005 [P] Write `axtest/testhelpers_test.go` (package `axtest_test`) implementing `newGreetCommand(t testing.TB) *cobra.Command`: a toy root command with one confirmation-gated subcommand that calls `ax.Confirm(cmd.Context(), "greet")`, checks `ax.DryRunFromContext(cmd.Context())`, and on success writes `ax.NewEnvelope(cmd.Context(), greetResult{Approved: bool, DryRun: bool})` as JSON to `cmd.OutOrStdout()` — mirroring `execute_test.go`'s `TestExecuteApprovalAndDryRunAreOrthogonal` fixture exactly rather than inventing new confirmation-gating logic. Returns a **fresh** `*cobra.Command` tree on every call (never a shared package-level variable — Principle X, no mutable package-level state), so each test gets an unmounted tree

**Checkpoint**: `axtest` compiles with its test-only enforcement live and a shared fixture ready; `go test -race ./axtest/... ./internal/testutil/...` is green. User story work can now begin.

---

## Phase 3: User Story 1 - Exercise the real startup lifecycle in a test (Priority: P1) 🎯 MVP

**Goal**: `axtest.Run` executes a command tree through the identical `ax.Execute` lifecycle a production binary uses and returns a `Result{Stdout, Stderr, ExitCode}`.

**Independent Test**: Run the toy command from T005 through `axtest.Run` with `--dry-run`, with a blocked confirmation, and with an approved confirmation; confirm each returns the exit code and stream separation the spec's acceptance scenarios describe — using only `axtest.Run` and `encoding/json` (not `axtest.Decode`, which is US2's deliverable), so this story is genuinely testable on its own.

### Tests for User Story 1 (write first, verify they fail for the right reason)

- [X] T006 [P] [US1] Write `axtest/run_test.go` (package `axtest_test`) as a table-driven test against `newGreetCommand`, decoding `Result.Stdout` with plain `encoding/json` (not `axtest.Decode`) to keep this story independent of US2. Cover: (1) `--dry-run` alone → `ExitCode == 0`, decoded `DryRun == true`; (2) confirmation-gated action with no `--yes` → `ExitCode == 2` (`ExitValidation`), `Stderr` contains `"confirmation_required"`, `Stdout` empty (spec Acceptance Scenario 2, Principle I); (3) confirmation-gated action with `--yes` → `ExitCode == 0`, decoded `Approved == true` (Acceptance Scenario 3); (4) no `--idempotency-key` supplied → run still succeeds and the envelope's `meta.idempotency_key` (decode the full `ax.Envelope[greetResult]`, not just `.Data`, to reach `Meta`) is non-empty, proving auto-generation occurred (Acceptance Scenario 4, FR-004); (5) an unknown subcommand/arg produces a non-zero exit code and `Run` returns normally rather than failing the test process itself (Acceptance Scenario 5, FR edge case). This file must fail to compile first — `axtest.Run` does not exist yet
- [X] T007 [P] [US1] In the same file, add `TestRunSupportsRepeatedInvocationOnSameTree`: call `axtest.Run` twice against **one** `newGreetCommand(t)` tree (first with `--dry-run`, then with `--yes`), asserting the second call still recognizes every agent-safety flag (no "unknown flag" error) and both calls return independently correct results — the FR-008 regression guard for flag re-mounting safety, which `Run` gets for free by delegating to `ax.Execute`'s existing `cli.EnsurePersistentXFlag` calls (research.md) but must still be asserted rather than assumed
- [X] T007a [P] [US1] In the same file, add `TestRunIsSafeForConcurrentUse`: launch several (e.g. 8) `t.Run` subtests via `t.Parallel()`, each constructing its **own** `newGreetCommand(t)` tree and calling `axtest.Run` with a distinct arg set (a mix of `--dry-run` and `--yes` cases), asserting every subtest's result is independently correct. This is the regression guard for spec.md's Edge Case "multiple subtests run concurrently, each against its own command tree... must not rely on any shared or global state" — `go test -race` only catches a race in a code path a test actually exercises concurrently, and no other task exercises concurrent `Run` calls

### Implementation for User Story 1

- [X] T008 [US1] Implement `axtest/run.go`: `type Result struct { Stdout, Stderr []byte; ExitCode int }` and `func Run(ctx context.Context, t testing.TB, root *cobra.Command, args []string, opts ...ax.ExecuteOption) Result` per contracts/axtest-package.md — sets `root`'s args, allocates its own `bytes.Buffer` pair, calls `ax.Execute(ctx, root, append(opts, ax.WithStdout(&stdout), ax.WithStderr(&stderr))...)` (its own capture options appended **last** so they win regardless of what a caller passed in `opts`, and this precedence is stated in the doc comment; `ctx` is forwarded from the caller, never `context.Background()` internally — Constitution Principle X, research.md), calls `t.Helper()`, and never calls `t.Fatal`/`t.Error` itself regardless of the returned exit code. Makes T006, T007, and T007a pass
- [X] T009 [US1] Register the new package with `doccover`: in `internal/cmd/doccover/main.go`, add an `axtestPackageAlias = "axtest"` constant, add it to `scannedPackages()`, and add `"axtest.Run"` to `requiredSymbols()`. Run `go run ./internal/cmd/doccover` and confirm it now fails, naming `axtest.Run` as missing a verified example — the right kind of red, since T008 already makes `Run` a real exported symbol with no example yet
- [X] T010 [P] [US1] Write `axtest/example_test.go` (package `axtest_test`) with `ExampleRun`, using `newGreetCommand` and a `// Output:` comment asserting the printed exit code and a decoded field. Run `go run ./internal/cmd/doccover` again and confirm it now passes (green)
- [X] T011 [US1] Add `rootImportPath + "/axtest"` to `PublicPackages()` in `internal/cmd/surfacecheck/inventory.go` (keep the list sorted) and `"github.com/rshade/ax-go/axtest"` to `allowedPackages()` in `internal/cmd/apidiff-verdict/main.go`, in the same change — the two allowlists are duplicated by design and guarded by a test that parses one and compares, so updating only one fails the `check-packages` guard. Update the doc comment at `internal/cmd/surfacecheck/main.go:14` and `:19` from "seven public packages"/"a seventh public package" to "eight"/"an eighth public package", explicitly noting the 24-load count is unchanged (a load is a configuration × profile combination, independent of package count)
- [X] T012 [US1] Run `make surface-update` and review every line of `git diff internal/cmd/surfacecheck/baseline.json`: expect new entries for `axtest.Result` (and its three fields) and `axtest.Run`, each with the `"all"` presence sentinel for both `configurations` and `profiles` (no build-tag- or platform-specific behavior). Treat anything else as unreviewed drift
- [X] T013 [US1] Validate the checkpoint: `gofmt -s -l .`, `go build ./...`, `go vet ./...`, `go test -race ./axtest/...`, `golangci-lint run`, `make doc-coverage`, and `make surface-check` all green for the `Run`-only surface

**Checkpoint**: User Story 1 is fully functional and independently testable — `axtest.Run` exercises the real lifecycle, is documented, and is gated.

---

## Phase 4: User Story 2 - Decode a command's result without a wrapper struct (Priority: P2)

**Goal**: `axtest.Decode[T]` unwraps `ax.Envelope[T]`'s `Data` field into a caller-supplied type, failing the test immediately on a shape mismatch.

**Independent Test**: Decode a literal, hand-built envelope `[]byte` fixture into a plain caller-defined type using only `axtest.Decode`, with no intermediate wrapper type declared — this does not require `axtest.Run` to exist, keeping the story independently testable.

### Tests for User Story 2 (write first, verify they fail for the right reason)

- [X] T014 [P] [US2] Write `axtest/decode_test.go` (package `axtest_test`). Primary cases use a literal JSON `[]byte` fixture (e.g. `` []byte(`{"data":{"greeting":"hi"},"meta":{"trace_id":"..."}}`) ``), not `axtest.Run`'s output, so this story stays independently testable: (1) a well-formed envelope decodes to the expected typed value with no wrapper struct declared in the test; (2) malformed JSON, and separately a JSON object missing the expected shape, both must fail the calling test immediately with a message naming the cause. For case (2), use a fake `testing.TB` that records `Helper()`/`Fatalf()` calls instead of exiting the goroutine — the standard technique for testing a helper that calls `t.Fatalf`, since a real `*testing.T`'s `Fatalf` cannot be observed without killing the test. This file must fail to compile first — `axtest.Decode` does not exist yet

### Implementation for User Story 2

- [X] T015 [US2] Implement `axtest/decode.go`: `func Decode[T any](t testing.TB, stdout []byte) T` per contracts/axtest-package.md — `t.Helper()`, `json.Unmarshal(stdout, &envelope)` into a local `ax.Envelope[T]`, `t.Fatalf` with the parse or shape-mismatch cause on error, otherwise return `envelope.Data`. Makes T014 pass
- [X] T016 [US2] In `internal/cmd/doccover/main.go`, add `"axtest.Decode"` to `requiredSymbols()`. Confirm `go run ./internal/cmd/doccover` now fails naming `axtest.Decode` as missing an example — the right reason, since T015 already made it a real exported symbol
- [X] T017 [P] [US2] Write `ExampleDecode` in `axtest/example_test.go` with a `// Output:` comment. Confirm `go run ./internal/cmd/doccover` now passes (green)
- [X] T018 [US2] Run `make surface-update` and review the additive `git diff internal/cmd/surfacecheck/baseline.json` entry for the new generic function `axtest.Decode`; confirm `go run ./internal/cmd/apidiff-verdict check-packages` still agrees the two allowlists match
- [X] T019 [US2] Add an explicit cross-story integration case to `axtest/decode_test.go`: decode the real `Result.Stdout` produced by `axtest.Run(context.Background(), t, newGreetCommand(t), []string{"greet", "--yes"})` (US1) with `axtest.Decode[greetResult]` (US2), asserting the two stories compose correctly end to end — this is validation that the two already-independent stories integrate, not a dependency of either one's own tests
- [X] T020 [US2] Validate the checkpoint: `go test -race ./axtest/...`, `golangci-lint run`, `make doc-coverage`, `make surface-check` all green for the `Run` + `Decode` surface

**Checkpoint**: User Stories 1 AND 2 both work independently, and compose correctly together.

---

## Phase 5: User Story 3 - Assert the common case in one step (Priority: P3)

**Goal**: `axtest.RunAndDecode[T]` composes `Run` and `Decode` for the success-path common case, with no independent logic that could drift from either.

**Independent Test**: Run a command known to succeed through `RunAndDecode` and confirm it returns the same typed result and exit code as calling `Run` then `Decode` separately.

### Tests for User Story 3 (write first, verify they fail for the right reason)

- [X] T021 [P] [US3] Write `axtest/runanddecode_test.go` (package `axtest_test`): `TestRunAndDecodeMatchesRunThenDecode` calls both `axtest.RunAndDecode[greetResult](context.Background(), t, newGreetCommand(t), []string{"greet", "--yes"})` and the two-step equivalent (a second, separately-constructed tree via `Run` + `Decode`, called with the same `context.Background()`) and asserts identical typed values and exit codes (spec Acceptance Scenario 1). This file must fail to compile first — `axtest.RunAndDecode` does not exist yet

### Implementation for User Story 3

- [X] T022 [US3] Implement `func RunAndDecode[T any](ctx context.Context, t testing.TB, root *cobra.Command, args []string, opts ...ax.ExecuteOption) (T, int)` in `axtest/decode.go` (or a new `axtest/runanddecode.go` if that keeps `decode.go` focused) as `t.Helper()` followed literally by `result := Run(ctx, t, root, args, opts...); return Decode[T](t, result.Stdout), result.ExitCode` — no independent logic beyond the `t.Helper()` call, per the contract. The `t.Helper()` call is required, not optional: without it, a `Decode` failure raised through this composition reports its `t.Fatalf` location inside `axtest/decode.go` instead of the caller's test line, because `testing.Helper()` only skips *consecutively*-marked frames walking up from the failure site (contracts/axtest-package.md). Doc comment states it is intended for the success path; a caller expecting a non-zero exit code should use `Run` directly. Makes T021 pass
- [X] T023 [US3] In `internal/cmd/doccover/main.go`, add `"axtest.RunAndDecode"` to `requiredSymbols()`. Confirm `go run ./internal/cmd/doccover` now fails naming it as missing an example — the right reason, since T022 already made it a real exported symbol
- [X] T024 [P] [US3] Write `ExampleRunAndDecode` in `axtest/example_test.go` with a `// Output:` comment. Confirm `go run ./internal/cmd/doccover` now passes (green) for all three required symbols
- [X] T025 [US3] Run a final `make surface-update` and review `git diff internal/cmd/surfacecheck/baseline.json` for the complete surface (`Result`, `Run`, `Decode`, `RunAndDecode`); confirm `go run ./internal/cmd/apidiff-verdict` reports no breaking change to any existing package (only additive `axtest` entries) and `check-packages` still agrees
- [X] T026 [US3] Validate the checkpoint across the complete surface: `gofmt -s -l .`, `go build ./...`, `go vet ./...`, `go test -race ./axtest/... -v`, `golangci-lint run`, `make doc-coverage`, `make surface-check`

**Checkpoint**: All three user stories are independently verified and compose correctly; `axtest`'s complete public surface exists, is tested, documented, and gated.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: The real-CLI demonstration, coverage calibration, documentation, and release hygiene (FR-010, SC-005).

- [X] T027 [P] Write `examples/integration/axtest_example_test.go` — a test exercising the shipped reference command (not the toy `newGreetCommand`) through `axtest.Run`/`axtest.RunAndDecode`, per research.md's two-example decision: this is the "canonical testing pattern" demonstration against a real command tree, distinct from `axtest/example_test.go`'s doccover-gated toy examples
- [X] T028 [P] Add a per-package coverage floor for `github.com/rshade/ax-go/axtest` to `defaultFloorConfig()` in `internal/cmd/covercheck/main.go`, calibrated ~2pp below its measured coverage after T001–T027 land, following the convention used when `internal/cli`/`internal/mcp`/`internal/schema` enrolled on 2026-07-17. Record the measured percentage and the chosen floor in the commit message
- [X] T029 [P] Update `README.md`: add `axtest` to `## Public Import Surfaces` (it is not one of the size-motivated isolated packages, so introduce it in its own short paragraph rather than the "Choosing a surface" size table — it answers "how do I test a command built on ax-go", not "how small can my binary be"), and add the quickstart.md walkthrough as a "Testing a command built on ax-go" subsection. If `documentation_test.go` asserts the set of documented public import paths, extend it to include `axtest` so this stays verified rather than drifting
- [X] T030 [P] Update `AGENTS.md`: add `axtest` to the Repository Layout's list of approved public packages (noting it is organizationally isolated — test-only by convention and an enforced check — rather than size-isolated); update the Public Surface Gate section's "seven public packages" to "eight" while explicitly keeping the 24-load-count language unchanged; add the T028 floor to the Coverage Policy table
- [X] T031 [P] Update `CONTEXT.md` and `ROADMAP.md` if either enumerates the public package list or tracks issue #178; link `specs/019-axtest-package/`
- [X] T032 [P] Add an `axtest` page to the `docs/` Starlight site under the appropriate Diátaxis quadrant (a how-to guide: "Test a command built on ax-go"), adapted from quickstart.md; run markdownlint on it
- [X] T033 [P] Run markdownlint across every changed Markdown file (`README.md`, `AGENTS.md`, `CONTEXT.md`, `ROADMAP.md`, `docs/`, and this spec directory) and fix all findings
- [X] T034 Run the complete CI gate set across all four build configurations: `make test`, `make validate`, `make lint`, `make doc-coverage`, `make cover-check`, `make surface-check`. `make bench-check` and `make size-check` are unaffected by this feature (research.md: no benchmark claim, no size gate applies) and need no new passing evidence beyond staying green as they already are
- [X] T035 Execute `specs/019-axtest-package/quickstart.md` end to end as written — every code snippet compiles and behaves as documented against a real command tree — and correct any snippet that does not work verbatim
- [X] T036 Write `PR_MESSAGE.md` as a Conventional Commit (`feat(axtest): ...`) describing the new package, why it is organizationally rather than size isolated, the FR-009 reverse-import enforcement, and the three allowlist updates; validate with `cat PR_MESSAGE.md | npx commitlint`. Do **not** hand-edit `CHANGELOG.md` — release-please owns it

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — can start immediately
- **Foundational (Phase 2)**: depends on Setup; **blocks all user stories** — the FR-009 guard must exist before any story's tests add real usage, and the shared fixture (T005) is what every story's tests are written against
- **US1 (Phase 3)**: depends on Foundational only — the MVP
- **US2 (Phase 4)**: depends on Foundational; independently testable via a literal fixture, but T019's integration case additionally needs `axtest.Run` from US1
- **US3 (Phase 5)**: depends on Foundational; needs both `Run` (US1) and `Decode` (US2) to exist, since it composes them with no independent logic
- **Polish (Phase 6)**: depends on all three stories. No ADR-retirement task — plan.md establishes this feature is governed by no ADR

### Within Each Phase

- Every test task precedes the implementation task it constrains and must be observed failing for the right reason first (Principle VII, binding)
- T002 → T003 (test before the function it specifies)
- T004 and T005 depend only on T001, not on each other or on T002/T003 — independent files
- T006/T007/T007a → T008 (tests before `Run`); T008 → T009 (the symbol must exist and be exported before doccover can correctly report "missing example" rather than "no longer exported"); T009 → T010 (register before writing the example that satisfies it)
- T011 and T012 both touch `internal/cmd/surfacecheck/` and `internal/cmd/apidiff-verdict/`; T011 (allowlist edit) must precede T012 (baseline regeneration)
- T014 → T015 (tests before `Decode`); T015 → T016 (the symbol must be real and exported before doccover can correctly report "missing example"); T016 → T017 (register before writing the example that satisfies it)
- T021 → T022 (tests before `RunAndDecode`); T022 → T023 (same reason as above); T023 → T024
- `internal/cmd/doccover/main.go`'s `requiredSymbols()` is edited three times (T009, T016, T023) — these are necessarily sequential on one shared file, in story order, and each edit is a small additive line, not a rewrite
- T028 must follow T026 (coverage cannot be measured meaningfully until the full surface's tests exist)

### Parallel Opportunities

- T002, T004, T005 are three independent files and can be written concurrently once T001 lands
- T006, T007, and T007a are the same file (sequential within it, but independent of T004/T005 once the fixture exists)
- T010, T017, T024 (the three `ExampleXxx` additions) each touch the same `axtest/example_test.go` file across different stories — treat as sequential on that file, in story order
- T027–T033 are independent documentation and process tasks

---

## Parallel Example: Foundational Test Wave

```bash
# Three independent files, one agent each, once T001 lands:
Task: "Write internal/testutil/imports_test.go fixture cases for FindNonTestImporters"
Task: "Write axtest/import_isolation_test.go (FR-009 standing guard)"
Task: "Write axtest/testhelpers_test.go (shared newGreetCommand fixture)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational — the FR-009 guard and shared fixture, both cheap
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: `axtest.Run` exercises dry-run and confirmation-gated behavior correctly, is documented, and is gated — this alone already fixes the production friction the issue opened with
5. `axtest` is shippable at this point as a preview, but SC-002 (zero hand-declared wrapper types) is not yet delivered without US2

### Incremental Delivery

1. Setup + Foundational → package exists, test-only enforcement live, shared fixture ready
2. US1 → `Run` works, is documented, is gated (**MVP**)
3. US2 → `Decode` removes the wrapper-struct chore; composes with US1's real output
4. US3 → `RunAndDecode` removes the two-call chore for the common case
5. Polish → real-CLI demonstration, coverage floor, documentation, PR message

### Parallel Team Strategy

Foundational is small and mostly parallel (T002/T004/T005). Once its checkpoint is green, US1 must land before US2's integration case (T019) and before US3 can exist at all (US3 composes both). A single-developer sequence (US1 → US2 → US3) is the realistic path here; the three-story split exists for independent testability and incremental delivery, not for parallel staffing.

---

## Notes

- `[P]` = different files, no unresolved same-phase dependency
- `[Story]` maps a task to a spec.md user story for traceability
- Verify every test fails for the right reason before implementing
- Commit after each task or logical group; never hand-edit `CHANGELOG.md`
- `internal/cmd/doccover/main.go`'s `requiredSymbols()` additions (T009, T016, T023) must each follow — not precede — the symbol they name becoming a real exported identifier, or doccover reports the wrong failure reason ("no longer exported" instead of "missing example")
- T011 (allowlist edit) and the corresponding `check-packages` guard must land together, exactly as the precedent in `specs/017-import-isolated-logging/tasks.md` (T032/T033) records
