---
description: "Task list for Real OTel Export & Span Lifecycle"
---

# Tasks: Real OTel Export & Span Lifecycle

**Input**: Design documents from `/specs/004-real-otel-export/`

**Prerequisites**: plan.md (required), spec.md (required for user stories),
research.md, data-model.md, contracts/telemetry-api.md, quickstart.md

**Tests**: INCLUDED and written test-first. This feature is governed by
Constitution Principle VII (Test-First Discipline, NON-NEGOTIABLE) and AGENTS.md
("Tests land before implementation"); plan.md enumerates the test files. Within
each story, failing tests land before the implementation that makes them pass.

**Organization**: Tasks are grouped by user story to enable independent
implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths are included in each description

## Path Conventions

Single Go library at the module root (constitution-mandated: public package `ax`
at root, mechanics under `internal/`). Public surface in `telemetry.go` /
`execute.go`; exporter/processor/sampler mechanics in
`internal/telemetry/telemetry.go`; tests are root-level `*_test.go` files.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish a clean pre-change baseline and add the two governed
exporter dependencies (FR-014, research D9).

- [X] T001 Capture a green pre-change baseline: run `go test -race ./...` and
  confirm it passes, so any later regression is attributable to this feature's
  changes.
- [X] T002 Add the two OTel-canonical exporter dependencies, version-locked in
  lockstep with `go.opentelemetry.io/otel/sdk` v1.44.0: run
  `go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.44.0`
  and `go get go.opentelemetry.io/otel/exporters/stdout/stdouttrace@v1.44.0`;
  confirm `go.mod` / `go.sum` are updated and `go build ./...` is clean
  (FR-014, research D9).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared option surface and config plumbing every story rides on.
No telemetry behavior changes yet — the provider stays no-op — but the seam for
the root span (US1) and the exporters (US2/US3) is established here.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T003 Add the four new functional options and extend `telemetryConfig` in
  `telemetry.go`: `WithTelemetryStderr(io.Writer)`,
  `WithTelemetryServiceName(string)`, `WithTelemetryServiceVersion(string)`,
  `WithTelemetryShutdownBudget(time.Duration)`; add `stderr`, `serviceName`,
  `serviceVersion`, `shutdownBudget` fields (defaults `os.Stderr`, `""`, `""`,
  `2s`). Each exported option carries a doc comment (godoclint `require-doc`).
  (data-model "Public option surface"; contract lines 27-43; FR-012)
