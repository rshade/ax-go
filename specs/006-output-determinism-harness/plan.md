# Implementation Plan: Output-Determinism Test Harness

**Branch**: `006-output-determinism-harness` | **Date**: 2026-06-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/006-output-determinism-harness/spec.md`

## Summary

Build a reusable in-process test harness that runs a command twice with identical inputs, masks
the three documented non-deterministic fields (`trace_id`, `span_id`, `idempotency_key`), and
asserts byte-identical `stdout`. The harness additionally supports NDJSON streaming output
(line-by-line comparison) and includes helpers for RFC 3339 UTC timestamp validation and
fully-typed envelope assertion. A new `internal/testutil/` package provides the comparison
logic (satisfying FR-008 reusability); `examples/integration/` hosts the determinism tests
that actually invoke `run()` for the success path and stream path.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: stdlib only — `bytes`, `encoding/json`, `regexp`, `strings`, `time`,
`testing`. No new entries in `go.mod`.

**Storage**: N/A — in-memory, test-only.

**Testing**: `go test -race ./...`

**Target Platform**: Linux/macOS CI (standard Go test runner, same runner used by the
repository).

**Project Type**: library / test infrastructure

**Performance Goals**: Entire test suite completes in under 30 seconds (SC-004). The two extra
in-process invocations per determinism test add < 100 ms each.

**Constraints**:
- In-process only (no `os/exec`; calls `run()` via `bytes.Buffer`).
- No new runtime or compile-time dependencies.
- All new code passes `go test -race`, `go vet`, and `golangci-lint run`.

**Scale/Scope**: Two integration test invocations per harness run; three determinism test
functions covering success path, break-detection, stream path, and stream line-count mismatch.

**Governing ADR(s)**: ADR-0011 (output format) — decisions absorbed into
`research.md` ("Decision Records Absorbed" section). The ADR retirement task (delete file,
update references) is the FINAL task in `tasks.md`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | ✅ PASS | Harness reads `stdout` only; never writes to `stdout` or `stderr`. |
| II. Deterministic Output | ✅ PASS | This feature *asserts* the determinism guarantee by test (SC-001, SC-002). |
| III. `__schema` | ✅ PASS | No new CLI commands; test-only feature. |
| IV. Agent-Safety Primitives | ✅ PASS | Tests use pinned `--idempotency-key`; `run()` called in-process. |
| V. Asymmetric JSON I/O | ✅ PASS | Harness reads `stdout`; never emits Hujson or modifies output. |
| VI. Library Scope | ✅ PASS | New code lives in `internal/testutil/` and `examples/integration/`. No public API change. |
| VII. Test-First Discipline | ✅ PASS | Tests land before implementation; table-driven form where applicable. |
| VIII. Observability | ✅ PASS | No observability code added; `trace_id`/`span_id` are masked inputs, not emitted. |
| IX. Security | ✅ PASS | No PII, no network, no user input in harness. |
| X. Idiomatic Go | ✅ PASS | Stdlib only; `testing.TB` accepted where needed; functional options not needed (≤2 config knobs per function). |

**ADR absorption gate**: ADR-0011 decisions absorbed in `research.md`. ADR retirement is
the final task in `tasks.md` and being executed in this session.

**Post-design re-check**: No constitution violations identified during Phase 1 design.

## Project Structure

### Documentation (this feature)

```text
specs/006-output-determinism-harness/
├── plan.md              # This file
├── research.md          # Phase 0: masking strategy, ADR-0011 decisions absorbed
├── data-model.md        # Phase 1: key entities (OutputMode, RunResult, etc.)
├── quickstart.md        # Phase 1: usage guide for new tests
├── contracts/
│   └── harness-api.md   # Phase 1: exported symbol signatures and usage contract
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code

```text
internal/testutil/
└── determinism.go          # Reusable masking + comparison + validation helpers

examples/integration/
├── main.go                 # (unchanged)
├── main_test.go            # (unchanged)
└── determinism_test.go     # New determinism tests; calls run() from package main
```

**Structure Decision**: Single-project layout. The masking/comparison helpers live in
`internal/testutil/` so any `_test.go` file within the module can import them (FR-008).
The integration invocations of `run()` stay in `examples/integration/` because `run()` is
unexported (`package main`). No `pkg/` or `src/` directories introduced.

## Complexity Tracking

No constitution violations. No complexity justification required.

---

