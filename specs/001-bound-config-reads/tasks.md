---

description: "Task list for Bound Config Reads at the Read Boundary (1 MiB cap)"
---

# Tasks: Bound Config Reads at the Read Boundary (1 MiB cap)

**Input**: Design documents from `/specs/001-bound-config-reads/`

**Prerequisites**: plan.md (required), spec.md (user stories), research.md
(decisions D1–D8 + absorbed ADR-0010), data-model.md (entities), contracts/config-api.md

**Tests**: REQUIRED for this feature. The constitution mandates Test-First
Discipline (Principle VII) and the spec's clarifications explicitly mandate a
deterministic tripwire/counting reader, a `-benchmem` benchmark, golden-locked
error codes, and boundary/edge/determinism tests. Test tasks are written first
and MUST fail for the right reason before the corresponding hardening lands.

**Mode**: This feature is in **reconcile-and-harden** mode (plan.md Summary). A
bootstrap already realizes the core read-boundary contract; the remaining work
is (1) threading `context.Context`, (2) closing verification gaps, (3)
golden-locking the frozen error codes, (4) docs-as-contract tightening, and (5)
retiring governing ADR-0010 after its absorption into research.md.

## Format: `[ID] [P?] [Story] Description with file path`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Maps the task to a user story (US1, US2, US3) for traceability
- Setup, Foundational, and Polish tasks carry no story label

## Path Conventions

Single Go library at the module root (package `ax`), with private mechanics
under `internal/config/`. Test artifacts sit beside the code they verify. No
`pkg/`, no `src/` (ADR-0012 / Principle X).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Capture a clean pre-change baseline so the ctx refactor's failures
are attributable and the existing green tests are a known-good reference.

- [X] T001 Capture pre-change baseline: run `go build ./...` and `go test -race ./...` and confirm the existing config tests in `config_test.go` pass; confirm the `bench`, `doc-coverage`, and `lint` targets exist and run via `Makefile` (`make bench`, `make doc-coverage`, `make lint`). Record the current green state as the "fails for the right reason" reference for the Foundational signature change.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Thread `context.Context` (research D2) through both public entry
points and the internal bounded reader, leaving the tree **compiling and
green**. Every user story's tests call the ctx-bearing signatures, so this
phase BLOCKS all of Phase 3+.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 [P] Write a failing cancelation test in `internal/config/config_test.go` (NEW file) covering **both** triggers FR-010 names: (a) an already-canceled `ctx` aborts the read before the first chunk and returns `ctx.Err()` wrapped with `%w` (discoverable via `errors.Is(err, context.Canceled)`), NOT a `TooLargeError`; (b) a deadline that expires **mid-read** — a slow, multi-chunk `io.Reader` paired with a short `context.WithTimeout` — aborts *between chunks* and surfaces `context.DeadlineExceeded` (discoverable via `errors.Is`), NOT a `TooLargeError`. Case (b) is the only test that exercises the per-chunk `ctx.Err()` check the byte cap alone never reaches; it is the "stop a *slow* (cooperative) source" half of the threat model. Use a cooperative, multi-chunk reader that yields between reads — do NOT attempt a reader that blocks indefinitely inside a single `Read`, which is explicitly outside the FR-010 guarantee (research D2). The new signature will not compile yet — confirm it fails to build for that reason (FR-010, research D2/D5, Principle IX/X).
- [X] T003 Change `internal/config/config.go` `ReadBounded` to `ReadBounded(ctx context.Context, r io.Reader, maxBytes int64) ([]byte, error)`: first reject an out-of-range cap — `maxBytes < 0` OR `maxBytes > MaxConfigBytesCeiling` → `InvalidMaxBytesError` — before reading a byte (define `MaxConfigBytesCeiling = 1 << 30` (1 GiB) in `internal/config`, re-exported as `ax.MaxConfigBytesCeiling`; because every valid cap is then `≤ 1 GiB`, `maxBytes + 1` cannot overflow `int64`, so DROP the old `math.MaxInt64` `limit = maxBytes` special-case). Then read in bounded chunks up to `maxBytes + 1`, check `ctx.Err()` before each chunk read, surface a mid-read non-EOF source error wrapped with `%w` distinct from `TooLargeError`, and keep the `len > maxBytes → TooLargeError` classification. Enforce the FR-009 precedence explicitly: when a single `Read` returns both `n > 0` and a non-EOF error, check the accumulated length first — if it has reached `maxBytes + 1`, classify oversize (`TooLargeError`); only if still `≤ maxBytes` surface the source error. Make T002 pass (research D1/D2/D4/D5).
- [X] T004 Thread ctx through the public entry points in `config.go`: `ParseConfig(ctx context.Context, r io.Reader, dst any, opts ...ParseConfigOption) error` and `ParseConfigFile(ctx context.Context, path string, dst any, opts ...ParseConfigOption) error` (keep `defer file.Close()`), pass ctx into `ReadBounded`, and change `normalizeConfigReadError(ctx, err)` to build the `ax.Error` envelope from the caller's ctx — replacing the bootstrap's `context.Background()` so `trace_id`/`span_id` are correlated (FR-010, research D2, Principle VIII).
- [X] T005 Update every existing call site to the new ctx signatures so the tree compiles and stays green: the existing test calls in `config_test.go`, the example calls in `example_test.go` (`ExampleParseConfig`, `ExampleParseConfigFile`), and the integration call sites in `examples/integration/main.go` (`readConfig` already receives `ctx` — pass it into both `ax.ParseConfig(...)` calls). Run `go build ./...` and `go test -race ./...` to confirm green.

