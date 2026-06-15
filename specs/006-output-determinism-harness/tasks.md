# Tasks: Output-Determinism Test Harness

**Feature**: `006-output-determinism-harness`
**Input**: Design documents from `specs/006-output-determinism-harness/`
**Branch**: `006-output-determinism-harness`

**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅,
contracts/harness-api.md ✅, quickstart.md ✅

**Organization**: Tasks follow TDD discipline (constitution Principle VII): tests
land before implementation, verified to fail before the implementation task runs.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1, US2, US3)
- Each task includes exact file path(s)

---

## Phase 1: Setup

**Purpose**: Create the `internal/testutil` package skeleton so all exported symbol
stubs compile and the package is importable. This unblocks test authoring in Phase 2+.

- [X] T001 Create `internal/testutil/determinism.go` with: package `testutil` doc
  comment; `OutputMode` type; `ModeBoundedJSON` / `ModeNDJSON` / `MaskedSentinel`
  constants; three package-level `var` compiled regex declarations (`reMaskTraceID`,
  `reMaskSpanID`, `reMaskIdempotencyKey`) using `regexp.MustCompile`; and stub
  bodies (`panic("not implemented")`) for all four exported functions
  (`MaskNonDeterministic`, `CompareOutputs`, `ValidateTimestamps`,
  `AssertFullyTyped`) — each with full doc comment per `contracts/harness-api.md`

**Checkpoint**: `go build ./internal/testutil/...` succeeds; all exported symbols
are visible to importers.

---

## Phase 2: Foundational — Core Masking

**Purpose**: `MaskNonDeterministic` is the dependency for every downstream function.
The required `ExampleMaskNonDeterministic` (gated by `make doc-coverage`) is written
first (TDD), then the function is implemented.

⚠️ **Write the example test FIRST and verify it panics before T003.**

- [X] T002 Write `ExampleMaskNonDeterministic` with `// Output:` anchor in
  `internal/testutil/determinism_example_test.go` (must demonstrate all three
  masking replacements in one call; verify test panics before proceeding to T003)
