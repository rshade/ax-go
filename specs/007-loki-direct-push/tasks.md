# Tasks: Loki Direct-Push Addon

**Input**: Design documents from `specs/007-loki-direct-push/`

**Prerequisites**: plan.md ✅ | spec.md ✅ | research.md ✅ | data-model.md ✅ | contracts/public-api.md ✅

**TDD mandate**: Per Constitution Principle VII and AGENTS.md, tests land *before* implementation
and must fail for the right reason before the implementation tasks in each phase begin.

**Governing ADR**: `docs/adr/0006-loki-integration.md` — absorbed into `research.md`;
retired as T038 (the final task).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (independent files, no unresolved dependencies)
- **[Story]**: User story label — US1/US2/US3/US4

---

## Phase 1: Setup — `logger.go` Extension Points

**Purpose**: The three minimal changes to `logger.go` that wire in generic sink fan-out.
These tasks touch `logger.go` only and carry no Loki-specific concepts.

**⚠️ CRITICAL**: Phases 2–6 depend on these changes being present.

- [X] T001 Add `additionalSinks []logSink` field to `loggerConfig` struct in `logger.go`
- [X] T002 Add `sinks []logSink` field to `zerologLogger` struct in `logger.go`; propagate from `cfg.additionalSinks` in `NewLogger` construction
- [X] T003 Update `NewLogger` in `logger.go` to build `io.MultiWriter(cfg.writer, sinks...)` when `len(cfg.additionalSinks) > 0` and store sinks on the returned `zerologLogger`
- [X] T004 Add unexported `flusher` interface (`flush(context.Context) error`) in `logger.go` and implement `flush()` on `zerologLogger` (iterates `sinks`, calls `drain(ctx)` on each, returns combined error)

**Checkpoint**: `go build ./...` must succeed; `go test ./...` (existing tests) must still pass.

---

## Phase 2: Foundational — Core Types + TDD Skeletons

**Purpose**: Define the unexported Loki types and write the failing tests that constrain the
implementation. All tests in this phase MUST fail before Phase 3 implementation begins.

**⚠️ CRITICAL**: No user-story implementation starts until these tests exist and fail correctly.

- [X] T005 [P] Define unexported types `lokiStreamKey`, `lokiPushBody`, `lokiStream` in `loki.go` per data-model.md; declare `lokiWriter` struct skeleton with fields: `pushURL string`, `authToken string`, `errorWriter io.Writer`, `ch chan lokiEntry`, `flushRequests chan chan struct{}`, `client *http.Client`, `done chan struct{}`
- [X] T006 [P] Write `TestLokiNoop_NoEnvVar` in `loki_test.go`: unset `AX_LOKI_URL`, call `NewLogger` with `WithLokiFromEnv()`, assert `len(l.(zerologLogger).sinks) == 0` (must FAIL — `WithLokiFromEnv` not yet implemented)
- [X] T007 [P] Write `TestLokiImportIsolation` in `loki_test.go`: run `go list -json -deps github.com/rshade/ax-go` via `os/exec`, parse output, assert no import path contains the substring `"loki"` (must FAIL until we confirm stdlib-only — or PASS if already clean)
- [X] T008 [P] Write `TestLokiWriter_PushesOnFlush` in `loki_test.go`: start `httptest.NewServer`, set `AX_LOKI_URL` to server URL, call `NewLogger` with `WithLokiFromEnv()`, write one log line, call `ax.Flush(ctx, logger)`, assert server received exactly one POST to `/loki/api/v1/push` with a valid JSON body containing `"streams"` key (must FAIL)
- [X] T009 [P] Write `TestLokiWriter_NetworkFailure` in `loki_test.go`: spin up a server that returns 503 for every request, write a log line, call `ax.Flush`, assert no panic and return value is `nil` (network failure is silent) (must FAIL)
- [X] T010 [P] Write `TestLokiWriter_BufferFull` in `loki_test.go`: use a server that never reads the request body (blocks indefinitely); write 300 log lines from a single goroutine; assert all 300 `Write()` calls return immediately (use `testing.T.Deadline` to enforce); assert no deadlock (must FAIL)
- [X] T011 [P] Write `TestLokiWriter_Race` in `loki_test.go`: launch 10 goroutines each writing 50 log lines to the same logger; call `ax.Flush(ctx, logger)` from the main goroutine concurrently; assert `go test -race` reports no data races (must FAIL)
- [X] T012 [P] Write `FuzzLokiWrite` in `loki_test.go`: fuzz corpus of arbitrary `[]byte` values passed to `lokiWriter.Write(p)`; assert Write always returns `(len(p), nil)` and never panics (must FAIL)

