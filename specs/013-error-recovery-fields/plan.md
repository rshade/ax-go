# Implementation Plan: Error-envelope recovery & remediation fields

**Branch**: `013-error-recovery-fields` | **Date**: 2026-06-29 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/013-error-recovery-fields/spec.md`

## Summary

Add two optional, machine-readable recovery fields to the `ax.Error` envelope so
an LLM agent can self-correct after a failure: a three-state retry-safety signal
(`retryable`) and a relative backoff hint (`retry_after_seconds`). The fields are
added to the import-isolated `contract.Error` struct, populated through new
`ErrorOption` builders, and re-exported byte-for-byte through the root `ax`
facade. Existing recovery hints (`actionable_fix`, `suggestions`) are the
free-text surface and are unchanged; no `next_action` field is introduced. The
change is additive, `omitempty`, deterministic (no wall-clock), and guarded by
golden-file tests for both the populated and the unchanged-default shapes.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: stdlib only (`encoding/json`, `errors`, `context`,
`io`). No new dependency is required or added.

**Storage**: N/A (no persistence; ax-go is a stateless library — Constitution VI).

**Testing**: `go test -race ./...`; golden-file fixtures under `testdata/`; fuzz
(`FuzzErrorEnvelope`, `FuzzErrorEnvelopeUnmarshal`); `ExampleXxx` for doc
coverage (`make doc-coverage`); coverage gate (`make cover-check`).

**Target Platform**: Cross-platform Go library consumed by Cobra CLIs.

**Project Type**: Single Go module (public package `ax` at module root + the
import-isolated `contract` package).

**Performance Goals**: No hot-path impact; two scalar struct fields, marshaled
only on error emission. No benchmark target asserted (no allocation claim).

**Constraints**: Byte-identical output determinism (Constitution II); structs not
maps; no `float64` for the seconds value (use `int64`); `omitempty` so the
default shape is unchanged; `contract` package must stay import-isolated (no root
runtime/OTel/Loki imports); additive Go API only (API Diff workflow without the
`breaking-change-approved` label).

**Scale/Scope**: ~2 struct fields, 2 option builders in `contract`, 2 thin
re-exports in root `ax`, 1 new golden fixture, test + example updates. Net new
public API: `WithRetryable`, `WithRetryAfterSeconds` (names finalized in
data-model.md / contracts).

**Governing ADR(s)**: ADR-0002 (error envelope). **Already retired** — no file
exists in `docs/adr/` (only `0004`, `0008`, `0009` remain). Its decisions are
recorded in `research.md` → "Decision Records Absorbed" for provenance; there is
no ADR file to delete, so **no retirement task** is generated.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Status |
|-----------|------|--------|
| **I. Stream Separation** | Envelope stays on `stderr`; nothing new on `stdout` | PASS — `WriteError` target unchanged |
| **II. Deterministic Output & Exit Codes** | Byte-identical output; structs not maps; no `float64`; no wall-clock | PASS — `int64` seconds (relative), `*bool` retry-safety, both `omitempty`; exit-code mapping untouched |
| **III. Machine Discoverability** | `ax.Error` is public API guarded by golden tests | PASS — new golden fixture for the populated shape; existing fixtures unchanged |
| **IV. Agent-Safety Primitives** | No change to `--dry-run`/`--idempotency-key`/mode | PASS — out of scope, untouched |
| **V. Asymmetric JSON I/O** | Writes strict minified JSON | PASS — `json.Marshal` path unchanged |
| **VI. Library, Not Application** | No state, no new framework, no globals | PASS — pure struct/option additions |
| **VII. Test-First Discipline** | Failing tests precede implementation; golden + fuzz + example | PASS — sequenced in tasks.md |
| **VIII. Observability & ID Discipline** | No ID/label changes | PASS — N/A |
| **IX. Security & Resource Safety** | No new input surface; no unbounded reads | PASS — producer-supplied scalars only |
| **X. Idiomatic Go & Dependency Minimalism** | No new deps; idiomatic options | PASS — stdlib only; functional-option pattern reused |
| **XI. Stability & SemVer** | Additive machine-payload + additive Go API | PASS — pre-v1.0 additive; rides `0.MINOR.0`; no `breaking-change-approved` label needed |
| **XII. Deprecation Lifecycle** | No symbol deprecated/removed | PASS — N/A |

**ADR absorption gate (Constitution §Governance)**: Governing ADR-0002 is already
deleted. `research.md` contains a "Decision Records Absorbed" section recording
ADR-0002's decision, alternatives, and consequences for provenance. Because no
ADR file remains, `tasks.md` carries **no** ADR-retirement task (nothing to
delete). This satisfies the gate's intent: an ADR is never deleted before its
decisions are transcribed — here the transcription happens retroactively.

**Result**: All gates PASS. No entries in Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/013-error-recovery-fields/
├── plan.md              # This file
├── research.md          # Phase 0 — decisions + ADR-0002 absorption
├── data-model.md        # Phase 1 — Error entity new fields & validation
├── quickstart.md        # Phase 1 — producer/consumer usage
├── contracts/
│   └── error-envelope.md # Phase 1 — envelope JSON contract delta
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
contract/
└── error.go             # Add Retryable (*bool) + RetryAfterSeconds (int64) fields;
                         #   WithRetryable, WithRetryAfterSeconds option builders

error.go                 # Re-export WithRetryable, WithRetryAfterSeconds through ax facade

error_test.go            # Extend TestWriteErrorEnvelope + facade-parity test for new fields
golden_test.go           # (harness; unchanged)
error_fuzz_test.go       # Ensure new fields survive round-trip fuzz
example_test.go          # ExampleWithRetryable / inside an existing error example

testdata/
├── error_envelope.golden.json          # Unchanged (proves default shape stable)
└── error_recovery_envelope.golden.json # New — populated retryable + retry_after_seconds

README.md                # Document the two new envelope fields (public contract)
examples/integration/    # Surface fields if an error path demonstrates them
```

**Structure Decision**: Single Go module. The envelope's source of truth is
`contract/error.go` (import-isolated, spec 010); the root `error.go` is a thin
facade of type aliases + option re-exports. Both must produce byte-identical
output, enforced by `TestRootErrorEnvelopeMatchesIsolatedContractShape`. New
fields and builders land in `contract` first, then are re-exported.

## Complexity Tracking

*No constitution violations. Section intentionally empty.*