- [X] T004 Reshape the internal constructor in `internal/telemetry/telemetry.go`
  to accept a resolved telemetry config (endpoint, debug, stderr, serviceName,
  serviceVersion, shutdownBudget) instead of only `env func(string) string`;
  resolve `OTEL_EXPORTER_OTLP_ENDPOINT` and `AX_OTEL_DEBUG` from `env` once at
  startup (research D5/D11) and keep the provider no-op (no processor) so
  behavior is unchanged. Update `StartTelemetry` in `telemetry.go` to build that
  config from its options and pass it through. (data-model "Telemetry
  configuration"; FR-012) (depends on T003)
- [X] T005 Wire `Execute` in `execute.go` to pass the new options into
  `StartTelemetry`: `WithTelemetryStderr(cfg.stderr)`,
  `WithTelemetryServiceName(root.Name())`,
  `WithTelemetryServiceVersion(cfg.version)`,
  `WithTelemetryShutdownBudget(cfg.shutdownTimeout)`. No behavior change yet.
  (data-model "State transitions"; plan execute.go bullet) (depends on T003, T004)

**Checkpoint**: Foundation ready — the option surface compiles and `Execute`
threads config through; `stdout`/exit codes are still byte-identical.

---

## Phase 3: User Story 1 - Every command's logs are trace-correlated, no collector required (Priority: P1) 🎯 MVP

**Goal**: A recording root span wraps every command so each `stderr` log line
carries a non-zero, stable `trace_id`/`span_id` — with no inbound `TRACEPARENT`
and no collector configured — without touching `stdout`.

**Independent Test**: Run a command with empty env (no `TRACEPARENT`, no
endpoint), capture `stderr`, and confirm log lines carry a non-zero
`trace_id`/`span_id` that are identical across lines and match the run's active
span; confirm those IDs never appear in the `stdout` payload.

### Tests for User Story 1 (write first, MUST fail before implementation) ⚠️

- [X] T006 [P] [US1] In `telemetry_test.go`, add failing tests: (a) with empty
  env, a recording root span is active and `TraceIDFromContext` /
  `SpanIDFromContext` over the run context return non-zero, stable values; (b)
  with an inbound `TRACEPARENT` whose `flags=00` (not-sampled), the root span
  still records (AlwaysSample overrides the `ParentBased` default). (FR-001,
  FR-002, FR-003, SC-001; research D1/D2)
- [X] T007 [P] [US1] In `execute_test.go`, add failing tests: every `stderr` log
  line emitted during a command run carries a non-zero `trace_id`/`span_id`,
  identical across lines and equal to the active span; an inbound `TRACEPARENT`
  is continued (run `trace_id` equals the inbound trace ID); and none of these
  IDs appear on `stdout`. (US1 acceptance 1-3; FR-001/FR-002/FR-003; SC-001)

### Implementation for User Story 1

- [X] T008 [US1] In `internal/telemetry/telemetry.go`, construct the provider
  with an explicit `sdktrace.WithSampler(sdktrace.AlwaysSample())` (overriding
  the SDK `ParentBased` default) and a `resource.Resource` carrying
  `semconv.ServiceName(serviceName)` + `semconv.ServiceVersion(serviceVersion)`
  via `sdktrace.WithResource(...)`. (research D2/D8; `resource`/`semconv` are
  subpackages of existing modules — no new dependency)
- [X] T009 [US1] In `execute.go`, open the root span: after `StartTelemetry`
  returns, get `otel.Tracer("github.com/rshade/ax-go")`, call
  `ctx, span := tracer.Start(ctx, root.Name())`, and register `defer span.End()`
  AFTER the existing `Shutdown` defer so LIFO runs `End()` before `Shutdown()`;
  on a failing run set `span.SetStatus(codes.Error, …)` (status code only — do
  NOT copy the error message into a span attribute). (FR-001, FR-010; research D1)
- [X] T010 [US1] In `execute.go`, refine the span name to `cmd.CommandPath()`
  inside the wrapped `PersistentPreRunE` via
  `trace.SpanFromContext(cmd.Context()).SetName(cmd.CommandPath())`, where the
  resolved subcommand is known. (research D1) (depends on T009; same file)
- [X] T011 [US1] Run `go test -race` for the US1 tests (T006, T007) and confirm
  they pass; re-run the existing `stdout` golden/separation tests in
  `golden_test.go` unchanged to prove zero `stdout` footprint (FR-010/SC-003).
  Also re-run the existing `logger_test.go` zero-fallback assertion
  (`ZeroTraceID`/`ZeroSpanID` for a span-less context) unchanged, confirming the
  always-on root span does not break the documented all-zeros fallback for a
  context with no active span (spec Edge Case "No active span (defensive)";
  research D10).

**Checkpoint**: US1 is fully functional — correlation works for every
invocation with zero infrastructure. This is the MVP.

---

## Phase 4: User Story 2 - Spans reach a configured collector, zero loss on exit (Priority: P2)

**Goal**: When `OTEL_EXPORTER_OTLP_ENDPOINT` is set, the run's spans are exported
to that collector via a synchronous `SimpleSpanProcessor` and are flushed before
the process exits; transport honors the endpoint scheme (plaintext `http://`
allowed, `https://` verified, never `InsecureSkipVerify`); and telemetry is
fail-open so a failing collector never changes `stdout` or the exit code.

**Independent Test**: Point `OTEL_EXPORTER_OTLP_ENDPOINT` at a capturing
`httptest` receiver, run a command, let it exit normally, and confirm the
receiver observed the run's span(s) before exit. Separately, point it at an
unreachable/malformed endpoint and confirm `stdout` and the exit code are
unchanged.

**Note**: Builds on US1's recording root span (an exporter has nothing
meaningful to send until a recording span exists — spec US2 rationale).