- [X] T003 Implement `MaskNonDeterministic` in `internal/testutil/determinism.go`
  (three sequential regex replacements applied to a new `[]byte` copy; do not
  modify the caller's buffer)

**Checkpoint**: `go test -race ./internal/testutil/...` — `ExampleMaskNonDeterministic`
output matches `// Output:` anchor and passes.

---

## Phase 3: User Story 1 — Bounded JSON Comparison (Priority: P1) 🎯 MVP

**Goal**: Prove that the standard success envelope is byte-identical across two
in-process `run()` invocations after masking the three non-deterministic fields.
Also verify the harness detects a deliberate payload divergence.

**Independent Test**:
`go test -race -run 'TestDeterminismSuccessPath' ./examples/integration/...` passes.

> **Write tests FIRST (T004–T005) and verify they panic before T006.**

- [X] T004 [US1] Write unexported `tbSpy` type (embeds `*testing.T`; overrides
  `Errorf` and `Fatal` to set a boolean flag) and `TestDeterminismSuccessPath`
  (two in-process `run()` calls with pinned `--idempotency-key=test-key
  --format=json`; calls `testutil.CompareOutputs(t, out1.Bytes(), out2.Bytes(),
  testutil.ModeBoundedJSON)`) in `examples/integration/determinism_test.go`
- [X] T005 [US1] Write `TestDeterminismSuccessPathBreakDetection` in
  `examples/integration/determinism_test.go` (two normal `run()` calls; mutate
  second run's output bytes to inject a deliberate divergence; wrap a `tbSpy`
  around `CompareOutputs` call; assert `tbSpy.errorfCalled == true`)
- [X] T006 [US1] Implement `CompareOutputs` for `ModeBoundedJSON` in
  `internal/testutil/determinism.go` (call `t.Helper()`; guard empty inputs with
  `t.Errorf`; mask both slices via `MaskNonDeterministic`; compare with
  `bytes.Equal`; on failure scan for first diverging byte index and report index
  plus up to 80-byte context excerpt via `t.Errorf`)

**Checkpoint**: `go test -race -run 'TestDeterminismSuccessPath' ./examples/integration/...`
— both `TestDeterminismSuccessPath` and `TestDeterminismSuccessPathBreakDetection` pass.

---

## Phase 4: User Story 2 — NDJSON Stream Comparison (Priority: P2)

**Goal**: Extend determinism coverage to multi-line NDJSON `stdout` from the
integration example's `stream` subcommand.

**Independent Test**:
`go test -race -run 'TestDeterminismStream' ./examples/integration/...` passes.

> **Write tests FIRST (T007–T008) and verify they panic before T009.**

- [X] T007 [US2] Write `TestDeterminismStreamPath` in
  `examples/integration/determinism_test.go` (two in-process `run()` calls with
  `stream --count=3 --idempotency-key=test-key --format=json`; calls
  `testutil.CompareOutputs(t, out1.Bytes(), out2.Bytes(), testutil.ModeNDJSON)`)
- [X] T008 [US2] Write `TestDeterminismStreamLineCountMismatch` in
  `examples/integration/determinism_test.go` (two normal stream `run()` calls;
  artificially trim one NDJSON line from second run's bytes; wrap a `tbSpy` around
  `CompareOutputs` call; assert line-count mismatch error was reported)
- [X] T009 [US2] Implement `ModeNDJSON` branch of `CompareOutputs` in
  `internal/testutil/determinism.go` (split each masked slice on `\n`; strip
  trailing empty element; report line-count mismatch first via `t.Errorf`; then
  compare each line pair and report first diverging line index)

**Checkpoint**: `go test -race -run 'TestDeterminismStream' ./examples/integration/...`
— both `TestDeterminismStreamPath` and `TestDeterminismStreamLineCountMismatch` pass.

---

## Phase 5: User Story 3 — Envelope Validation (Priority: P3)

**Goal**: Verify RFC 3339 UTC timestamp format and fully-typed envelope shape for
the integration example's success-path `stdout`.

**Independent Test**:
`go test -race -run 'TestDeterminismTimestamp|TestDeterminismFullyTyped' ./examples/integration/...`
passes.

> **Write tests FIRST (T010–T011) and verify they panic before T012–T013.**

- [X] T010 [US3] Write `TestDeterminismTimestampValidation` in
  `examples/integration/determinism_test.go` (single `run()` call for default
  success command; pass `stdout` bytes to `testutil.ValidateTimestamps(t,
  out.Bytes())`; expect pass — current payload has no timestamp fields, SC-003)
- [X] T011 [US3] Write `TestDeterminismFullyTypedEnvelope` in
  `examples/integration/determinism_test.go` (single `run()` call for default
  success command; call `testutil.AssertFullyTyped[ax.Envelope[helloPayload]](t,
  out.Bytes())`; expect successful unmarshal — `helloPayload` is defined in
  `examples/integration/main.go`)
- [X] T012 [US3] Implement `ValidateTimestamps` in `internal/testutil/determinism.go`
  (call `t.Helper()`; `json.Unmarshal` raw bytes into `any`; define unexported
  recursive `walkAny(t testing.TB, v any)` that switches on `string` /
  `map[string]any` / `[]any`; for strings call `time.Parse(time.RFC3339, v)` —
  if parse succeeds AND zone offset is non-zero, call `t.Errorf`; report unmarshal
  error and return if JSON is invalid)
- [X] T013 [US3] Implement `AssertFullyTyped[T any]` in
  `internal/testutil/determinism.go` (call `t.Helper()`; declare `var target T`;
  call `json.Unmarshal(data, &target)`; on error call `t.Errorf` with unmarshal
  message)

**Checkpoint**: `go test -race ./examples/integration/... ./internal/testutil/...`
— all six new test functions pass with `-race`.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Optional encouraged `ExampleXxx` functions (not gated, but increase
godoc quality), `make doc-coverage` baseline ratchet (gated), and ADR retirement
(constitution governance — FINAL task).

- [X] T014 [P] Add `ExampleCompareOutputs` demonstrating `ModeBoundedJSON` usage
  with two sample byte slices in `internal/testutil/determinism_example_test.go`
- [X] T015 [P] Add `ExampleValidateTimestamps` demonstrating a JSON payload with
  no timestamp fields returning a pass in `internal/testutil/determinism_example_test.go`
- [X] T016 [P] Add `ExampleAssertFullyTyped` demonstrating typed unmarshal into a
  concrete struct in `internal/testutil/determinism_example_test.go`
- [X] T017 Run `make doc-coverage` and update `internal/cmd/doccover/baseline.txt`
  to ratchet the four new primary exported symbols (`MaskNonDeterministic`,
  `CompareOutputs`, `ValidateTimestamps`, `AssertFullyTyped`) — net change must be
  additive (baseline only increases)
- [X] T018 [FINAL] Retire `docs/adr/0011-output-payload-json.md` — decisions are
  absorbed into `specs/006-output-determinism-harness/research.md` ("Decision
  Records Absorbed" section): delete `docs/adr/0011-output-payload-json.md` and
  update every reference (README.md ADR index/links, CONTEXT.md, AGENTS.md, any
  Go doc-comments that cite ADR-0011)

**Checkpoint**: `go test -race ./...` passes; `make doc-coverage` passes with
updated baseline; `docs/adr/0011-output-payload-json.md` no longer exists.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on T001 — stubs must exist for the example
  test to compile and import `internal/testutil`
- **US1 (Phase 3)**: T004–T005 can be written after T001 (stubs compile);
  T006 implementation depends on T003 (`MaskNonDeterministic` must be real)
- **US2 (Phase 4)**: T007–T008 can be written after T001; T009 depends on T006
  (extending the same `CompareOutputs` function)
- **US3 (Phase 5)**: T010–T011 can be written after T001; T012–T013 depend only
  on T001 (stubs already declared); US3 is independently implementable after US1
- **Polish (Phase 6)**: T014–T016 depend on T013 (all implementations complete);
  T017 depends on T014–T016; T018 is the unconditional final task

### User Story Dependencies

- **US1 (P1)**: Can start after Foundational (Phase 2) — no story dependencies
- **US2 (P2)**: Can start after US1 is complete — extends `CompareOutputs`
- **US3 (P3)**: Can start after Foundational (Phase 2) — independent of US2;
  `ValidateTimestamps` and `AssertFullyTyped` do not depend on `CompareOutputs`

### Within Each User Story

1. Write tests → verify they panic or fail for the right reason
2. Implement → make tests pass
3. Verify: `go test -race ./...` is clean

### Parallel Opportunities

With multiple agents in separate worktrees:
- T007–T008 (US2 test authoring) and T010–T011 (US3 test authoring) can be
  drafted concurrently after T001 — both target `examples/integration/determinism_test.go`
  and must be merged before implementing T009 / T012–T013
- T014, T015, T016 are logically independent example additions to the same file;
  an agent can batch all three in one edit pass
- T018 (ADR retirement) has no source-code dependencies beyond all implementation
  being complete — it only touches docs and the ADR file

---

## Parallel Example: User Story 1

```bash
# Write tests (T004 then T005 — same file, sequential):
# examples/integration/determinism_test.go

# Verify panics (stubs not yet implemented):
go test -race -run 'TestDeterminismSuccessPath' ./examples/integration/...
# Expected: panic: not implemented

# Implement (T006):
# internal/testutil/determinism.go — CompareOutputs ModeBoundedJSON branch

# Verify pass:
go test -race -run 'TestDeterminismSuccessPath' ./examples/integration/...
# Expected: PASS
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002–T003)
3. Complete Phase 3: User Story 1 (T004–T006)
4. **STOP and VALIDATE**:
   `go test -race -run 'TestDeterminismSuccessPath' ./examples/integration/...`
5. Bounded JSON determinism coverage is complete and independently testable

### Incremental Delivery

1. Setup + Foundational → `MaskNonDeterministic` + Example compiles and passes (T001–T003)
2. US1 complete → bounded JSON comparison + break-detection functional (T004–T006)
3. US2 complete → NDJSON stream comparison functional (T007–T009)
4. US3 complete → timestamp + typed-envelope validation functional (T010–T013)
5. Polish → examples present, doc-coverage ratcheted, ADR retired (T014–T018)

---

## Notes

- [P] tasks = different files or logically independent additions with no
  dependency on incomplete tasks
- [Story] label maps each task to a specific user story for traceability
- **TDD discipline is mandatory** (constitution Principle VII): write tests first,
  verify they fail, then implement
- `tbSpy` (T004): embed `*testing.T`; override only `Errorf` and `Fatal` to set
  a flag; all other methods delegate to the embedded `*testing.T` — the standard
  Go pattern for testing harness error-reporting behavior
- `AssertFullyTyped[T any]` (T013) uses Go generics — requires Go 1.18+; this
  project targets Go 1.26.4 ✅
- `ExampleMaskNonDeterministic` (T002) is **required** for `make doc-coverage`
  gate — place it in `determinism_example_test.go`, not in Polish; do not defer it
- T018 ADR retirement is gated on research.md having captured all decisions from
  `docs/adr/0011-output-payload-json.md` — this gate is already satisfied (see
  `specs/006-output-determinism-harness/research.md`, "Decision Records Absorbed")
- Run `go test -race ./...` after every phase checkpoint; the race detector is
  mandatory per constitution Principle VII
- No new entries in `go.mod`; all helpers use stdlib only (`bytes`, `encoding/json`,
  `regexp`, `strings`, `time`, `testing`)
