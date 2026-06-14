---

description: "Task list for Fuzz Tests for Every Parser Surface"
---

# Tasks: Fuzz Tests for Every Parser Surface

**Input**: Design documents from `/specs/005-fuzz-parser-surfaces/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/fuzz-surfaces.md, quickstart.md

**Tests**: This feature *is* tests. Each user story delivers one or more fuzz
functions plus their committed seed corpora (US3 delivers two — see Phase 5).
The code under test already exists, so the workflow per story is: write the fuzz
function(s) (with `f.Add` seeds + invariant assertions) → author the committed
corpus → verify green. There is no separate "test task" layer because the fuzz
function is the artifact.

**Organization**: Tasks are grouped by user story (US1–US4). All four stories
touch distinct files and are fully parallelizable against one another.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US4)
- All paths are repo-root-relative (module root `github.com/rshade/ax-go`)

## Path Conventions

- Fuzz tests are white-box `package ax` files at the **module root** (matching
  existing `config_fuzz_test.go` / `telemetry_fuzz_test.go`).
- Committed corpora live under `testdata/fuzz/<FuncName>/` in the
  `go test fuzz v1` format (see contracts/fuzz-surfaces.md).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the toolchain and a clean starting point.

- [X] T001 Confirm Go native fuzzing is available and the working tree builds: run `go version` (≥ 1.18; go.mod pins 1.26.4) and `go build ./...` from repo root.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Establish the green baseline that every new fuzz function is measured against.

**⚠️ CRITICAL**: Run before any story so that any new failure is attributable to this feature.

- [X] T002 Establish green baseline: run `go test -race ./...` from repo root and confirm it passes (records the pre-change state; no code added here).

**Checkpoint**: Baseline green. All four user stories may now proceed in parallel — they share no code and touch distinct files.

> **Note**: There is no shared foundational *code* task. Each parser surface is
> independent and the implementation under test (`config.go`, `id.go`/`context.go`/`json.go`,
> `error.go`, `telemetry.go`/`trace.go`) already exists. T002 is a verification
> gate, not a build dependency.

---

## Phase 3: User Story 1 - Config Reader Is Safe Under Arbitrary Input (Priority: P1) 🎯 MVP

**Goal**: `FuzzParseConfig` proves `ParseConfig` never panics/hangs and always
classifies its outcome (typed `*ax.Error`), with explicit cap−1/cap/cap+1
boundary coverage — the bounded-reader DoS guarantee (Principle V).

**Independent Test**: `go test -run=FuzzParseConfig/ .` replays the committed
corpus with zero failures, and `go test -run=^$ -fuzz=FuzzParseConfig -fuzztime=30s .`
is panic-free.

### Implementation for User Story 1

- [X] T003 [P] [US1] Add `FuzzParseConfig(f *testing.F)` to `config_fuzz_test.go` (append below `FuzzPatchConfig`): signature `(data []byte, maxBytes int64)`; call `ParseConfig(context.Background(), bytes.NewReader(data), &map[string]any{}, WithMaxConfigBytes(maxBytes))`; add `f.Add` seeds covering valid Hujson, boundary, and invalid-cap cases; assert invariants I1–I6 from data-model.md (no panic; non-nil errors are `*ax.Error` via `errors.As`; `len(data) > maxBytes` with valid cap ⇒ `config_too_large`/exit 2; invalid cap ⇒ `config_max_bytes_invalid`/exit 2; in-bound ⇒ success or `config_invalid`; `ErrorExitCode` agrees with envelope `ExitCode()`). Include a contract-style doc comment.
- [X] T004 [US1] Author committed corpus under `testdata/fuzz/FuzzParseConfig/` (one entry per file, line 1 `[]byte("…")`, line 2 `int64(…)`): ≥ 3 entries spanning valid/boundary/invalid, and MUST include `len==cap-1`, `len==cap`, `len==cap+1` straddle entries plus an invalid-cap entry (`int64(-1)` and a value `> MaxConfigBytesCeiling`) and a malformed-Hujson entry. (depends on T003 signature)
- [X] T005 [US1] Verify US1: `go test -run=FuzzParseConfig/ .` passes and `go test -run=^$ -fuzz=FuzzParseConfig -fuzztime=30s .` finds no crashers. (depends on T003, T004)

**Checkpoint**: `FuzzParseConfig` and its corpus are green and panic-free — MVP complete.

---

## Phase 4: User Story 2 - Idempotency Key Validation Is Fuzz-Hardened (Priority: P2)

**Goal**: `FuzzIdempotencyKey` proves an arbitrary user-supplied key survives the
context+envelope round-trip unchanged and that generated keys remain UUID v4
(ADR-0007) — no validation API is introduced (research D2).

**Independent Test**: `go test -run=FuzzIdempotencyKey/ .` replays the corpus
green; `go test -run=^$ -fuzz=FuzzIdempotencyKey -fuzztime=30s .` is panic-free.

### Implementation for User Story 2

- [X] T006 [P] [US2] Create `id_fuzz_test.go` (package `ax`) with `FuzzIdempotencyKey(f *testing.F)`: signature `(key string)`; round-trip via `WithIdempotencyKey(ctx, key)` → `IdempotencyKeyFromContext` → `NewEnvelope(ctx, struct{}{})` → `json.Marshal`/`json.Unmarshal`; assert invariants I1–I3 (no panic; round-trip fidelity, accounting for `omitempty` on empty key; once-per-run `NewIdempotencyKey()` parses as UUID with `Version()==4`). Include `f.Add` seeds (real UUID v4, empty, non-UUID, Unicode) and a contract-style doc comment.
- [X] T007 [US2] Author committed corpus under `testdata/fuzz/FuzzIdempotencyKey/` (one `string("…")` per entry): ≥ 3 entries spanning a real UUID v4, empty string, non-UUID, control-char, Unicode, and a very long string. (depends on T006 signature)
- [X] T008 [US2] Verify US2: `go test -run=FuzzIdempotencyKey/ .` passes and `go test -run=^$ -fuzz=FuzzIdempotencyKey -fuzztime=30s .` finds no crashers. (depends on T006, T007)

**Checkpoint**: US1 and US2 both independently green.

---

## Phase 5: User Story 3 - Error Envelope Round-Trip Preserves the Cause Chain (Priority: P3)

**Goal**: `FuzzErrorEnvelope` proves `ax.Error` build → marshal → unmarshal is
panic-free, exported fields round-trip, and the `WithErrorCause` chain is
reachable in-process before serialization but never serialized (ADR-0002,
research D3). `FuzzErrorEnvelopeUnmarshal` proves the arbitrary-bytes
`json.Unmarshal` path never panics and is serialization-idempotent (research D7),
closing US3 acceptance scenario 3 and the empty/malformed-JSON edge case.

**Independent Test**: `go test -run='FuzzErrorEnvelope/|FuzzErrorEnvelopeUnmarshal/' .`
replays both corpora green; the `-fuzz=FuzzErrorEnvelope` and
`-fuzz=FuzzErrorEnvelopeUnmarshal` 30s runs are panic-free.

### Implementation for User Story 3

- [X] T009 [P] [US3] Create `error_fuzz_test.go` (package `ax`) with BOTH error-envelope fuzz functions (one file → one task; they are not mutually `[P]`):
  **(a)** `FuzzErrorEnvelope(f *testing.F)` — signature `(code, message, cause string)`; build `NewError(ctx, code, message, WithErrorCause(errors.New(cause)), WithErrorExitCode(ExitValidation))`, marshal → unmarshal into fresh `*ax.Error`, exercise `WriteError(buf, e)`; assert invariants I1–I6 (no panic; marshal succeeds + valid JSON; `ErrorCode`/`Message`/`SchemaVersion` round-trip; cause reachable via `errors.Is`/`Unwrap` pre-marshal; cause absent from JSON and `nil` after unmarshal; `WriteError` one JSON line + `\n`, `WriteError(buf, nil)` no-op, and `WriteError(buf, (*Error)(nil))` typed-nil panic-free).
  **(b)** `FuzzErrorEnvelopeUnmarshal(f *testing.F)` — signature `(data []byte)`; `var e ax.Error; err := json.Unmarshal(data, &e)`; assert invariants I1–I3 (no panic on any bytes; on `err==nil` re-marshal succeeds and the byte-level fixpoint `marshal(unmarshal(marshal(unmarshal(data)))) == marshal(unmarshal(data))` holds — compare **bytes**, NOT struct `DeepEqual`, to avoid the `omitempty` empty-vs-nil false positive per research D7).
  Include `f.Add` seeds and contract-style doc comments for both.
- [X] T010 [P] [US3] Author committed corpus under `testdata/fuzz/FuzzErrorEnvelope/` (three `string("…")` lines per entry): ≥ 3 entries spanning normal code/message/cause, all-empty, control-char/newline in message, and very long strings. (depends on T009 signature)
- [X] T011 [P] [US3] Author committed corpus under `testdata/fuzz/FuzzErrorEnvelopeUnmarshal/` (one `[]byte("…")` per entry): ≥ 3 entries spanning a full valid envelope JSON, `{}`, empty/whitespace, truncated/garbage bytes, and a deeply nested `context`. (depends on T009 signature; distinct directory from T010 → `[P]` with it)
- [X] T012 [US3] Verify US3: `go test -run='FuzzErrorEnvelope/|FuzzErrorEnvelopeUnmarshal/' .` passes and both `go test -run=^$ -fuzz=FuzzErrorEnvelope -fuzztime=30s .` and `go test -run=^$ -fuzz=FuzzErrorEnvelopeUnmarshal -fuzztime=30s .` find no crashers. (depends on T009, T010, T011)

**Checkpoint**: US1, US2, US3 all independently green.

---

## Phase 6: User Story 4 - TRACEPARENT/TRACESTATE Extraction Has a Committed Seed Corpus (Priority: P4)

**Goal**: Extend the existing `FuzzTraceparentExtraction` to also fuzz
`TRACESTATE` (research D4) and commit its missing seed corpus, closing the
acceptance criterion that names both headers.

**Independent Test**: `go test -run=FuzzTraceparentExtraction/ .` replays the
committed corpus green; `go test -run=^$ -fuzz=FuzzTraceparentExtraction -fuzztime=30s .`
is panic-free.

### Implementation for User Story 4

- [X] T013 [P] [US4] Modify `FuzzTraceparentExtraction` in `telemetry_fuzz_test.go`: change signature to `(traceparent, tracestate string)`; update the `WithTelemetryEnv` callback to return `traceparent` for `"TRACEPARENT"` and `tracestate` for `"TRACESTATE"`; update existing `f.Add` calls to two args and add tracestate-bearing seeds; keep invariants I1–I2 (no error from `StartTelemetry`, clean `Shutdown`, ID lengths == `ZeroTraceID`/`ZeroSpanID`).
- [X] T014 [US4] Author committed corpus under `testdata/fuzz/FuzzTraceparentExtraction/` (two `string("…")` lines per entry): ≥ 3 entries spanning a valid sampled traceparent + simple tracestate, all-zero/version-`00`, and a malformed traceparent + oversized/bad tracestate. (depends on T013 signature)
- [X] T015 [US4] Verify US4: `go test -run=FuzzTraceparentExtraction/ .` passes and `go test -run=^$ -fuzz=FuzzTraceparentExtraction -fuzztime=30s .` finds no crashers. (depends on T013, T014)

**Checkpoint**: All four parser surfaces fuzzed with committed corpora.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Whole-suite gates and validation across all stories.

- [X] T016 Run the full quality gate from repo root: `go test -race ./...`, `go vet ./...`, `golangci-lint run`, and `make doc-coverage` — all MUST be clean (SC-002, SC-004). (depends on T005, T008, T012, T015)
- [X] T017 [P] Run quickstart.md validation: execute each of the five `-fuzz=<name> -fuzztime=30s` commands and confirm zero panics (SC-005). (depends on T005, T008, T012, T015)
- [X] T018 [P] Confirm SC-001/SC-003/SC-006: every committed corpus replays via `go test ./...` (no `-fuzz`) with zero failures; each function has ≥ 3 corpus entries across valid/boundary/invalid; all five fuzz functions exist (incl. `FuzzErrorEnvelopeUnmarshal`) with `FuzzTraceparentExtraction` covering `TRACESTATE`.

> **No ADR-retirement task.** Per plan.md and research.md "Decision Records
> Referenced", ADR-0002/0004/0007 are referenced for the invariants the fuzz
> tests assert and are **not** superseded by adding tests; no ADR file is
> deleted. Adding a deletion task here would break references owned by other
> features. (Template's FINAL ADR task is correctly omitted.)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup. T002 baseline gate precedes all stories.
- **User Stories (Phase 3–6)**: All depend only on T002. They share no code and may run fully in parallel.
- **Polish (Phase 7)**: T016–T018 depend on all four story verifications (T005, T008, T012, T015).

### User Story Dependencies

- **US1 (P1)**, **US2 (P2)**, **US3 (P3)**, **US4 (P4)**: mutually independent — each owns a distinct file and corpus directory.

### Within Each User Story

- Function task (writes `f.Add` seeds + assertions) → corpus task (needs the exact signature) → verify task (replays corpus + short fuzz run).

### Parallel Opportunities

- The four function tasks **T003, T006, T009, T013** can all run in parallel (distinct files; T009 creates both error-envelope functions in one file).
- The five corpus tasks **T004, T007, T010, T011, T014** can all run in parallel (distinct directories), each after its own function task.
- Entire user stories US1–US4 can be assigned to four developers simultaneously after T002.

---

## Parallel Example: All Four Surfaces At Once

```bash
# After T002 baseline is green, launch the four function tasks in parallel:
Task: "Add FuzzParseConfig to config_fuzz_test.go"               # T003 [US1]
Task: "Create id_fuzz_test.go with FuzzIdempotencyKey"           # T006 [US2]
Task: "Create error_fuzz_test.go w/ FuzzErrorEnvelope + Unmarshal" # T009 [US3]
Task: "Extend FuzzTraceparentExtraction for TRACESTATE"          # T013 [US4]