**Checkpoint**: All T006–T012 tests exist and fail for the right reason (symbol not found or assertion failure — not compilation error). Run: `go test -run 'TestLokiNoop|TestLokiImport|TestLokiWriter|FuzzLoki' ./... 2>&1 | grep -E 'FAIL|undefined'`

---

## Phase 3: User Story 1 — Core Push Functionality (Priority: P1) 🎯 MVP

**Goal**: When `AX_LOKI_URL` is set and the CLI author calls `WithLokiFromEnv()`, log
lines are non-blocking fan-out to both stderr and the Loki push endpoint.

**Independent Test**: Point `AX_LOKI_URL` at the `httptest.Server` in `TestLokiWriter_PushesOnFlush`; run `go test -race -run TestLokiWriter_PushesOnFlush ./...` — test must pass.

### Implementation

- [X] T013 [US1] Implement `lokiWriter.Write(p []byte) (int, error)` in `loki.go`: copy `p` into `lw.ch` non-blocking (select with default drop); always return `(len(p), nil)`
- [X] T014 [US1] Implement `lokiWriter` background goroutine (`run()` method) in `loki.go`: ticker 1 s flush + max-100-entry batch flush; on flush request drain remaining entries from `ch` (non-blocking loop), post current batch, acknowledge request, and keep running; on logger context cancellation post a final batch, then `close(lw.done)`
- [X] T015 [US1] Implement `postBatch(ctx context.Context, batch []lokiEntry)` in `loki.go`: extract `level` per line using a scan (see plan.md §Level extraction); extract permitted label fields from the emitted JSON line; group entries into `lokiStream` slices by full stream key; marshal `lokiPushBody`; `POST` to `lw.pushURL` using `ax.HTTPClient()` with 10 s per-request `context.WithTimeout`; log non-2xx to the configured writer; always `Close()` response body
- [X] T016 [US1] Implement `extractLevel(line []byte) string` in `loki.go`: scan for `"level":"` prefix; read level name chars until `"`; return `"unknown"` if not found
- [X] T017 [US1] Implement `lokiWriter.drain(ctx context.Context) error` in `loki.go`: send an in-band flush request, wait on its acknowledgement respecting caller context but capped at 2 seconds, and leave the sink running; return nil
- [X] T018 [US1] Implement `WithLokiFromEnv() LoggerOption` in `loki.go`: read `AX_LOKI_URL` via `os.Getenv`; if empty return no-op option; validate URL with `url.Parse` — if malformed write warning to `cfg.writer` and return no-op; construct `*lokiWriter` with `ax.HTTPClient()`, configured writer, `ch: make(chan lokiEntry, 256)`, `flushRequests: make(chan chan struct{})`, `done: make(chan struct{})`; start background goroutine; append to `cfg.additionalSinks`
- [X] T019 [US1] Implement `Flush(ctx context.Context, l Logger) error` in `loki.go`: type-assert `l.(flusher)`; if ok call `l.flush(ctx)`; return nil if assertion fails; document contract per `contracts/public-api.md`
- [X] T020 [US1] Run `go test -race -run 'TestLokiNoop|TestLokiWriter_PushesOnFlush|TestLokiWriter_NetworkFailure|TestLokiWriter_BufferFull|TestLokiWriter_Race' ./...` — all five tests must pass; fix any failures before proceeding

**Checkpoint**: US1 independently functional. Run `go test -race ./...` — all existing tests plus T006–T011 pass.

---

## Phase 4: User Story 2 — Import Isolation + No-Op Disabled Path (Priority: P1)

**Goal**: A CLI built without `WithLokiFromEnv()` (or with `AX_LOKI_URL` unset) has zero
Loki-related imports and zero additional startup overhead.

