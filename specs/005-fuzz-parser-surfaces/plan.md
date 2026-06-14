# Implementation Plan: Fuzz Tests for Every Parser Surface

**Branch**: `005-fuzz-parser-surfaces` | **Date**: 2026-06-13 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-fuzz-parser-surfaces/spec.md`

## Summary

AGENTS.md and Constitution Principle VII mandate a fuzz test for *every parser
surface*. Two exist (`FuzzPatchConfig`, `FuzzTraceparentExtraction`); three
mandated surfaces are uncovered and no committed seed corpora exist. This
feature adds `FuzzParseConfig` (bounded Hujson read path), `FuzzIdempotencyKey`
(idempotency-key round-trip), `FuzzErrorEnvelope` (envelope build/round-trip +
cause-chain reachability), and `FuzzErrorEnvelopeUnmarshal` (arbitrary-bytes
unmarshal + serialization idempotence); extends `FuzzTraceparentExtraction` to
also fuzz `TRACESTATE`; and commits seed corpora under `testdata/fuzz/<FuncName>/`
for all five functions. The change is **test-only** — no new public API, no
runtime behavior change, no new dependency. It hardens code that already exists.

## Technical Context

**Language/Version**: Go 1.26.4 (`go.mod`)

**Primary Dependencies**: standard-library native fuzzing (`testing.F`, Go 1.18+);
existing deps only — `github.com/google/uuid` (idempotency-key invariant),
`go.opentelemetry.io/otel` (traceparent extraction), `tidwall/hujson` via
`internal/config` (bounded reader). **No new dependency is introduced.**

**Storage**: N/A (Principle VI — the library persists no state; corpora are
test fixtures, not runtime storage)

**Testing**: Go native fuzzing. Each fuzz function carries in-code `f.Add(...)`
seeds AND committed corpus entries under `testdata/fuzz/<FuncName>/`. Seed
corpora replay under the normal `go test -race ./...` run (no `-fuzz` flag);
extended exploration is opt-in via `-fuzz=<name> -fuzztime=...`.

**Target Platform**: Any Go-supported platform; CI runs on Linux/macOS.

**Project Type**: Single Go library (public package `ax` at module root; tests
are white-box `package ax` files at the repo root, matching existing
`config_fuzz_test.go` / `telemetry_fuzz_test.go`).

**Performance Goals**: Seed-corpus replay completes in well under a second and
adds no measurable time to `go test`. Fuzz functions must never hang: every
read path is bounded (`ParseConfig` caps at ≤ `MaxConfigBytesCeiling`; no
unbounded loops).

**Constraints**: No panics on any input (the core invariant); no new public API
surface; deterministic seed replay; race-clean; corpus arg types must match each
fuzz signature exactly.

**Scale/Scope**: 5 fuzz functions total (4 new + 1 extended). 2 new test files
(`id_fuzz_test.go`, `error_fuzz_test.go` — the latter holds both
`FuzzErrorEnvelope` and `FuzzErrorEnvelopeUnmarshal`), 2 modified
(`config_fuzz_test.go`, `telemetry_fuzz_test.go`), and 5 committed corpus
directories.

**Governing ADR(s)**: ADR-0002 (error-envelope schema), ADR-0004 (trace-id
format), ADR-0007 (id strategy) — **referenced for the invariants the fuzz tests
assert, NOT retired by this feature.** None is superseded by adding tests; each
is owned by the feature that built its implementation (`error.go`, telemetry,
`id.go`). ADR-0004 is already "absorbed for the record, file retained" by feature
004. See research.md → "Decision Records Referenced". No ADR is deleted, so no
ADR-retirement task is required (consistent with the constitution's "MUST NOT be
deleted until absorbed" — here nothing is deleted).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | ✅ PASS | No new output paths. `FuzzParseConfig` reinforces that parse failures are typed `*ax.Error` (destined for stderr), never stdout leakage. |
| II. Deterministic Output & Exit Codes | ✅ PASS | `FuzzParseConfig` asserts the fixed exit-code mapping (`config_too_large`/`config_max_bytes_invalid`/`config_option_invalid` → 2) and `*ax.Error` typing. |
| III. `__schema` Discoverability | ✅ N/A | No schema surface touched. |
| IV. Agent-Safety Primitives | ✅ PASS | `FuzzIdempotencyKey` hardens the idempotency-key round-trip — an agent-safety primitive. |
| V. Asymmetric JSON I/O / bounded reads | ✅ PASS (core) | `FuzzParseConfig` is the direct verifier of the bounded-reader boundary — the DoS-resistance guarantee. |
| VI. ADR-Governed Scope — Library, Not App | ✅ PASS | Test-only. No new public API, no state, no second framework, no pluggable logger. |
| VII. Test-First Discipline (NON-NEGOTIABLE) | ✅ PASS (fulfills) | This feature *is* the "fuzz tests for every parser surface" mandate. Implementations under test already exist; new test-only code is added with seeds first. Doc comments encouraged on fuzz funcs (not gated — `FuzzTraceparentExtraction` ships without one today). |
| VIII. Observability & ID Discipline | ✅ PASS | `FuzzTraceparentExtraction` asserts trace/span-ID extraction invariants; `FuzzIdempotencyKey` asserts UUID v4 (ADR-0007). |
| IX. Security & Resource Safety | ✅ PASS | "No panics on any input" is the central assertion; `FuzzErrorEnvelopeUnmarshal` hardens the untrusted-bytes parse path; bounded reads; no PII; no `panic` in library code surfaced. |
| X. Idiomatic Go & Dependency Minimalism | ✅ PASS | No new dependency (native `testing.F` + existing `uuid`/`otel`/`hujson`); `context.Context`-first preserved. |

**Result**: No violations. Complexity Tracking table intentionally empty.

**ADR absorption gate (Constitution §Governance)**: Governing ADR(s) are
referenced, not retired. research.md contains a "Decision Records Referenced"
section recording the decision and the invariant each ADR contributes to the
fuzz assertions. Because no ADR file is deleted, no final ADR-retirement task is
created. This honors "an ADR MUST NOT be deleted until its decisions are
absorbed" (nothing is deleted) and matches feature 004's ADR-0004
absorbed-file-retained precedent.

## Project Structure

### Documentation (this feature)

```text
specs/005-fuzz-parser-surfaces/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 output — decisions + Decision Records Referenced
├── data-model.md        # Phase 1 output — fuzz inputs, invariants, corpus shapes
├── quickstart.md        # Phase 1 output — how to run/extend the fuzz suite
├── contracts/
│   └── fuzz-surfaces.md # Phase 1 output — per-function signature + invariants + corpus format
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
# Test files at module root (white-box package ax, matching existing convention)
config_fuzz_test.go        # MODIFY — add FuzzParseConfig alongside existing FuzzPatchConfig
id_fuzz_test.go            # NEW    — FuzzIdempotencyKey
error_fuzz_test.go         # NEW    — FuzzErrorEnvelope + FuzzErrorEnvelopeUnmarshal
telemetry_fuzz_test.go     # MODIFY — extend FuzzTraceparentExtraction to fuzz TRACESTATE