## Implementation Notes (for `/speckit-tasks`)

The following notes inform task breakdown. They are guidance, not prescriptions; the task
generator may refine them.

### `internal/testutil/determinism.go`

**Package header & doc**:
- Package doc comment required (golangci-lint `godoclint`).
- Every exported symbol must have a doc comment.

**Compiled regex (package-level `var`, initialized with `regexp.MustCompile`)**:
- `reMaskTraceID`, `reMaskSpanID`, `reMaskIdempotencyKey`
- These are `var` (not `const`) because compiled regexes are not constants; they are safe for
  concurrent use after init.

**`MaskNonDeterministic(b []byte) []byte`**:
- Apply all three regex replacements in sequence.
- Return a new slice (no in-place mutation of the caller's buffer).

**`CompareOutputs(t testing.TB, run1, run2 []byte, mode OutputMode)`**:
- Call `t.Helper()` first.
- Guard: if either input is empty, `t.Errorf` and return.
- Mask both inputs via `MaskNonDeterministic`.
- Branch on `mode`:
  - `ModeBoundedJSON`: `bytes.Equal(masked1, masked2)`. On failure, find first diverging byte
    index with a loop; report index and up to 80 bytes of context around divergence.
  - `ModeNDJSON`: split each masked slice on `\n`, strip trailing empty element. Check line
    counts first. Then compare line-by-line; report first diverging line index.

**`ValidateTimestamps(t testing.TB, data []byte)`**:
- Call `t.Helper()`.
- `json.Unmarshal(data, &m)` where `m` is `any`. Report unmarshal error and return.
- Recursive walk helper `walkAny(t, v any)` that switches on value type:
  - `string`: attempt `time.Parse(time.RFC3339, v)`. If err == nil and zone offset != 0,
    call `t.Errorf`.
  - `map[string]any`: recurse into values.
  - `[]any`: recurse into elements.
  - others: ignore.

**`AssertFullyTyped[T any](t testing.TB, data []byte)`**:
- Call `t.Helper()`.
- `var target T; if err := json.Unmarshal(data, &target); err != nil { t.Errorf(...) }`.

### `examples/integration/determinism_test.go`

**Package**: `package main` (matches `examples/integration/`).

**Tests**:

| Test name | Description | FR/SC |
|-----------|-------------|-------|
| `TestDeterminismSuccessPath` | Runs default command twice; `CompareOutputs(ModeBoundedJSON)`. | FR-001, FR-002, FR-007, SC-002 |
| `TestDeterminismSuccessPathBreakDetection` | Runs twice normally, then mutates second masked output to inject divergence; asserts comparison detects it via a spy `testing.TB`. | SC-001 |
| `TestDeterminismStreamPath` | Runs `stream --count=3` twice; `CompareOutputs(ModeNDJSON)`. | FR-001, FR-002, FR-003, FR-007, SC-002 |
| `TestDeterminismStreamLineCountMismatch` | Runs stream twice, artificially trims one line from second output; asserts line-count mismatch is detected. | FR-004 (NDJSON) |
| `TestDeterminismTimestampValidation` | Single run; `ValidateTimestamps`; expects pass (no timestamps in current payload). | FR-005, SC-003 |
| `TestDeterminismFullyTypedEnvelope` | Single run; `AssertFullyTyped[ax.Envelope[helloPayload]]`; expects pass. | FR-006 |

**`tbSpy` helper** (unexported, in `determinism_test.go`):
- Implements `testing.TB` interface with a minimal pass-through to the real `t`, but sets a
  flag when `Errorf` or `Fatal` is called. Used for SC-001 break-detection test.
- Only the methods actually called by the harness need to be implemented; others delegate to
  the embedded `*testing.T`.

### ExampleXxx functions

- An `ExampleMaskNonDeterministic` in `internal/testutil/determinism_example_test.go` is
  REQUIRED by the constitution's `make doc-coverage` gate for primary API symbols.
  The example demonstrates the masking replacement with a `// Output:` comment.
- `ExampleCompareOutputs`, `ExampleValidateTimestamps`, `ExampleAssertFullyTyped` are encouraged
  but not gated (per constitution's two-tier example policy).

### `make doc-coverage` baseline update

After adding exported symbols in `internal/testutil/`, run `make doc-coverage` and update
`baseline.txt` if the tool uses a ratchet. The net change should be additive (new symbols have
examples), so the baseline only increases.
