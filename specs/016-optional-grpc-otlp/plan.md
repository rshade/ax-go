# Implementation Plan: Independently Opt-Out OTLP Export and gRPC Dial Adapter

**Branch**: `016-optional-grpc-otlp` | **Date**: 2026-07-22 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/016-optional-grpc-otlp/spec.md`

## Summary

Introduce two negative build constraints — `ax_no_otlp` and `ax_no_grpc` — that
let a root-facade consumer decline OTLP trace export and the instrumented
outbound gRPC dial helper independently. Both default off, so the default build's
public surface, machine payloads, and behaviour are byte-identical to today and
the change is purely additive under Constitution Principle XI.

Declining both drops a root-facade binary from 14,897,314 to 5,460,130 bytes
(**−63.3%**) and eliminates all 68 gRPC, 36 protobuf, 4 OTLP-proto, and 3
grpc-gateway packages — independently measured, not taken from the issue.
Tracing degrades to *no export*, never to *no tracing*.

The feature also builds `internal/cmd/surfacecheck`, a new CI gate that
type-checks the six public packages across 4 configurations × 6 GOOS/GOARCH
profiles and diffs the exported surface against a committed baseline — closing
the blind spot that build tags would otherwise open.

## Technical Context

**Language/Version**: Go 1.26.5 (`go.mod:3`)

**Primary Dependencies**: existing — `cobra`, `zerolog`, OTel SDK, `otelhttp`,
`otlptracehttp` (now gateable), `otelgrpc` + `grpc` (now gateable).
**One addition**: `golang.org/x/tools v0.47.0` promoted from indirect to direct
for `go/packages` (research.md D6; already in the module graph and `go.sum`, so
net-new upstream modules = 0).

**Storage**: N/A — no runtime persistence. One committed artifact,
`internal/cmd/surfacecheck/baseline.json`.

**Testing**: `go test -race` (table-driven, golden-file, fuzz, `testing.B`),
**plus tagged runs** — `-tags=ax_no_grpc`, `-tags=ax_no_otlp`,
`-tags=ax_no_grpc,ax_no_otlp`. `internal/testutil` `ForbiddenImport` assertions
for the dependency boundary.

**Target Platform**: 6 profiles — `{linux,darwin,windows} × {amd64,arm64}`,
`CGO_ENABLED=0`, pure Go.

**Project Type**: Go library (foundation/AX layer) with internal CI tooling.

**Performance Goals**: No runtime performance change intended. The tracked
benchmarks (`BenchmarkLoggerEmit/*`, `BenchmarkParseConfigBoundedRead/*`,
`BenchmarkBuildCommand`, `BenchmarkWriteError`) must stay within the 5% ns/op
and +1 allocs/op budget. **Build-time** cost: the surfacecheck sweep is ~7.3s
cold + 90–450ms per additional combination (measured).

**Constraints**:
- Default public surface byte-identical (no `breaking-change-approved`)
- Machine payloads byte-identical across all four configurations
- Fail-open preserved: a declined exporter never fails a command
- No mutable package-level state (drives research.md D4)
- Coverage floors met with no floor lowered

**Scale/Scope**: ~2 gated production functions, 3 new production files, 1 new
internal command (~400 LOC + tests), 1 committed baseline, test-file
re-partitioning across ~6 root-package test files, and Makefile/CI matrix work.

**Governing ADR(s)**: **N/A** — no ADR in `docs/adr/` governs this feature. The
ADR-absorption gate is satisfied vacuously; no ADR-retirement task is required.

## Constitution Check

*GATE: evaluated before Phase 0 and re-evaluated after Phase 1 design.*

| Principle | Verdict | Notes |
| --- | --- | --- |
| I. Stream Separation | ✅ PASS | Diagnostics stay on `stderr`; no new `stdout` writer. `surfacecheck` follows house style (violations→stderr, verdict→stdout). |
| II. Deterministic Output & Exit Codes | ✅ PASS | Payloads byte-identical across all four configurations (FR-012). Baseline written sorted for byte-determinism. Gate uses the standard 0/1/2 codes. |
| III. `__schema` Discoverability | ✅ PASS | Untouched; golden fixtures asserted unchanged under every configuration. |
| IV. Agent-Safety Primitives | ✅ PASS | Untouched; asserted unchanged under every configuration. |
| V. Asymmetric JSON I/O | ✅ PASS | Not touched. Baseline is strict JSON, read with a size cap per house convention. |
| VI. Library-Not-Application Scope | ✅ PASS | No new domain surface, no pluggable-backend registry (explicitly out of scope), no second framework. Build tags are a compile-time contract, not a runtime selection API. |
| VII. Test-First Discipline | ✅ PASS | Tests land first, including tagged runs. See "Test blind spot" gate below. |
| VIII. Observability & ID Discipline | ✅ PASS | W3C propagation, root span, and `trace_id`/`span_id` correlation preserved in **all** configurations (FR-009, FR-010) — this is the feature's central guarantee. |
| IX. Security & Resource Safety | ⚠️ NOTE | Principle IX names `ax.GRPCDial` as a secure-default helper. It **remains present and unchanged by default**; only an explicit consumer decline removes it, and no insecure alternative is introduced. Not a violation, but the principle's wording assumes unconditional presence — see Complexity Tracking. |
| X. Idiomatic Go & Dependency Minimalism | ⚠️ JUSTIFIED | One new direct dependency (`x/tools`). No package-level mutable state — this is why research.md D4 keeps per-`Start` once-semantics. See Complexity Tracking. |
| XI. Stability & SemVer | ✅ PASS | Default build's exported surface unchanged → non-breaking → `feat:` → minor bump. No `breaking-change-approved` needed. This is the constraint that selected build tags over a package split (research.md D1). |
| XII. Deprecation Lifecycle | ✅ N/A | Nothing deprecated or removed from the default surface. |
| **ADR absorption** | ✅ N/A | No governing ADR (`research.md` § Decision Records Absorbed). |

### Gate: test/lint blind spot (research.md D7)

**Finding**: `.golangci.yml` has no `run:` section, and neither `go vet ./...`
nor `go test -race ./...` passes tags. Files behind a custom constraint would be
invisible to all ~90 linters, vet, and the entire test suite.

**Verdict**: ⚠️ **Conditional pass.** Shipping tagged code without tagged
lint/vet/test would violate Principle VII in substance while passing it on
paper. Closing this is **in scope** and explicitly budgeted (Phase 3 below), not
deferred.

### Post-Phase-1 re-evaluation

Re-checked after `data-model.md` and `contracts/` were written. **No new
violations.** The design surfaced one spec precision issue (research.md D4:
"per process" → "per telemetry start"), which was fixed in `spec.md` rather than
silently diverged from — the alternative would have required package-level
mutable state, breaching Principle X.

## Project Structure

### Documentation (this feature)

```text
specs/016-optional-grpc-otlp/
├── plan.md                              # This file
├── spec.md                              # Feature specification
├── research.md                          # Phase 0 — D0-D10 decisions + measurements
├── data-model.md                        # Phase 1 — configuration matrix, baseline shape
├── quickstart.md                        # Phase 1 — consumer + contributor guide
├── contracts/
│   └── build-configuration.md           # Phase 1 — normative tag/surface/behaviour contract
├── checklists/
│   └── requirements.md                  # Spec quality checklist
└── tasks.md                             # Phase 2 — NOT created by /speckit-plan
```

### Source Code (repository root)

```text
.
├── http.go                              # MODIFY — narrow to HTTP helpers only
├── grpc.go                              # NEW    —  //go:build !ax_no_grpc  → GRPCDial
├── grpc_disabled.go                     # NEW    —  //go:build ax_no_grpc   → doc-only
├── http_test.go                         # MODIFY — HTTP tests only
├── grpc_test.go                         # NEW    —  //go:build !ax_no_grpc  → 3 GRPCDial tests
├── telemetry_export_test.go             # MODIFY — gate !ax_no_otlp; move shared helper out first
├── telemetry_helpers_test.go            # NEW    — untagged home for executeTelemetryCommand
├── telemetry_debug_test.go              # MODIFY — gate one test
├── telemetry_security_test.go           # MODIFY — gate !ax_no_otlp
├── execute_test.go                      # MODIFY — gate two tests
│
├── internal/telemetry/
│   ├── telemetry.go                     # MODIFY — remove otlptracehttp import + newOTLPExporter
│   ├── otlp.go                          # NEW    —  //go:build !ax_no_otlp
│   └── otlp_disabled.go                 # NEW    —  //go:build ax_no_otlp
│
├── internal/testutil/
│   └── imports.go                       # MODIFY — variadic tag support
│
├── internal/cmd/surfacecheck/           # NEW    — the surface inventory gate
│   ├── main.go
│   ├── main_test.go
│   └── baseline.json
│
├── internal/cmd/covercheck/main.go      # MODIFY — floor entry for surfacecheck
├── Makefile                             # MODIFY — surface-check target, tagged test/vet/lint, ci
├── .golangci.yml                        # MODIFY — build-tags support for the lint matrix
├── .github/workflows/ci.yml             # MODIFY — surfacecheck step, tagged test/lint matrix
├── .github/workflows/crosscompile.yml   # MODIFY — tag combinations in the matrix
├── README.md                            # MODIFY — document tags + thin-package tracing caveat
├── CONTEXT.md                           # MODIFY — same
└── examples/integration/                # MODIFY — verified tagged build
```

**Structure Decision**: No new public package and no directory restructuring.
The public package `ax` stays at the module root (Principle X); all new
mechanics live in `internal/`. Gated code is split into sibling files in the
*same* package rather than relocated, which is exactly what preserves the default
build's import path and exported surface.

## Implementation Phases

Sequenced so the size win (the feature's actual value) is not blocked on the
gate. Phases 1 and 4 are independent and may proceed in parallel.

| Phase | Scope | Delivers | Blocked by |
| --- | --- | --- | --- |
| **1. Opt-out mechanism** | `grpc.go`, `grpc_disabled.go`, `otlp.go`, `otlp_disabled.go`, test re-partitioning | FR-001–014, FR-022–024; SC-001, SC-003, SC-005, SC-006 | — |
| **2. Dependency boundary** | `internal/testutil` tag support + the zero-gRPC assertion | FR-015, FR-016; SC-002, SC-008 | Phase 1 |
| **3. Toolchain coverage** | Tagged `test`/`vet`/`lint` in Makefile + CI; crosscompile matrix | FR-017; SC-007 | Phase 1 |
| **4. Surface gate** | `internal/cmd/surfacecheck` + baseline + `make surface-check` + covercheck floor | FR-018–020; SC-009, SC-010, SC-012 | — |
| **5. Docs & example** | README, CONTEXT, integration example, measurement record | FR-025–028; SC-011 | Phases 1, 4 |

### Critical sequencing constraints

1. **`executeTelemetryCommand` must move first.** It lives at
   `telemetry_export_test.go:217` and is used by `execute_test.go`. Gating its
   file before relocating it breaks the declined build's test compilation.
2. **Both tags ship in the same release.** `ax_no_grpc` alone measures −0.03%.
   Splitting them across releases would present the first as a failed change.
3. **Fail-closed assertion before baseline generation.** Write
   surfacecheck's bogus-profile test *before* generating `baseline.json`, or a
   vacuously-passing gate bakes in a wrong baseline.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
| --- | --- | --- |
| New direct dependency `golang.org/x/tools` (Principle X: dependency minimalism) | Cross-GOOS type-checking for the surface gate. `x/tools/go/packages` is the canonical and only supported entry point. | **Stdlib `go/build` + `go/parser`** was prototyped and works (337µs, and `doccover` already uses the technique) — but it has no type resolution (misses the root package's type aliases, promoted methods, inferred constant types) and, decisively, **cannot detect that a tag combination fails to compile**, which is half the gate's value. Mitigation: `x/tools v0.47.0` is *already* the selected version in the module graph with an `h1:` in `go.sum:97-98`; measured cost is +1 direct/+2 first-party `x/*` indirect requires and +4 `go.sum` lines, with **zero** new upstream modules and zero version bumps. Precedent: `golang.org/x/perf` is already a direct dependency serving `benchcheck`. |
| First production build constraints in the repository | The only mechanism that removes a package from the link graph while leaving the default exported surface byte-identical (FR-005). | **Package split** (`ax/grpcx`) fits the existing import-isolation pattern better and is the cleaner long-term shape — but it *removes* `ax.GRPCDial`, which is breaking under Principle XI, forcing `breaking-change-approved`, a `0.MINOR.0`, and an import edit for every consumer. It also cannot address the exporter root at all. Recorded in research.md D1 as a candidate for the v1 API review. Mitigation: tag count held at exactly two, wired into existing gates rather than new ones, and the lint/vet/test blind spot they create is closed in-scope (Phase 3). |
| Principle IX names `ax.GRPCDial` as a secure-default helper, yet it becomes declinable | The helper is what links 66 of the 68 gRPC packages; leaving it unconditional forfeits the entire size win. | Not a substantive violation: `GRPCDial` remains **present, unchanged, and secure by default**, and no insecure alternative is introduced — a consumer who declines it is not offered a weaker dial path, they are offered none. Only an explicit build-time opt-out removes it. Flagged because the principle's wording presumes unconditional presence; if the tags prove durable, a PATCH-level constitution clarification is warranted, not an amendment. |
| Spec FR-008/SC-006 reworded "per process" → "per telemetry start" | Literal per-process semantics require a package-level `sync.Once`. | Principle X and Principle VI forbid mutable package-level state outright; it would also leak between parallel `-race` test cases. The distinction is invisible in practice — `Execute` calls `StartTelemetry` exactly once per process (`execute.go:121`), so once-per-start **is** once-per-process for every real invocation. Applied to `spec.md` rather than silently diverged from. |