**Independent Test**: Run `go test -run TestLokiNoop_NoEnvVar ./...` and `go test -run TestLokiImportIsolation ./...` — both must pass.

### Implementation

- [X] T021 [US2] Verify `TestLokiNoop_NoEnvVar` (T006) passes with `AX_LOKI_URL` unset: `WithLokiFromEnv()` must return a no-op option that leaves `zerologLogger.sinks` empty; fix `WithLokiFromEnv` if the no-op path is broken
- [X] T022 [P] [US2] Verify `TestLokiImportIsolation` (T007) passes: run `go list -json -deps github.com/rshade/ax-go` and confirm no import path contains `"loki"`; since `loki.go` uses only stdlib (`net/http`, `encoding/json`, `bytes`, `sync`, `time`, `context`), this must be trivially satisfied
- [X] T023 [P] [US2] Audit `logger.go` imports: open the file and verify the import block contains no reference to `"loki"`, no `"net/http"` (that wasn't there before), and no packages from `loki.go`; document result in a one-line comment in `loki_test.go` alongside T007

**Checkpoint**: US2 independently verified. `go test -run 'TestLokiNoop|TestLokiImport' ./...` passes.

---

## Phase 5: User Story 3 — Authenticated Loki / Secure Transport (Priority: P2)

**Goal**: When `AX_LOKI_AUTH_TOKEN` is set, push requests carry `Authorization: Bearer <token>`;
TLS is never skipped; an HTTP (non-HTTPS) URL on a non-loopback host emits a warning.

**Independent Test**: Run `go test -run 'TestLokiAuth' ./...` — both auth tests pass.

### Tests (TDD — write before implementation)

- [X] T024 [P] [US3] Write `TestLokiAuth_BearerToken` in `loki_test.go`: set `AX_LOKI_AUTH_TOKEN`; capture request headers in `httptest.Server` handler; assert `Authorization` header equals `"Bearer <token>"` (must FAIL before T026)
- [X] T025 [P] [US3] Write `TestLokiAuth_InsecureURLWarning` in `loki_test.go`: set `AX_LOKI_URL` to an `http://` URL with a non-loopback host (`http://example.com:3100`); capture `cfg.writer` output; assert warning message containing `"insecure"` or `"http"` is written to `cfg.writer` at construction time (must FAIL before T027)

### Implementation

- [X] T026 [US3] Implement bearer token header in `postBatch` in `loki.go`: when `lw.authToken != ""` set `req.Header.Set("Authorization", "Bearer "+lw.authToken)` before `client.Do(req)`; store `authToken` from `os.Getenv("AX_LOKI_AUTH_TOKEN")` in `WithLokiFromEnv`
- [X] T027 [US3] Implement insecure-URL warning in `WithLokiFromEnv` in `loki.go`: after `url.Parse`, if scheme is `"http"` and host is not a loopback address (`127.0.0.1`, `::1`, `localhost`), write `"ax: AX_LOKI_URL uses insecure http transport\n"` to `cfg.writer`; continue (do not disable Loki)
- [X] T028 [US3] Run `go test -race -run 'TestLokiAuth' ./...` — both auth tests must pass; fix any failures

**Checkpoint**: US3 independently verified. Auth header present, insecure URL warned.

---

## Phase 6: User Story 4 — Cardinality Discipline (Priority: P2)

**Goal**: Loki stream labels contain exactly the five permitted low-cardinality fields;
`trace_id`, `span_id`, and all high-cardinality fields remain in the log-line body only.

**Independent Test**: Run `go test -run 'TestLokiCardinality|TestLokiLevel' ./...` — both tests pass.

### Tests (TDD — write before implementation)

- [X] T029 [P] [US4] Write `TestLokiCardinality_StreamLabelsOnly5Keys` in `loki_test.go`: construct a `Logger` with full `Labels{Environment: "prod", Application: "app", Host: "h1", Version: "1.0"}`, write a log line from a context with an active OTel span (so `trace_id` is non-zero), call `ax.Flush`, inspect the captured POST body: assert `streams[*].stream` map contains no key outside `{environment, application, host, version, level}` and `trace_id`/`span_id` do NOT appear as stream keys (must FAIL before T031–T032)
- [X] T030 [P] [US4] Write `TestLokiLevelExtraction` in `loki_test.go`: table-driven test over zerolog JSON lines for levels `debug`, `info`, `warn`, `error`, `panic`, and a line with no level field; call `extractLevel(line)` and assert expected output; must include the "no level field → `unknown`" case (must FAIL before T031)

### Implementation

- [X] T031 [US4] Verify `extractLevel` (T016, already implemented in Phase 3) satisfies all `TestLokiLevelExtraction` cases; fix edge cases if needed (empty lines, malformed JSON, level value not a simple string)
- [X] T032 [US4] Verify `postBatch` (T015) builds the `lokiStream.Stream` map from the fixed allowlist in `lokiStreamKey` only (the five permitted fields); confirm that the log-line JSON (full zerolog output) is placed verbatim as the `values[][1]` string — no field stripping; run `TestLokiCardinality_StreamLabelsOnly5Keys`; fix if failing

**Checkpoint**: US4 independently verified. `go test -race -run 'TestLokiCardinality|TestLokiLevel' ./...` passes.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: doc-coverage, integration example, linter, and ADR retirement.

- [X] T033 [P] Update `examples/integration/main.go`: add `ax.WithLokiFromEnv()` to the `NewLogger` call (line ~92) and add `defer func() { flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second); defer cancel(); _ = ax.Flush(flushCtx, logger) }()` in the command body, following the pattern in `quickstart.md`
- [X] T034 [P] Add `ExampleWithLokiFromEnv` in `example_test.go` demonstrating the no-op pattern (AX_LOKI_URL unset, so no network call): call `NewLogger` with `WithLokiFromEnv()`, write one log line, call `ax.Flush(ctx, logger)` — add `// Output:` comment to make it runnable; add `ExampleFlush` similarly
- [X] T035 Run `make doc-coverage` — verify `WithLokiFromEnv` and `Flush` appear in the required-primary-API list and have `ExampleXxx` coverage passing the ratchet; add them to `internal/cmd/doccover/baseline.txt` if needed
- [X] T036 Run `golangci-lint run ./...` — fix any `godoclint require-doc` failures on exported symbols in `loki.go` (`WithLokiFromEnv`, `Flush`); ensure all doc comments satisfy the "contract not narration" rule from AGENTS.md
- [X] T037 Run `go vet ./...` and `gofmt -w loki.go loki_test.go logger.go examples/integration/main.go example_test.go` — fix any issues; verify all changed files end with newline (LF)
- [X] T038 Run full test suite `go test -race ./...` — ALL tests (old + new) must pass; run `go test -fuzz FuzzLokiWrite -fuzztime 30s ./...` to smoke-test the fuzz target
- [X] T039 [FINAL] Retire ADR-0006 — gated on `research.md` having fully captured its decisions (✅ already done): (a) delete `docs/adr/0006-loki-integration.md`; (b) remove the ADR-0006 entry from `README.md` ADR index; (c) update `CONTEXT.md` to remove the reference to ADR-0006; (d) update `AGENTS.md` references; (e) search `ROADMAP.md` for "ADR-0006" and remove; (f) update Go doc-comments in `logger.go` and `loki.go` that cite ADR-0006 by file name — replace with constitution principle citation (Principle VIII)

**Checkpoint**: `golangci-lint run`, `go test -race ./...`, `make doc-coverage` all clean. ADR-0006 file deleted.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately.
- **Phase 2 (Foundational)**: Depends on Phase 1 completion (T001–T004 must be present for tests to compile).
- **Phase 3 (US1)**: Depends on Phase 2 (tests must exist and fail before implementation).
- **Phase 4 (US2)**: Can run after Phase 3 (validates that Phase 3 impl preserved no-op + isolation).
- **Phase 5 (US3)**: Depends on Phase 3 (auth is additive to the push implementation).
- **Phase 6 (US4)**: Depends on Phase 3 (cardinality enforcement is in `postBatch`/`extractLevel`).
- **Phase 7 (Polish)**: Depends on Phases 3–6 all complete. T039 is the absolute final task.

### User Story Dependencies

- **US1 (P1)**: Only depends on Phase 1+2. All other stories build on US1.
- **US2 (P1)**: Can validate in parallel with US3/US4 after US1 implementation exists.
- **US3 (P2)**: Additive to US1 — T026/T027 extend `WithLokiFromEnv` and `postBatch`.
- **US4 (P2)**: Additive to US1 — T031/T032 verify `extractLevel` and `postBatch` label map.

### Within Each Phase

- **TDD gate**: All test tasks marked must FAIL before the implementation tasks in the same phase begin.
- **Phase 2**: T005–T012 are all [P] and can be written in any order.
- **Phase 3**: T013 → T014 → T015 (background goroutine needs Write; postBatch needs goroutine); T016–T019 can be written once T015 scaffolding exists.

### Parallel Opportunities

- All Phase 2 test-writing tasks (T005–T012) can run in parallel.
- US3 test tasks T024/T025 and US4 test tasks T029/T030 can all run in parallel (different test functions, same file).
- Polish tasks T033, T034, T035 can run in parallel (different files).
- T036, T037, T038 must run sequentially (linter → format → full test suite).

---

## Parallel Example: Phase 2 Test Writing

```
# All eight tests can be drafted simultaneously:
Task: "TestLokiNoop_NoEnvVar skeleton in loki_test.go"          (T006)
Task: "TestLokiImportIsolation skeleton in loki_test.go"         (T007)
Task: "TestLokiWriter_PushesOnFlush skeleton in loki_test.go"    (T008)
Task: "TestLokiWriter_NetworkFailure skeleton in loki_test.go"   (T009)
Task: "TestLokiWriter_BufferFull skeleton in loki_test.go"       (T010)
Task: "TestLokiWriter_Race skeleton in loki_test.go"             (T011)
Task: "FuzzLokiWrite skeleton in loki_test.go"                   (T012)
```

## Parallel Example: Phase 3 US1 Tests+Types

```
# Type definitions and test writing can overlap:
Task: "Define lokiWriter struct skeleton in loki.go"             (T005)
Task: "TestLokiNoop_NoEnvVar in loki_test.go"                    (T006)
Task: "TestLokiWriter_PushesOnFlush in loki_test.go"             (T008)
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2 — both P1)

1. Complete Phase 1: `logger.go` extension points (T001–T004)
2. Complete Phase 2: Write all failing tests (T005–T012)
3. Complete Phase 3: Implement core push (T013–T020) — US1 done
4. Complete Phase 4: Validate import isolation (T021–T023) — US2 done
5. **STOP and VALIDATE**: run `go test -race ./...` — MVP complete

### Full Delivery (all four stories)

6. Complete Phase 5: Auth + TLS (T024–T028) — US3 done
7. Complete Phase 6: Cardinality discipline (T029–T032) — US4 done
8. Complete Phase 7: Polish + ADR retirement (T033–T039)

---

## Notes

- `[P]` tasks = independent files, no unsatisfied task dependencies — safe to parallelize
- `[USN]` label maps to user story N from `specs/007-loki-direct-push/spec.md`
- Constitution Principle VII mandates TDD: every implementation task in a phase presupposes its test tasks in that phase exist and fail
- `go test -race ./...` is REQUIRED (not optional) before closing any phase checkpoint
- T039 (ADR retirement) is the unconditional final task; do not delete the ADR file before all other tasks are complete
- `make doc-coverage` ratchet in `internal/cmd/doccover/baseline.txt` must include `WithLokiFromEnv` and `Flush` after T035

---

## Task Summary

| Phase | Tasks | User Story | Parallelizable |
|-------|-------|-----------|----------------|
| 1 — Setup | T001–T004 | (shared) | T001 is serial; T002–T003 follow sequentially |
| 2 — Foundational | T005–T012 | (shared) | T005–T012 all [P] |
| 3 — US1 Core Push | T013–T020 | US1 | T013–T019 are sequential; T020 verifies |
| 4 — US2 Isolation | T021–T023 | US2 | T022–T023 are [P] |
| 5 — US3 Auth/TLS | T024–T028 | US3 | T024–T025 are [P] |
| 6 — US4 Cardinality | T029–T032 | US4 | T029–T030 are [P] |
| 7 — Polish | T033–T039 | (shared) | T033–T035 are [P] |
| **Total** | **39 tasks** | | |