# Then the five corpora in parallel:
Task: "Author testdata/fuzz/FuzzParseConfig/ corpus"             # T004 [US1]
Task: "Author testdata/fuzz/FuzzIdempotencyKey/ corpus"          # T007 [US2]
Task: "Author testdata/fuzz/FuzzErrorEnvelope/ corpus"           # T010 [US3]
Task: "Author testdata/fuzz/FuzzErrorEnvelopeUnmarshal/ corpus"  # T011 [US3]
Task: "Author testdata/fuzz/FuzzTraceparentExtraction/ corpus"   # T014 [US4]
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 Setup → Phase 2 baseline (T001–T002).
2. Phase 3: `FuzzParseConfig` + corpus + verify (T003–T005).
3. **STOP and VALIDATE**: the bounded-reader DoS surface — the highest-risk
   parser — is now fuzz-covered. This alone is a shippable increment.

### Incremental Delivery

1. Baseline green → add US1 (config) → US2 (idempotency) → US3 (error envelope) → US4 (traceparent/tracestate).
2. Each story is independently green and adds one parser surface's coverage.
3. Phase 7 gates the whole suite once all four land.

### Parallel Team Strategy

After T002, assign US1–US4 to four developers; each writes its fuzz function(s) +
corpus/corpora + verification with zero cross-story coordination (US3 owns two
functions in one file plus two corpora), then T016–T018 gate the merged result.

---

## Notes

- [P] tasks = different files/directories, no dependencies.
- The implementation under test already exists; the fuzz function is the deliverable, so "verify green" replaces "make a failing test pass".
- Corpus arg types MUST match each fuzz signature exactly or `go test` fails to load the corpus (see contracts/fuzz-surfaces.md).
- Doc comments on fuzz functions are encouraged (contract-style) but not gated by `godoclint`/`make doc-coverage` (test-only symbols).
- Commit after each story (function + corpus + verify) as a logical group.
- Never weaken an assertion to make a crasher pass — fix the corpus or file a real defect against the implementation.