**Checkpoint**: Both entry points and `ReadBounded` take ctx; the full tree
builds and all pre-existing tests pass. User stories can now begin.

---

## Phase 3: User Story 1 - Oversized config is rejected without exhausting memory (Priority: P1) 🎯 MVP

**Goal**: An oversized configuration source becomes a bounded validation
failure; the memory attributable to detecting it stays ≈ `cap + 1` bytes,
independent of how large the source actually is.

**Independent Test**: Feed an input far larger than the default cap (e.g., 100×)
and confirm the call returns an error while a counting reader proves the parser
never requested more than `cap + 1` bytes.

### Tests for User Story 1 (write first; MUST fail for the right reason) ⚠️

- [X] T006 [P] [US1] Add internal bounded-read unit tests with a deterministic tripwire/counting `io.Reader` in `internal/config/config_test.go`: 100× cap input returns `TooLargeError` while the counting reader records ≤ `cap + 1` bytes requested (`t.Fatal` if it exceeds), an input exactly at the cap is accepted, a cap above `MaxConfigBytesCeiling` (incl. `math.MaxInt64`) is rejected as `InvalidMaxBytesError`, and a cap exactly at the ceiling is accepted with no overflow (the ceiling makes `maxBytes + 1` safe) (research D4/D7, SC-001, FR-005, FR-006).
- [X] T007 [P] [US1] Add public-entry tripwire + boundary table tests in `config_test.go`: a counting reader wrapping the source proves `ParseConfig` reads ≤ `cap + 1` bytes for a 100× input; table cases assert exactly-at-cap accepted (FR-004/SC-003), one-byte-over rejected as `config_too_large` exit `2` (FR-003), and a cap above `MaxConfigBytesCeiling` (incl. `math.MaxInt64`) is rejected as `config_max_bytes_invalid` exit `2`, while a cap exactly at the ceiling accepts a normal input with no overflow (edge cases, SC-001, FR-005). Include one case that uses the **default** cap (no `WithMaxConfigBytes`) on a stream larger than 1 MiB and confirms it is rejected as `config_too_large`, proving the 1 MiB default applies to the stream entry point and not only to the file path (FR-001). For every rejection case, also assert `dst` is left unmodified (its zero value) — the bound-check short-circuits before `json.Unmarshal`, so no partial parse and no side effects occur (US1 Acceptance Scenario 2).
- [X] T008 [P] [US1] Add `config_bench_test.go` (NEW): a `testing.B` benchmark (run via `make bench`, i.e. `-benchmem`) over inputs at 1×, 10×, and 100× the cap that **records** `B/op` / `allocs/op`, substantiating the claim that allocations stay bounded (do not scale with input size) as input grows. The benchmark *records* numbers for review; the deterministic pass/fail assertion that the parser never reads past `cap + 1` lives in the T006/T007 tripwire, not here — a `testing.B` must not gate CI on a numeric threshold (research D7, SC-001).