### Tests for User Story 2 (write first, MUST fail before implementation) ⚠️

- [X] T012 [P] [US2] Create `telemetry_export_test.go` (NEW): stand up an
  `httptest.Server` OTLP receiver, set `OTEL_EXPORTER_OTLP_ENDPOINT` to its
  `http://` URL, run a command, let it exit, and assert the receiver observed
  the run's span(s) before exit (SC-002); add a case with an inbound
  `TRACEPARENT` asserting the exported spans carry the inbound `trace_id`
  (SC-008); add a `flags=00` case asserting export still happens (FR-002).
- [X] T013 [P] [US2] In `execute_test.go`, add failing fail-open tests: a
  malformed endpoint and an unreachable collector leave `stdout` byte-identical
  and the exit code unchanged, surfacing only a `stderr` diagnostic
  (FR-008/SC-006); add a stuck-collector case (a receiver that blocks) asserting
  the command returns within the shutdown budget (FR-007 edge case).

### Implementation for User Story 2

- [X] T014 [US2] In `internal/telemetry/telemetry.go`, build the OTLP HTTP
  exporter when the endpoint is set: `otlptracehttp.New(ctx, …)` with the
  per-attempt timeout derived from the shutdown budget
  (`otlptracehttp.WithTimeout(budget)`) and retry disabled
  (`otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false})`);
  delegate scheme/TLS to the exporter (plaintext `http://` permitted, `https://`
  verified — never pass `WithInsecure()` blindly nor `InsecureSkipVerify: true`);
  register it via `sdktrace.NewSimpleSpanProcessor(exporter)`. (research
  D3/D4/D5; FR-004/FR-007)
- [X] T015 [US2] Make telemetry fail-open in `internal/telemetry/telemetry.go`
  and `telemetry.go`: exporter-construction or endpoint errors degrade to the
  no-op path with a single `stderr` diagnostic (e.g.
  `ax: otel exporter disabled: <reason>`) and `StartTelemetry` returns a usable
  `*Telemetry` (possibly no-op) — never a fatal error. (research D7; FR-008)
  (depends on T014)
