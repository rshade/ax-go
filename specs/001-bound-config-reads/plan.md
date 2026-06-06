# Implementation Plan: Bound Config Reads at the Read Boundary (1 MiB cap)

**Branch**: `001-bound-config-reads` | **Date**: 2026-06-02 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-bound-config-reads/spec.md`

## Summary

Make oversized configuration a **bounded, predictable validation failure**
instead of an unbounded memory load, satisfying the constitution's "never read
unbounded user input" mandate (Principle V / IX). A bootstrap already realizes
the core contract: `ax.ParseConfig` / `ax.ParseConfigFile` read through
`internal/config.ReadBounded` (an `io.LimitReader(r, maxBytes+1)` + length
check), default to a 1 MiB cap, expose `WithMaxConfigBytes`, and map oversize /
negative-cap failures to the `ax.Error` envelope with exit code `2`.

This feature is therefore in **reconcile-and-harden** mode (per the spec's
Assumptions). The remaining work is: (1) add `context.Context` as the first
parameter of both parse entry points so the I/O is cancelable and the error
envelope is trace-correlated (Principle X / VIII) — which replaces the bootstrap's
bare `io.LimitReader(r, maxBytes+1)` with a chunked read loop that checks
`ctx.Err()` between chunks while preserving the same `maxBytes+1` byte bound
(research D1/D2; do **not** regress to `io.ReadAll(io.LimitReader(...))`, which
cannot honor per-chunk cancelation) and map the resulting context errors to
deterministic exit codes (FR-011: `DeadlineExceeded`→`3`, `Canceled`→`1`);
(2) close the verification
gaps the spec's clarifications demand — a deterministic tripwire/counting reader,
a `-benchmem` allocation benchmark, and explicit boundary/edge tests; (3)
golden-lock the two frozen `error_code` values; (4) tighten the docs-as-contract
doc comments and demonstrate the option inside a parent example; and (5) absorb
governing ADR-0010 into `research.md` and retire it as the final task.

## Technical Context

**Language/Version**: Go `1.25.8` (module `github.com/rshade/ax-go`, package `ax`).

**Primary Dependencies**: `github.com/tailscale/hujson` (read-side parse +
`Standardize`); stdlib `io`, `os`, `encoding/json`, `math`, `context`. No new
dependency is introduced.

**Storage**: N/A (pure read helper; no persistence — Principle VI).

**Testing**: `go test -race ./...`; table-driven tests; golden-file fixtures
(`testdata/`); `testing.B` with `-benchmem`; `ExampleXxx` (gated by
`make doc-coverage` / `internal/cmd/doccover`). Fuzz of the parser surface is
explicitly **deferred** to a tracked follow-up (see Complexity Tracking).

**Target Platform**: Cross-platform Go library consumed by Cobra CLIs.

**Project Type**: Single Go library at the module root with private mechanics
under `internal/` (no `pkg/`, no `src/` — Principle X / ADR-0012).

**Performance Goals**: Memory attributable to detecting an oversize input stays
bounded near the cap (≈ `cap + 1` bytes), independent of source size — verified
by benchmark (SC-001).

**Constraints**: Bounded memory at the read boundary; a finite cap ceiling
(`MaxConfigBytesCeiling`, 1 GiB) so no unbounded read path exists; deterministic
classification and exit code (`2`); frozen public `error_code` contract; no
panic; errors wrapped with `%w`; `defer Close()` on the opened file.

**Scale/Scope**: Two exported entry points, one functional option, two frozen
error codes, one internal bounded-read helper. Small, security-critical surface.

**Governing ADR(s)**: `docs/adr/0010-input-config-hujson.md` (Input Config
Format — Hujson). Its read-path decision **and** its still-unimplemented
write-path (AST `Patch`) decision are absorbed into `research.md` (Phase 0) and
the ADR is deleted as the final task; ROADMAP item #9 (write path) will
reference this feature's `research.md` thereafter.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Relevance & Verdict |
|-----------|---------------------|
| **I. Stream Separation** | PASS. The parse helpers return errors; nothing is written to `stdout`. The caller emits the `ax.Error` envelope to `stderr` via `WriteError`. |
| **II. Deterministic Output & Exit Codes** | PASS (verified by new work). Oversize → `config_too_large`, negative cap → `config_max_bytes_invalid`, both exit `2`. Cancelation paths map deterministically (`DeadlineExceeded`→`3`, `Canceled`→`1` via `errors.Is` — FR-011/SC-006). Envelope is a struct. FR-008 determinism gets an explicit repeated-parse test; `trace_id` is the documented non-deterministic field (zero when no span is active → deterministic goldens). |
| **III. Machine Discoverability (`__schema`)** | N/A for the command tree (no new command/flag). The two `error_code` values are part of the public error-envelope contract and are golden-locked. |
| **IV. Agent-Safety Primitives** | N/A. A pure read helper has no `--idempotency-key` / `--dry-run` / mode surface. |
| **V. Asymmetric JSON I/O** | PASS — this feature **is** the principle's "config reads MUST be size-capped at the read boundary (default 1 MiB, configurable); oversized = validation error (exit 2), never OOM." Reads accept Hujson; writes are out of scope. The configurable cap is bounded by a finite safe ceiling (`MaxConfigBytesCeiling`, 1 GiB); a cap above it is rejected as `config_max_bytes_invalid`, so no unbounded read path exists. |
| **VI. ADR-Governed Scope** | PASS. Library read helper; no state, no new framework, no globals. Governing ADR-0010 is absorbed + retired through this Spec Kit feature, not edited. |
| **VII. Test-First Discipline** | PASS with one **documented deferral**. New assertions (tripwire reader, benchmark, boundary/edge/determinism tests, goldens) are written before the corresponding hardening and must fail for the right reason first. Doc-comment presence stays 100%; `ParseConfig`/`ParseConfigFile` keep their gated examples and gain a `WithMaxConfigBytes` demonstration inside a parent example. **Fuzz of the Hujson parser surface is deferred** to a tracked follow-up per the spec — recorded in Complexity Tracking. |
| **VIII. Observability & ID Discipline** | PASS (improved by the `ctx` decision). The error envelope is now built from the caller's `ctx`, so a `trace_id`/`span_id` is correlated when a span is active. No logging, no ID minting in this path. |
| **IX. Security & Resource Safety** | PASS — core intent. "Never read unbounded user input" is enforced absolutely: every cap is bounded by the finite ceiling `MaxConfigBytesCeiling` (1 GiB), and a cap above it (incl. `math.MaxInt64`) is rejected as `config_max_bytes_invalid` — there is no unbounded read path. No panic; read errors surfaced with `%w` (FR-009), distinct from oversize, with explicit oversize-vs-source precedence. Adding `ctx` adds cancelation against slow (cooperative) sources; `ErrorExitCode` maps `DeadlineExceeded`→`3` / `Canceled`→`1` (FR-011). |
| **X. Idiomatic Go & Dependency Minimalism** | PASS (resolved by the `ctx` decision). `context.Context` becomes the first parameter of both I/O entry points. Functional options used; `internal/config` for mechanics; `defer file.Close()` present; no new dependency. |

**ADR absorption gate (Constitution §Governance)**: Governing ADR-0010 is
**not** N/A, so `research.md` MUST contain a "Decision Records Absorbed" section
recording ADR-0010's decision, alternatives, and consequences (read **and**
write paths), and `tasks.md` MUST include the final ADR-retirement task. The ADR
MUST NOT be deleted until its decisions are recorded in `research.md`. **Satisfied
by design below** (Phase 0 writes the section; the retirement task is the final
task in Phase 2).

**Gate result**: PASS. One deferral (Principle VII fuzz) is documented in
Complexity Tracking with a tracking reference; it is a scope decision recorded in
the spec, not an unjustified violation. The bounded-read safety guarantee is
unconditional — the finite ceiling (`MaxConfigBytesCeiling`) leaves no
unbounded-read path, so Principle IX has no residual tension.

## Project Structure

### Documentation (this feature)

```text
specs/001-bound-config-reads/
├── plan.md              # This file (/speckit-plan output)
├── spec.md              # Feature specification (input)
├── research.md          # Phase 0 — holds "Decision Records Absorbed" for ADR-0010
├── data-model.md        # Phase 1 — the cap, input, and error-envelope entities
├── quickstart.md        # Phase 1 — consumer-facing usage walkthrough
├── contracts/
│   └── config-api.md    # Phase 1 — public API + frozen error-code contract
├── checklists/
│   └── requirements.md  # Pre-existing spec-quality checklist
└── tasks.md             # Phase 2 — created by /speckit-tasks (NOT here)
```

### Source Code (repository root)

```text
ax-go/                              # module root; public package `ax`
├── config.go                       # ParseConfig, ParseConfigFile, WithMaxConfigBytes,
│                                    #   DefaultMaxConfigBytes, MaxConfigBytesCeiling,
│                                    #   normalizeConfigReadError
│                                    #   → gains ctx params + ctx-correlated envelope
├── config_test.go                  # table-driven entry-point tests
│                                    #   → + tripwire/counting reader, boundary, zero-cap,
│                                    #     unbounded-cap, stream-default, mid-read-failure,
│                                    #     public-entry cancelation, determinism cases
├── config_bench_test.go            # NEW — testing.B (-benchmem) bounded-allocation proof (SC-001)
├── error.go                        # ax.Error envelope (shape unchanged); ErrorExitCode
│                                    #   gains context.DeadlineExceeded→3 / Canceled→1 mapping (FR-011)
├── example_test.go                 # ExampleParseConfig{,File} → add ctx + WithMaxConfigBytes demo
├── exit.go                         # Exit code constants (ExitValidation = 2)
├── internal/
│   └── config/
│       ├── config.go               # ReadBounded, Unmarshal, TooLargeError, InvalidMaxBytesError,
│       │                            #   MaxConfigBytesCeiling (canonical; re-exported by ax)
│       │                            #   → ReadBounded gains ctx + cancelation-aware bounded read
│       │                            #     + out-of-range cap (negative / > ceiling) rejection
│       └── config_test.go          # NEW — internal bounded-read unit + tripwire +
│                                    #   cancelation (already-canceled + deadline-mid-read) tests
├── testdata/
│   ├── error_envelope.golden.json  # existing generic envelope golden
│   ├── config_too_large.golden.json        # NEW — frozen config_too_large envelope
│   └── config_max_bytes_invalid.golden.json# NEW — frozen config_max_bytes_invalid envelope
├── examples/integration/main.go    # readConfig → pass cmd.Context() into ParseConfig
├── README.md                       # config section already documents the cap; retire ADR-0010 row
├── ROADMAP.md / AGENTS.md / docs/adr/0011-*.md  # update ADR-0010 references on retirement
└── docs/adr/0010-input-config-hujson.md         # DELETED as the final task (post-absorption)
```

**Structure Decision**: Single-project Go library. Public contract lives in the
root package `ax` (`config.go`); the bounded-read mechanics live in
`internal/config` so consumers cannot depend on them. No new top-level packages,
no `pkg/`/`src/`. New test artifacts sit beside the code they verify. This
matches the existing layout and ADR-0012 directory decision.

## Complexity Tracking

> Only deviations that must be justified are listed.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| **Principle VII fuzz coverage deferred** for the Hujson parser surface fronted by the bounded reader. | The spec (Assumptions) and source issue #1 explicitly scope fuzzing as a **separately tracked follow-up**, not a deliverable of this size-boundary feature. Deterministic tripwire + benchmark + boundary tests cover the bounded-read contract now. | Writing the fuzz harness in this feature would expand scope beyond the byte-cap boundary and duplicate work the follow-up will own end-to-end (Hujson + idempotency + envelope round-trip + TRACEPARENT). Deferral is recorded here and MUST be reconciled with the existing canonical tracker — `ROADMAP.md` #4 ("Fuzz tests for every parser surface") and source issue #1 — before this feature closes (task T018); no parallel/duplicate tracker is opened. |

*(No Principle X entry: the `ctx`-first decision brings both I/O entry points
into compliance rather than justifying a deviation.)*