### Implementation for User Story 1

- [X] T009 [US1] Tighten the `ReadBounded` doc comment in `internal/config/config.go` to a contract (units = bytes; reads at most `cap + 1`; oversize → `TooLargeError`; negative cap → `InvalidMaxBytesError`; mid-read error surfaced with `%w`; the FR-009 source-vs-oversize precedence; an out-of-range cap (negative or above `MaxConfigBytesCeiling`) → `InvalidMaxBytesError`, and the finite ceiling makes `maxBytes + 1` overflow impossible so no special `math.MaxInt64` case exists), then run T006/T007 and `make bench` (T008) to confirm the T003 chunked read already satisfies the tripwire and allocation claims; adjust the chunked read only if a tripwire trips (Principle VII: no numeric claim without the benchmark).

**Checkpoint**: Oversize input is a bounded validation failure proven by both a
deterministic tripwire and a `-benchmem` benchmark. MVP is functional.

---

## Phase 4: User Story 2 - The size limit is configurable per consumer (Priority: P2)

**Goal**: A consumer adjusts the cap per parse invocation via
`WithMaxConfigBytes` with no global or residual state, and misconfigured caps
(negative) fail as validation errors rather than crashing.

**Independent Test**: Parse the same input twice — once under a cap that rejects
it and once under a cap that admits it — and confirm the outcome flips with the
configured limit, with no carryover between calls.

### Tests for User Story 2 (write first; MUST fail for the right reason) ⚠️

- [X] T010 [US2] Extend the cap table tests in `config_test.go` (table-driven): (a) raising the cap above the input size accepts input that the default rejects; (b) lowering the cap below the input size rejects (FR-002); (c) two sequential calls with different caps each honor their own limit, proving no residual/global state (SC-004); (d) zero cap: empty input is NOT rejected as `config_too_large` (it passes the size check — assert it is not the oversize error, not that it parses successfully, since empty bytes are a non-size parse error out of scope), while any non-empty input IS rejected as `config_too_large` exit `2` (research D3, edge: zero cap); (e) an out-of-range cap returns `config_max_bytes_invalid` exit `2` — assert BOTH a negative cap AND a cap above `MaxConfigBytesCeiling` (e.g., `math.MaxInt64`), plus a cap exactly at the ceiling accepted — via `var axErr *ax.Error; errors.As(err, &axErr)` on `axErr.ErrorCode` (FR-005). Also cover the file-path entry point in `ParseConfigFile` with both a default-cap rejection case (confirming the default 1 MiB applies to the file path, not only to streams — FR-001) and one override case (edge: file-path entry point).

### Implementation for User Story 2

- [X] T011 [US2] Tighten the `WithMaxConfigBytes` doc comment in `config.go` to a contract: per-invocation only (no global/residual state — SC-004); zero is a valid, honored limit that passes only empty input through the *size* check (parse semantics unchanged — empty is a non-size parse error), NOT a "use default" sentinel; a negative value is `config_max_bytes_invalid` mapped to exit `2`; a cap above `MaxConfigBytesCeiling` (1 GiB), including `math.MaxInt64`, is likewise `config_max_bytes_invalid` (there is no unbounded read path) (research D3/D4, FR-005). Run T010 to confirm the existing option machinery already satisfies the contract.

**Checkpoint**: The cap is per-call configurable across the full value range
(negative → invalid, zero → empty-only, positive → honored), proven independent
of prior invocations.

---

## Phase 5: User Story 3 - The rejection is a machine-actionable error envelope (Priority: P3)

**Goal**: Every rejection is the standard `ax.Error` envelope, discoverable via
`errors.As`, carrying the validation exit code (`2`), a stable frozen
`error_code`, an actionable fix, and the active limit as informational context —
while a mid-read source error stays distinct and chain-preserving (with explicit
oversize-vs-source precedence) and a canceled/timed-out parse maps to a
deterministic exit code (`DeadlineExceeded`→`3`, `Canceled`→`1`; FR-011).

