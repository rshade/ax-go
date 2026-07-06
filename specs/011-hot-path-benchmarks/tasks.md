---

description: "Task list for feature: Hot-Path Benchmarks with -benchmem"
---

# Tasks: Hot-Path Benchmarks with `-benchmem`

**Input**: Design documents from `/specs/011-hot-path-benchmarks/`

**Prerequisites**: plan.md (required), spec.md (required), research.md,
data-model.md, quickstart.md

**Tests**: This feature's deliverable IS a `testing.B` suite — the benchmarks
are the test artifacts (Constitution Principle VII: `testing.B` with `-benchmem`
for any allocation claim). No *separate* test tasks are generated; the benchmark
functions themselves are written and verified by the user-story tasks below.

**Organization**: Tasks are grouped by user story (P1→P3 from spec.md) so each
story is an independently runnable benchmark increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths included in each description

## Path Conventions

Single Go library; root package `ax`. The entire implementation is one new
test-only file at the module root: `logger_bench_test.go` (package `ax`,
mirroring the existing `config_bench_test.go`). Because all benchmark functions
live in that one file, story tasks that edit it are **sequential, not [P]**
(same-file conflict). Parallelism appears only in the Polish phase across
different files.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the benchmark file scaffold.

- [X] T001 Create `logger_bench_test.go` at the repo root in `package ax` with
  the imports the suite needs (`context`, `io`, `testing`, and
  `oteltrace "go.opentelemetry.io/otel/trace"`), matching the import/style of
  `config_bench_test.go`. File compiles empty (no benchmark bodies yet).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared benchmark fixtures every variant needs. MUST complete before
any user story.

**⚠️ CRITICAL**: No story benchmark can be written until these helpers exist.

- [X] T002 In `logger_bench_test.go`, add the shared, allocation-free-at-call
  fixtures used by all variants: (a) a helper that builds an `ax.Logger` via
  `NewLogger(ctx, WithLoggerWriter(io.Discard), ...)` so every variant discards
  output (FR-007, research Decision 3); (b) a package-level pair of decoded
  span/trace IDs built once with `oteltrace.TraceIDFromHex`/`SpanIDFromHex` plus
  a helper returning a context carrying a populated `SpanContext` via
  `oteltrace.ContextWithSpanContext(ctx, oteltrace.NewSpanContext(...))` —
  decoded OUTSIDE any `b.Loop()` so decode cost is excluded (research Decision 4,
  pattern from `config_test.go:558`). Keep helpers unexported.

**Checkpoint**: Foundation ready — story benchmarks can now be written.

---

## Phase 3: User Story 1 - Substantiate the logger allocation claim (Priority: P1) 🎯 MVP

**Goal**: Produce a measured per-operation allocation profile for the standard
enabled-level emit path and the disabled-level (filtered) fast path.

**Independent Test**: `go test -run '^$' -bench 'BenchmarkLoggerEmit' -benchmem ./...`
prints `B/op` and `allocs/op` for both `enabled/no_fields` and `disabled_level`,
reported separately (spec FR-003, FR-005, SC-001, SC-002).

### Implementation for User Story 1

- [X] T003 [US1] In `logger_bench_test.go`, add `func BenchmarkLoggerEmit(b *testing.B)`
  as a table-driven benchmark (sub-cases `enabled/no_fields` and
  `disabled_level`) using `b.Run(name, ...)`, `b.ReportAllocs()`, and `b.Loop()`.
  `enabled/no_fields`: `InfoLevel` logger, `Info(context.Background()).Msg(...)`.
  `disabled_level`: `InfoLevel` logger, `Debug(context.Background()).Msg(...)`
  (filtered fast path). Both use the discard-sink helper from T002. Carry a
  doc comment on the function stating it substantiates the enabled vs.
  filtered hot-path allocation profile (FR-003/FR-005; FR-011 doc convention).

**Checkpoint**: US1 benchmark runs and reports distinct profiles for the enabled
and disabled paths — the MVP that backs the headline claim.

---

## Phase 4: User Story 2 - Isolate the tracing-hook cost (Priority: P2)