- [X] T016 [US2] In `execute.go`, remove the fail-closed
  `telemetryErr != nil → ExitInternal` branch so telemetry errors no longer fail
  the command (the diagnostic now comes from T015's fail-open path). The now-nil
  error is discarded at the call site — change
  `ctx, telemetry, telemetryErr := StartTelemetry(...)` to
  `ctx, telemetry, _ := StartTelemetry(...)` so no unused variable remains;
  `StartTelemetry`'s 3-tuple signature is frozen and unchanged, the trailing
  `error` is simply always nil now (contract godoc). (research D7; FR-008)
- [X] T017 [US2] Run `go test -race` for the US2 tests (T012, T013); confirm no
  `InsecureSkipVerify` code path exists and `stdout` stays clean across the OTLP
  path. (FR-004/FR-009/FR-013/SC-007)

**Checkpoint**: US1 and US2 both work — correlation everywhere, plus guaranteed
export-before-exit to a configured collector with fail-open safety.

---

## Phase 5: User Story 3 - See spans locally without a collector (Priority: P3)

**Goal**: With `AX_OTEL_DEBUG` set, the command prints human-readable span data
to `stderr` (never `stdout`); strictly opt-in; and when both `AX_OTEL_DEBUG` and
`OTEL_EXPORTER_OTLP_ENDPOINT` are set, both destinations receive the spans.

**Independent Test**: Set `AX_OTEL_DEBUG=1`, run a command with no collector,
and confirm span data appears on `stderr` and nothing telemetry-related appears
on `stdout`. Unset it and confirm silence everywhere.

**Note**: Reuses the US1/US2 span lifecycle and the fail-open path from US2; only
the destination differs.

### Tests for User Story 3 (write first, MUST fail before implementation) ⚠️

- [X] T018 [P] [US3] Create `telemetry_debug_test.go` (NEW): with `AX_OTEL_DEBUG`
  truthy and a captured `stderr` buffer, run a command and assert human-readable
  span data appears on `stderr` and nothing telemetry-related appears on
  `stdout` (FR-006/FR-009/SC-005); add the absent-variable case asserting silence
  everywhere; add the both-set case (debug + OTLP receiver) asserting both
  destinations receive the run's spans (edge case).

### Implementation for User Story 3

- [X] T019 [US3] In `internal/telemetry/telemetry.go`, build the debug exporter
  when `AX_OTEL_DEBUG` is truthy: `stdouttrace.New(stdouttrace.WithWriter(w))`
  where `w` is the configured `stderr` wrapped in a small mutex-synchronized
  `io.Writer` (so concurrent debug exports and zerolog lines sharing the sink
  cannot interleave or trip `-race`); register it via a second
  `SimpleSpanProcessor` alongside the OTLP one when both are configured. (research
  D6; FR-006/FR-009/FR-013)
- [X] T020 [US3] Run `go test -race` for the US3 tests (T018), including the
  shared-buffer debug-plus-logger concurrency case, and confirm zero data races.
  (SC-007)

**Checkpoint**: All three user stories are independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Cross-cutting test surfaces, the verified example + doc-coverage
ratchet, docs, the full validation gate, and the governed ADR retirement.

- [X] T021 [P] Create `telemetry_fuzz_test.go` (NEW) with
  `FuzzTraceparentExtraction(f *testing.F)` exercising the `TRACEPARENT`
  extraction parser surface with malformed/edge inputs, asserting no panic and a
  valid context. (Constitution Principle VII parser-surface rule)
- [X] T022 [P] Create `http_test.go` (NEW) — `http.go` stays unchanged — adding
  an outbound-propagation coverage test: with a root span active, assert
  `ax.HTTPClient().Do(req.WithContext(ctx))` propagates the active span's
  `trace_id` in the outbound W3C `traceparent` header (use an `httptest` echo
  server). (FR-011)
- [X] T023 [P] In `execute_test.go`, add a stream-separation test across all
  three telemetry modes (no-op, OTLP, debug) asserting zero telemetry bytes ever
  reach `stdout`. (FR-009/SC-004)
- [X] T024 Add a verified `ExampleStartTelemetry` in `example_test.go` that
  demonstrates the new options inside it (with an `// Output:` line), then remove
  **only** the `StartTelemetry` line from `internal/cmd/doccover/baseline.txt`
  and run `make doc-coverage` to confirm it stays green (ratchet off the
  baseline). The `Telemetry` line stays in `baseline.txt` this feature: an
  `ExampleStartTelemetry` satisfies the `StartTelemetry` symbol only, not the
  `Telemetry` type, which remains a grandfathered gap to be burned down by a
  later feature. (Constitution Principle VII; plan.md line 103)
- [X] T025 [P] Update the telemetry section of `README.md`: correlation-by-default,
  the two env vars (`OTEL_EXPORTER_OTLP_ENDPOINT`, `AX_OTEL_DEBUG`), scheme-honoring
  TLS, fail-open behavior, and the flush-on-exit budget. (Workstream C)
- [X] T026 [P] Update `examples/integration/main.go` to document
  `OTEL_EXPORTER_OTLP_ENDPOINT` and `AX_OTEL_DEBUG` in its `--help`/`Example`
  output, keeping the integration example current with public behavior.
  (Workstream C; AGENTS.md workflow step 4)
- [X] T027 Run the full validation gate and fix all findings: `gofmt`,
  `go vet ./...`, `golangci-lint run` (incl. `godoclint` require-doc),
  `make doc-coverage`, `go test -race ./...`, and `markdownlint` on changed
  markdown. (AGENTS.md workflow steps 5-7; SC-007)
- [X] T028 Validate `specs/004-real-otel-export/quickstart.md` end to end:
  exercise correlation-for-free, export-to-a-receiver, the `AX_OTEL_DEBUG` debug
  path, the fail-open/unreachable-collector path, and outbound propagation, and
  confirm each behaves as documented.
- [X] T029 [FINAL] Retire ADR-0005 — ONLY now that `research.md` §Decision
  Records Absorbed captures its decision/alternatives/consequences: delete
  `docs/adr/0005-otel-integration.md` and update its **7 references** —
  `ROADMAP.md:40` (#2 line → mark done, drop the `(ADR-0005)` tag) and
  `ROADMAP.md:165` (resolve the "no-op scaffold (ADR-0005, partial)" line to
  done); and redirect the sibling-ADR cross-references in
  `docs/adr/0004-trace-id-format.md:61`, `docs/adr/0007-id-strategy.md:35`,
  `docs/adr/0008-cli-framework-cobra.md:70`, and
  the structured-logger decision now absorbed in
  `../011-hot-path-benchmarks/research.md` to constitution §VIII or
  `specs/004-real-otel-export/`. ADR-0004 is **retained** (research D10). No Go
  source file references ADR-0005 by number, so no doc-comment edits are needed.
  (Constitution §Governance; research "Retirement note")

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup. BLOCKS all user stories. Internal
  order: T003 → T004 → T005.
- **User Story 1 (Phase 3, P1)**: Depends on Foundational. No dependency on US2/US3.
- **User Story 2 (Phase 4, P2)**: Depends on Foundational AND on US1's recording
  root span (spec US2 rationale: nothing meaningful to export without it).
- **User Story 3 (Phase 5, P3)**: Depends on Foundational AND on US1's span
  lifecycle; reuses US2's fail-open path. Independent of US2's OTLP specifics.
- **Polish (Phase 6)**: Depends on the user stories it covers. T024 depends on
  US1 (the example exercises `StartTelemetry`'s new shape). T029 (ADR retirement)
  is LAST: it depends on every story AND on `research.md` having absorbed
  ADR-0005's decisions (already done).

### Within Each User Story

- Tests are written and MUST FAIL before implementation.
- Provider/sampler config (`internal/telemetry`) before span lifecycle
  (`execute.go`); exporter construction before fail-open wiring.
- Story complete and `-race`-clean before moving to the next priority.

### Parallel Opportunities

- US1 tests T006 and T007 are different files → parallel.
- US2 tests T012 (new file) and T013 (`execute_test.go`) → parallel.
- Polish T021, T022, T023, T025, T026 are different files → parallel.
- Documentation tasks (T025, T026) can run concurrently with US implementation
  (Workstream C runs alongside A/B).

---

## Parallel Example: User Story 1

```bash
# Launch the two US1 test-authoring tasks together (different files):
Task: "T006 [US1] failing correlation + AlwaysSample tests in telemetry_test.go"
Task: "T007 [US1] failing log-correlation + continuity tests in execute_test.go"

# Then implement sequentially (shared files):
# T008 (internal/telemetry/telemetry.go) → T009 → T010 (execute.go) → T011 (verify)
```

## Parallel Example: Polish Phase

```bash
# Independent cross-cutting tasks, all different files:
Task: "T021 FuzzTraceparentExtraction in telemetry_fuzz_test.go"
Task: "T022 outbound-propagation test in http_test.go"
Task: "T023 stream-separation-across-modes test in execute_test.go"
Task: "T025 telemetry section in README.md"
Task: "T026 env-var docs in examples/integration/main.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1: Setup (T001-T002).
2. Phase 2: Foundational (T003-T005) — CRITICAL, blocks all stories.
3. Phase 3: User Story 1 (T006-T011).
4. **STOP and VALIDATE**: every command's logs carry non-zero correlated IDs with
   no collector; `stdout` byte-identical. Ship the highest-value, zero-infra win.

### Incremental Delivery

1. Setup + Foundational → seam ready.
2. US1 → correlation everywhere → validate → demo (MVP).
3. US2 → export-before-exit + fail-open → validate → demo.
4. US3 → local debug exporter → validate → demo.
5. Polish → fuzz/propagation/separation tests, example + doc ratchet, docs,
   validation gate, ADR-0005 retirement.

### Notes

- `[P]` = different files, no dependencies.
- `[Story]` label maps each task to its user story for traceability.
- Verify tests fail for the right reason before implementing.
- Run `go test -race ./...` after each story; the race detector is REQUIRED.
- Do NOT delete `docs/adr/0005-otel-integration.md` before `research.md` records
  its decisions (it already does) — T029 is gated on that and runs last.