**Independent Test**: Trigger an oversize and a negative-cap rejection; confirm
each is the standard envelope, byte-matches its golden fixture, exposes the
frozen `error_code` and exit `2` without string parsing, and that a broken
stream surfaces its own error (not an oversize classification).

### Tests for User Story 3 (write first; MUST fail for the right reason) ⚠️

- [X] T012 [US3] Create the deterministic golden fixtures `testdata/config_too_large.golden.json` and `testdata/config_max_bytes_invalid.golden.json` (NEW) — the raw library `*ax.Error` envelope JSON for each rejection, exactly as `ParseConfig`/`ParseConfigFile` return it. Pin **every** non-constant field so the fixtures are byte-stable across builds: generate with no active span so `trace_id` is the zero W3C value, and expect `tool` / `version` as **empty strings** (`""`) — `ParseConfig` is a library function and does not populate them (they are injected at the CLI emission boundary, not by the parse helper; they are not `omitempty`, so `""` serializes deterministically). Do NOT pin them to `"app"` / `"v0.1.0"` as `testdata/error_envelope.golden.json` does — that fixture is a hand-built CLI-level envelope, a different layer (F5). The envelope carries no timestamp field, so no other field needs neutralizing (research D6, FR-007).
- [X] T013 [US3] Add the golden-lock + frozen-field test in `config_test.go` using the existing `assertGolden` helper from `golden_test.go`: assert the oversize and negative-cap envelopes byte-match the T012 goldens, AND directly assert the frozen contract — `ErrorCode` equals `config_too_large` / `config_max_bytes_invalid`, `ExitCode()` is `2` (SC-002), `SchemaVersion` is `ErrorSchemaVersion` — AND assert `Context["max_bytes"]` is present but treated as informational (not part of the frozen guarantee). Confirm `errors.As(err, &axErr)` (with `var axErr *ax.Error`) discoverability and a non-empty `ActionableFix` (FR-007, SC-005, research D6).
- [X] T014 [US3] Add determinism, mid-read, and **public-entry cancelation** tests in `config_test.go`: (a) repeated parses of identical input + identical cap yield the same classification and the same `error_code` (FR-008); (b) a deliberately failing `io.Reader` (errors partway through, before the cap) has its error surfaced with the chain preserved — `errors.Is` against the source error succeeds, the result is NOT an `*ax.Error`, and it is NOT classified as `config_too_large` (FR-009, research D5); (b2) **precedence** — a reader that returns enough bytes to cross `cap + 1` AND a non-EOF error in the same `Read` is classified oversize (`config_too_large`, exit `2`), while a reader that errors *before* `cap + 1` bytes surfaces the source error (not oversize), pinning the FR-009 boundary precedence; (c) both public entry points honor cancelation at the public boundary, across **both** FR-010 triggers: (c1) **already-canceled** — `ParseConfig(canceledCtx, …)` and `ParseConfigFile(canceledCtx, …)` surface `context.Canceled` (`errors.Is(err, context.Canceled)`); (c2) **deadline-mid-read** — `ParseConfig` over a slow, multi-chunk `io.Reader` paired with a short `context.WithTimeout` surfaces `context.DeadlineExceeded` (`errors.Is(err, context.DeadlineExceeded)`) *between chunks*, proving the per-chunk `ctx.Err()` check fires at the public surface and not only inside `ReadBounded` (T002). In every case the result is NOT an `*ax.Error` and NOT `config_too_large`. (`ParseConfigFile`'s mid-read deadline rides the same shared `ReadBounded` loop covered by T002; a deterministic, cross-platform slow *file-path* source is impractical, so its mid-read path is verified internally, while its public surface is verified here via the already-canceled case.) (c3) **exit-code mapping** — assert `ax.ErrorExitCode(deadlineErr)` is `3` and `ax.ErrorExitCode(canceledErr)` is `1`, classified via `errors.Is` with the chain preserved; these assertions fail until T014b lands (FR-011, SC-006, research D9). (FR-010, research D2).

### Implementation for User Story 3

- [X] T014b [US3] Map context errors to deterministic exit codes in `error.go`: update `ErrorExitCode` to recognize `context.DeadlineExceeded` → `ExitNetwork` (`3`) and `context.Canceled` → `ExitInternal` (`1`) via `errors.Is` for non-envelope errors, so a wrapped context error no longer falls through to `ExitInternal` for the deadline case. (Refined during review remediation: an explicit `*ax.Error` envelope is consulted FIRST — its exit code wins over any sentinel buried in its cause chain, reachable via `Unwrap` since T021 — research D9/D10.) Do NOT wrap the context error in an `ax.Error` (that would break `errors.Is` against the sentinel — research D9). Add the matching unit test in `error_test.go` (a wrapped `context.DeadlineExceeded` → `3`, a wrapped `context.Canceled` → `1`, a plain non-context error → `1` unchanged). Makes T014(c3) pass (FR-011, SC-006, research D9, Principle II).
- [X] T015 [US3] Tighten the `ParseConfig` and `ParseConfigFile` doc comments in `config.go` to contracts: state the `error_code` → exit-code mapping (`config_too_large` / `config_max_bytes_invalid` → exit `2`), `errors.As` discoverability, ctx cancelation honored between chunk reads (a reader blocking inside a single `Read` is out of scope), the cancelation exit-code mapping (`context.DeadlineExceeded` → `3`, `context.Canceled` → `1` via `ErrorExitCode`; FR-011), that an out-of-range cap (negative or above `MaxConfigBytesCeiling`) is rejected as `config_max_bytes_invalid`, and that a mid-read source error is returned with its chain preserved and is NOT classified as oversize (with the FR-009 source-vs-oversize precedence). `normalizeConfigReadError` is already ctx-correlated (T004); run T013/T014 to confirm the envelope, determinism, FR-009, and FR-011 contracts hold.

**Checkpoint**: All three user stories are independently functional; the frozen
error-code contract is golden-locked and field-asserted.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Demonstrate the option in the godoc surface, sync user-facing docs,
file the deferred fuzz follow-up, run the full verification gate, and retire the
governing ADR.

- [X] T016 [P] Demonstrate `WithMaxConfigBytes` inside the parent `ExampleParseConfig` in `example_test.go` (functional options are demonstrated inside a parent example, not gated individually — AGENTS.md), keeping the `// Output:` deterministic and ctx threaded.
- [X] T017 [P] Sync user-facing docs in `README.md`: update the config section for the ctx-bearing `ParseConfig`/`ParseConfigFile` signatures and the frozen `error_code` values, and document the read-Hujson / write-strict-JSON asymmetry (absorbed ADR-0010 consequence). Do NOT edit `CHANGELOG.md` (release-please-managed).
- [X] T018 [P] [GATE] Ensure the deferred fuzz work has ONE canonical tracker before the feature closes (do NOT open a parallel item): the Hujson parser-surface fuzz mandate already lives as `ROADMAP.md` #4 ("Fuzz tests for every parser surface") and originates from source issue #1. Reconcile with those — confirm/expand ROADMAP #4 (and, if a GitHub issue is the working tracker, the issue filed from #4) to explicitly cover this feature's deferral and the related idempotency-key / envelope-round-trip / `TRACEPARENT` surfaces, and cross-link it from this feature's deferral note (plan.md Complexity Tracking, research D8). This is a **constitution-compliance gate, not optional polish**: the feature MUST NOT be marked complete while it is missing — without it the Principle VII fuzz deferral silently becomes an *unjustified* violation.
- [X] T019 Run the full verification gate per `quickstart.md` and AGENTS.md and confirm all clean: `go test -race ./...` (incl. the integration `examples/integration/main_test.go`), `make bench` (`-benchmem`, SC-001), `make doc-coverage` (`ExampleParseConfig`/`ExampleParseConfigFile` stay gated and green), and `make lint` (golangci-lint incl. godoclint `require-doc` + markdownlint).
- [X] T020 [FINAL] Retire governing ADR-0010 — ONLY now that research.md's "Decision Records Absorbed" section captures its decision, alternatives, and consequences (read AND write paths): delete `docs/adr/0010-input-config-hujson.md` and update every reference — `README.md` (ADR index row 0010), `ROADMAP.md` (#9 write path + read-path "done" line), `AGENTS.md` (the `(ADR-0010)` mention), and `docs/adr/0011-output-payload-json.md` (its cross-reference). Run markdownlint on every edited markdown file. (Constitution §Governance ADR-absorption gate.)

---

## Phase 7: Review Remediation (post-implementation code review)

**Purpose**: Close the validated findings from the comprehensive code review of
the implemented feature (decode error-chain destruction, unspecified error
codes, doccover verified-example loophole, review-skill detector path,
read-buffer growth, doc ambiguity). Tests landed before each fix.

- [X] T021 Add `cause` + `Unwrap()` + `WithErrorCause` to `ax.Error` (`error.go`) and pass the decode error into `normalizeConfigDecodeError` (`config.go`) so `errors.Is`/`errors.As` reach the underlying hujson/json error (research D10). Reorder `ErrorExitCode` so an explicit envelope exit code wins over sentinels in its cause chain (research D9 refinement). Tests: cause-chain + nil-cause (`error_test.go`), envelope-wins precedence case, `*json.UnmarshalTypeError` discovery through `config_invalid` (`config_test.go`).
- [X] T022 Freeze and golden-lock `config_invalid` + `config_option_invalid` (research D10): new `testdata/config_invalid.golden.json` and `testdata/config_option_invalid.golden.json` (raw library envelope: zero `trace_id`, empty `tool`/`version`, no `context`), `TestParseConfigErrorGoldens` extended to all four frozen codes; spec.md (FR-007, SC-005, clarifications, zero-cap edge, Assumptions), contracts/config-api.md (frozen table, goldens, doc-contract sync, out-of-scope rewrite, envelope additions), data-model.md, and quickstart.md absorbed the codes.
- [X] T023 doccover counts only verified examples: `go/doc.Examples` + `parser.ParseComments`, coverage requires an `// Output:` / `// Unordered output:` comment; stale baseline entries and unknown required symbols now FAIL (one-way ratchet); `report` takes `io.Writer` + injected required list; new `internal/cmd/doccover/main_test.go` (exit codes, unverified-example filtering, suffix resolution).
- [X] T024 Pre-allocate the bounded read buffer — `make([]byte, 0, min(limit, maxPreallocBytes))` with `maxPreallocBytes = 1 MiB + 1` (`internal/config/config.go`) — so the default cap reads with zero reallocation and larger caps grow with actual input, never allocating the ceiling up-front; large-cap growth/oversize tests + default-cap `-benchmem` benchmark (`config_bench_test.go`).
- [X] T025 Fix the review-skill detector: wrapper at `.specify/scripts/bash/detect-changed-files.sh` delegating to `.specify/extensions/review/scripts/bash/detect-changed-files.sh` (all 14 generated SKILL.md references resolve again); the detector now includes untracked files (`git ls-files --others --exclude-standard`) in both modes.
- [X] T026 Doc-contract fixes: unambiguous "no unbounded read path" phrasing in `ParseConfig`'s doc comment; `(0, nil)` → `io.ErrNoProgress` and the pre-allocation bound contracted in `ReadBounded`'s doc comment; spec.md `Status` → `Implemented`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately (T001).
- **Foundational (Phase 2)**: Depends on Setup. The ctx signature change BLOCKS
  all user stories because every story's tests call the ctx-bearing signatures.
  Internal order: T002 (failing test) → T003 (`ReadBounded` ctx) → T004 (public
  entry points + ctx-correlated envelope) → T005 (compile all call sites green).
- **User Stories (Phase 3–5)**: All depend on Foundational completion. Once it is
  done they can proceed in parallel (different concerns) or in priority order
  P1 → P2 → P3. They share `config.go` / `config_test.go`, so cross-story edits
  to the same file serialize even when conceptually independent.
- **Polish (Phase 6)**: Depends on all desired user stories.
  - T019 (verification gate) runs after all code/doc tasks.
  - **T020 (ADR-0010 retirement) is the LAST task**: it depends on every user
    story AND on research.md having already absorbed ADR-0010 (satisfied —
    research.md §"Decision Records Absorbed").

### User Story Dependencies

- **US1 (P1)**: Starts after Foundational. No dependency on US2/US3. Delivers the
  core protective value (MVP).
- **US2 (P2)**: Starts after Foundational. Exercises the same machinery US1
  hardens but via the option surface; independently testable.
- **US3 (P3)**: Starts after Foundational. Classifies the rejections US1/US2
  produce; independently testable by triggering a rejection directly.

### Within Each User Story

- Tests are written FIRST and must fail for the right reason before the
  implementation/doc-contract task makes them pass.
- US1: T006, T007, T008 (tests, parallel) → T009 (doc-contract + verify).
- US2: T010 (tests) → T011 (doc-contract + verify).
- US3: T012 (golden fixtures) → T013 (golden-lock + field assertions) → T014
  (determinism + FR-009 precedence + public-entry cancelation + exit-code
  assertions) → T014b (ErrorExitCode ctx mapping) → T015 (doc-contract + verify).

### Parallel Opportunities

- **US1 tests** T006 (`internal/config/config_test.go`), T007 (`config_test.go`),
  T008 (`config_bench_test.go`) are three different files → run in parallel.
- **Foundational** T002 can be authored in parallel with reading the code, but
  T003→T004→T005 are strictly sequential (shared signatures).
- **Polish** T016 (`example_test.go`), T017 (`README.md`), T018 (ROADMAP #4 /
  fuzz tracker) are independent → run in parallel. T019 then T020 are sequential
  and last.
- Across stories: once Foundational lands, US1/US2/US3 test-authoring can be
  split among contributors, mindful that `config_test.go` and `config.go` are
  shared (serialize edits to those two files).

---

## Parallel Example: User Story 1

```bash
# After Foundational (Phase 2) is green, launch the three US1 test files together:
Task: "T006 internal bounded-read unit + tripwire tests in internal/config/config_test.go"
Task: "T007 public-entry tripwire + boundary table tests in config_test.go"
Task: "T008 -benchmem allocation benchmark in config_bench_test.go"
# Then T009 (ReadBounded doc-contract + verification) once T006/T007/T008 exist.
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001 baseline).
2. Complete Phase 2: Foundational (T002–T005 — ctx threaded, tree green). CRITICAL.
3. Complete Phase 3: User Story 1 (T006–T009).
4. **STOP and VALIDATE**: oversize input is a bounded validation failure, proven
   by the tripwire reader and the `-benchmem` benchmark.
5. This is the deployable MVP — it closes the "never read unbounded user input"
   guardrail violation that justifies the whole feature.

### Incremental Delivery

1. Setup + Foundational → ctx-bearing API, green tree.
2. US1 → bounded oversize rejection (tripwire + bench) → MVP.
3. US2 → per-invocation configurable cap (override / zero / negative).
4. US3 → golden-locked machine-actionable error envelope + determinism + FR-009.
5. Polish → option-in-parent example, doc sync, fuzz follow-up issue, full
   verification gate, ADR-0010 retirement (final).

### Parallel Team Strategy

1. One contributor lands Setup + Foundational (sequential, shared signatures).
2. Once Foundational is green, split US1/US2/US3 test-authoring — coordinating on
   the shared `config_test.go` / `config.go` (serialize those edits).
3. Reconvene for Polish; T020 (ADR retirement) is performed last by one person.

---

## Notes

- [P] = different files, no dependency on an incomplete task.
- [Story] label maps each story task for traceability; Setup/Foundational/Polish
  carry no story label.
- This is **reconcile-and-harden**: most behavior already exists, so the bulk of
  new work is tests (tripwire, benchmark, goldens, determinism/mid-read) plus
  docs-as-contract tightening — not new control flow.
- Verify each test FAILS for the right reason before the implementation/doc task
  closes it (Constitution Principle VII).
- **Do NOT** hand-edit `CHANGELOG.md` (release-please-managed) and **do NOT**
  create or edit ADRs — ADR-0010 is *retired* (deleted) per §Governance, not
  amended.
- Fuzzing the parser surface is a tracked follow-up (T018), not a deliverable of
  this feature (plan.md Complexity Tracking, research D8).
- Avoid: vague tasks, same-file parallel conflicts (`config.go` / `config_test.go`
  are shared), and cross-story dependencies that break independent testability.