**Goal**: Make the always-on tracing hook's cost visible and show that it differs
between an absent and an active trace context.

**Independent Test**: `go test -run '^$' -bench 'BenchmarkLoggerTracingHook' -benchmem ./...`
prints separate `B/op`/`allocs/op` for `no_trace_context` and
`active_trace_context` (spec FR-004, SC-002).

### Implementation for User Story 2

- [X] T004 [US2] In `logger_bench_test.go`, add
  `func BenchmarkLoggerTracingHook(b *testing.B)` as a table-driven benchmark
  (sub-cases `no_trace_context` and `active_trace_context`) using
  `b.ReportAllocs()` and `b.Loop()`. Both emit `Info(ctx).Msg(...)` on an
  `InfoLevel` discard-sink logger; `no_trace_context` passes
  `context.Background()` (zero-ID constant path), `active_trace_context` passes
  the populated-`SpanContext` context from the T002 helper (hex-format path that
  allocates). Doc comment states it isolates the hook's context-dependent
  allocation cost (FR-004; research Decision 2).

**Checkpoint**: US1 and US2 benchmarks both run independently; the hook's
marginal cost (active vs. absent span) is measurable.

---

## Phase 5: User Story 3 - Cover representative field-shape variations (Priority: P3)

**Goal**: Reflect realistic usage — a line carrying typed payload fields and a
logger configured with low-cardinality labels.

**Independent Test**: `go test -run '^$' -bench 'BenchmarkLoggerFieldShapes' -benchmem ./...`
prints separate profiles for `typed_fields` and `with_labels` (spec FR-006, US3).

### Implementation for User Story 3

- [X] T005 [US3] In `logger_bench_test.go`, add
  `func BenchmarkLoggerFieldShapes(b *testing.B)` as a table-driven benchmark
  (sub-cases `typed_fields` and `with_labels`) using `b.ReportAllocs()` and
  `b.Loop()`. `typed_fields`: `Info(ctx).Str("k","v").Int("n",1).Msg(...)` on a
  plain discard-sink logger. `with_labels`: a logger built with
  `WithLoggerLabels(Labels{Application:"app", Environment:"test"})` (plus the
  discard sink), then `Info(ctx).Msg(...)`. Doc comment states it captures the
  representative field-shape and labeled-logger profiles (FR-006).

**Checkpoint**: All three benchmark functions run independently; the full
six-variant matrix from data-model.md is covered.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Record the evidence, reconcile the claim, validate, and retire the
governing ADR.

- [X] T006 [P] Run `go test -run '^$' -bench '^BenchmarkLogger' -benchmem -count=5 ./...`
  and append a "Measured Allocation Profile" results table to
  `specs/011-hot-path-benchmarks/research.md` recording `B/op` and `allocs/op`
  per variant plus the conditions of measurement (FR-009, SC-004). (Different
  file from T007 → parallelizable.)
- [X] T007 [P] Reconcile the "zero or near-zero allocation hot path" claim with
  the measured numbers (research Decision 5): if confirmed, state the backed
  claim and the documented active-context exception in `README.md` (the
  surviving home, near `README.md:167`); if contradicted, revise the wording to
  match the evidence. No CI allocation gate is added (spec Assumptions). (Edits
  `README.md` → parallelizable with T006.)
- [X] T008 Run the full local gate and fix all findings: `gofmt -l .` (must be
  empty), `go vet ./...`, `golangci-lint run`, `go test -race ./...` (the new
  `logger_bench_test.go` must build and its benchmarks compile under `-race`),
  `make doc-coverage` (must stay clean — no new exported runtime symbol), and
  `markdownlint-cli2 "specs/011-hot-path-benchmarks/**/*.md"`. (AGENTS.md
  workflow steps 5-7; SC-005)
- [X] T009 Validate `specs/011-hot-path-benchmarks/quickstart.md` end to end:
  run each documented command and confirm the output columns and the
  acceptance-verification map behave as written (SC-001…SC-005).