# Committed seed corpora (Go "go test fuzz v1" format, one entry per file)
testdata/fuzz/
├── FuzzParseConfig/             # NEW — []byte input + int64 cap entries (incl. cap-1/cap/cap+1)
├── FuzzIdempotencyKey/          # NEW — string entries (valid UUID v4, empty, non-UUID, Unicode)
├── FuzzErrorEnvelope/           # NEW — (code, message, cause) string triples
├── FuzzErrorEnvelopeUnmarshal/  # NEW — []byte entries (valid envelope, {}, empty, garbage)
└── FuzzTraceparentExtraction/   # NEW — (traceparent, tracestate) string pairs

# Implementation under test (UNCHANGED — these already exist):
#   config.go (ParseConfig)        id.go (NewIdempotencyKey)
#   context.go (With/FromContext)  json.go (NewEnvelope)
#   error.go (NewError/WriteError) telemetry.go / trace.go (StartTelemetry, *FromContext)
```

**Structure Decision**: Single-project Go library. Fuzz tests are white-box
`package ax` files at the module root, consistent with the existing
`config_fuzz_test.go` and `telemetry_fuzz_test.go`. Seed corpora live under the
Go-standard `testdata/fuzz/<FuncName>/` path so `go test` replays them
automatically. No production source file changes — the feature exclusively adds
test code and fixtures.

## Complexity Tracking

> No Constitution Check violations. Table intentionally empty.