- [X] T010 [FINAL] Retire ADR-0009 — ONLY now that
  `research.md` §"Decision Records Absorbed" captures its decision, considered
  alternatives, and consequences: delete `docs/adr/0009-logger-zerolog.md` and
  update its references (research "Retirement note", verified 2026-06-26):
  `logger.go:30` doc comment ("Logger is the ADR-0009 logging surface…")
  redirected to Constitution §VIII; `README.md:186` link and `README.md:213`
  ADR-index row removed/redirected to Constitution §VIII; `ROADMAP.md:82`
  (benchmark line → mark done, drop `(ADR-0009)` tag), `ROADMAP.md:213` (resolve
  tag) and `ROADMAP.md:238` (redirect to Constitution §VI); `CONTEXT.md:71` and
  `AGENTS.md:110` ("while ADR-0009 stands") redirected to Constitution §VI/§VIII.
  Historical references in `specs/002-version-injection/`,
  `specs/004-real-otel-export/`, and `specs/008-stability-deprecation-policy/`
  are redirected to this feature's absorbed decision record where needed so the
  ADR deletion leaves no dead links. Re-run `markdownlint` on changed markdown and
  confirm no dangling `0009-logger-zerolog` reference remains
  (`grep -rn "0009-logger-zerolog\|ADR-0009"` returns only this feature's specs).
  (Constitution §Governance)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1, T001)**: No dependencies — start immediately.
- **Foundational (Phase 2, T002)**: Depends on T001. BLOCKS all user stories.
- **User Story 1 (Phase 3, T003)**: Depends on T002. Independent of US2/US3.
- **User Story 2 (Phase 4, T004)**: Depends on T002. Independent of US1/US3.
- **User Story 3 (Phase 5, T005)**: Depends on T002. Independent of US1/US2.
- **Polish (Phase 6)**: T006/T007 depend on all of T003–T005 (suite must exist
  to measure). T008/T009 depend on the suite + docs. **T010 (ADR retirement) is
  LAST** — depends on every prior task AND on `research.md` having absorbed
  ADR-0009 (already done in Phase 0).

### User Story Dependencies

- US1, US2, US3 each depend ONLY on Foundational (T002); none depends on another
  story. They are independently runnable benchmarks. They are written sequentially
  in practice only because they share `logger_bench_test.go` (same-file edit), not
  because of a logical dependency.

### Within Each User Story

- One task per story; each writes one self-contained `BenchmarkLogger*` function
  with its doc comment, then is verified by running just that benchmark.

### Parallel Opportunities

- **T006 and T007 are [P]** — they edit different files (`research.md` vs
  `README.md`) and only share the prerequisite that the suite has been run.
- Story tasks T003/T004/T005 are NOT [P]: all edit `logger_bench_test.go`.
  If isolation were needed they could be split across worktrees, but the file is
  small and sequential editing is simpler.

---

## Parallel Example: Polish Phase

```bash
# After T003–T005 land and the suite runs, T006 and T007 can proceed together:
Task: "T006 Record measured allocation profile table in research.md"
Task: "T007 Reconcile/confirm the allocation claim wording in README.md"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. T001 (Setup) → T002 (Foundational helpers).
2. T003 (US1: `BenchmarkLoggerEmit`).
3. **STOP and VALIDATE**: run the US1 benchmark; confirm `B/op`/`allocs/op`
   appear for the enabled and disabled paths. This alone backs the headline
   claim — a shippable MVP.

### Incremental Delivery

1. Setup + Foundational → file scaffold + fixtures ready.
2. US1 → run → headline allocation number (MVP).
3. US2 → run → tracing-hook cost isolated (active vs. absent context).
4. US3 → run → representative field/label profiles.
5. Polish → record numbers, reconcile claim, validate, retire ADR-0009.

---

## Notes

- [P] = different files, no dependencies. Only T006/T007 qualify here.
- The benchmarks ARE the tests; there is no separate implementation to test.
- Run benchmarks with `-benchmem`; the suite also calls `b.ReportAllocs()` so the
  allocation columns appear regardless.
- T010 deletes a frozen ADR — never do it before `research.md` absorption (done)
  and never before the rest of the feature is complete.
- Commit after each task or logical group (do not commit unless instructed).
