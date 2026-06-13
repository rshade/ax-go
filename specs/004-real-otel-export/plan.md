# Implementation Plan: Real OTel Export & Span Lifecycle

**Branch**: `004-real-otel-export` | **Date**: 2026-06-11 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/004-real-otel-export/spec.md`

## Summary

Close the gap between ax-go's *promise* of zero-telemetry-loss correlation and
its current no-op reality. Today `internal/telemetry.Start()` builds a
`TracerProvider` with **no span processor, no exporter, and no root span**, and
`Execute()` never opens a span around command execution. Two defects follow:
nothing is ever exported, and a normally-invoked command logs the all-zeros
`trace_id`/`span_id` because no span is active (spec §Input).

This feature makes the lifecycle real in three moves: (A) open a **root span**
around the whole command inside `Execute()`, forced to record via an explicit
`AlwaysSample()` sampler so correlation works with or without an inbound
`TRACEPARENT` and regardless of its sampled flag (research D1–D2); (B) attach a
**synchronous `SimpleSpanProcessor`** feeding an env-selected exporter — OTLP
HTTP when `OTEL_EXPORTER_OTLP_ENDPOINT` is set, a human-readable `stderr` debug
exporter when `AX_OTEL_DEBUG` is set, both/neither permitted — with the exporter
timeout bounded by the existing shutdown budget so a stuck collector cannot hang
the CLI (research D3–D6); and (C) make telemetry **fail-open** — a malformed
endpoint or a failed exporter degrades to the no-op path with a `stderr`
diagnostic and never changes `stdout` or the exit code (research D7), replacing
the current fail-closed `ExitInternal` branch.

The root span affects only `stderr` correlation and the separate trace export;
no `stdout` payload changes (FR-010/SC-003). Governing **ADR-0005** is absorbed
into `research.md` and retired as the final task; **ADR-0004**'s decision is
absorbed for the record but its file is retained (research determination — it is
cross-cutting and already a constitution principle).

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/rshade/ax-go`, public package
`ax` at module root; non-public mechanics under `internal/`)

**Primary Dependencies**: Existing — `go.opentelemetry.io/otel` v1.44.0,
`go.opentelemetry.io/otel/sdk` v1.44.0, `go.opentelemetry.io/otel/trace` v1.44.0,
`otelhttp`/`otelgrpc` contrib v0.69.0, `spf13/cobra`, `rs/zerolog`. **NEW
(FR-014, research D9)** — `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
v1.44.0 (+ transitive `…/otlptrace`) and
`go.opentelemetry.io/otel/exporters/stdout/stdouttrace` v1.44.0, both
OTel-canonical and version-locked in lockstep with the SDK. `resource` and
`semconv` are subpackages of the already-present `otel/sdk` + `otel` modules — no
new module.

**Storage**: N/A — no persisted state (Principle VI). Telemetry config is derived
from the environment at startup and held in a local value (FR-012).

**Testing**: `go test -race ./...` (REQUIRED, FR-013/SC-007); table-driven tests;
a capturing OTLP receiver (`httptest.Server`) to prove export-before-exit
(SC-002); a `stderr`-buffer assertion for the debug exporter (SC-005); a
fail-open test with a malformed endpoint (SC-006); a fuzz harness on the
`TRACEPARENT` extraction parser surface (Principle VII); existing `stdout`
golden/separation tests re-run unchanged to prove zero footprint (SC-003/SC-004).

**Target Platform**: any Go platform (library + Cobra integration example); CI on
`ubuntu-latest`.

**Project Type**: single Go library at module root + `examples/integration/`
Cobra CLI.

**Performance Goals**: No numeric target asserted, so no benchmark is required
(Principle VII). `SimpleSpanProcessor` deliberately adds a synchronous export at
span end; the latency is acceptable for a short-lived CLI and is the price of the
zero-loss guarantee (research D3).

**Constraints**: `stdout` byte-identical to pre-feature when nothing is
configured (SC-003) and zero telemetry bytes on `stdout` in every mode (SC-004);
flush bounded by the existing shutdown budget and never hanging (FR-007 edge,
research D4); fail-open on every telemetry failure (FR-008); race-clean under
`-race` including concurrent log + export + flush (FR-013); secure transport —
plaintext `http://` allowed, `https://` verified, **never** `InsecureSkipVerify`
(FR-004, research D5).

**Scale/Scope**: edit 3 source files (`internal/telemetry/telemetry.go`,
`telemetry.go`, `execute.go`); `http.go` unchanged (propagation already wired —
add a coverage test); ~6 new/expanded test files; 2 new dependencies; doc updates
to `README.md` + `examples/integration/`; retire `docs/adr/0005-otel-integration.md`
and update its 7 references as the final task.

**Governing ADR(s)**: `docs/adr/0005-otel-integration.md` — absorbed into
`research.md` (§Decision Records Absorbed) and **retired as the feature's final
task**. `docs/adr/0004-trace-id-format.md` — decision absorbed for the record but
**file retained** (research D10: cross-cutting, already constitution Principle
VIII, governs ADR-0002 envelopes and ADR-0007 ID strategy beyond this feature).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Status | Evidence |
|---|---|---|---|
| I | Stream Separation (NON-NEGOTIABLE) | ✅ PASS | Every telemetry path targets `stderr`: debug exporter writes to the configured `stderr` (D6), export/shutdown failures are `stderr` diagnostics (D7), exported spans go to the collector — never `stdout` (FR-009/SC-004). Existing `stdout`-separation tests re-run unchanged. |
| II | Deterministic Output & Exit Codes | ✅ PASS | The root span affects only `stderr` correlation and the separate export, never the machine payload; `trace_id`/`span_id` stay documented non-deterministic; `stdout` is byte-identical when no telemetry is configured (FR-010/SC-003). |
| III | Machine Discoverability via `__schema` | ✅ PASS | No command-tree/flag/schema change; `__schema` goldens are untouched. |
| IV | Agent-Safety Primitives | ✅ PASS | `--idempotency-key`/`--dry-run`/mode-resolution behavior is unchanged; the root span wraps but does not alter them. |
| V | Asymmetric JSON I/O | ✅ PASS | No config read/write surface changes; Hujson never reaches `stdout`. |
| VI | ADR-Governed Scope | ✅ PASS | The public-API change (new `TelemetryOption`s, fail-open `StartTelemetry`, root-span lifecycle) flows through this Spec Kit feature, **not** a new ADR; no new state, no second CLI framework, no pluggable logger. ADR-0005 retired via this feature (Governance). |
| VII | Test-First Discipline (NON-NEGOTIABLE) | ✅ PASS | Failing correlation/export/fail-open tests land before implementation; `TRACEPARENT` fuzz added; new exported `WithTelemetry*` options carry doc comments (presence gated by `godoclint`). `StartTelemetry`/`Telemetry` are currently grandfathered gaps in `internal/cmd/doccover/baseline.txt`; because this feature reshapes that primary-API surface, a verified `ExampleStartTelemetry` is added and **ratcheted off the baseline** (the options are demonstrated inside it; functional options are not gated individually). Only `StartTelemetry` is ratcheted off this feature — `ExampleStartTelemetry` satisfies the `StartTelemetry` symbol, not the `Telemetry` type, which stays grandfathered in `baseline.txt` for a later feature to burn down (tasks T024). |
| VIII | Observability & ID Discipline | ✅ PASS | This feature *is* the discipline made real: W3C context via the OTel SDK, `trace_id`/`span_id` on every line when the root span is active, cardinality split untouched, no observability/resource ID mixing. |
| IX | Security & Resource Safety | ✅ PASS | TLS verification never disabled (`http://` plaintext allowed, `https://` verified — D5); span attributes carry only low-cardinality `service.name`/`service.version`, no PII/secrets (D8); no `panic`, `%w` wrapping preserved; telemetry config is bounded env, not unbounded input. |
| X | Idiomatic Go & Dependency Minimalism | ✅ PASS | `context.Context` first on every I/O path; **no new ax-go package-level mutable state** — config enters via constructor/options and the OTel SDK global provider is set once from `StartTelemetry` (the SDK's documented model, already the existing pattern), not from `init()` or package scope (D11); two new deps justified per FR-014 (D9); version injected at build is propagated into `service.version`. |

**ADR absorption gate (Constitution §Governance)**: Governing ADR(s) ≠ N/A →
`research.md` carries a **"Decision Records Absorbed"** section transcribing
ADR-0005's *and* ADR-0004's decision, considered alternatives, and consequences,
and `tasks.md` (Phase 2) MUST include, as its FINAL task, deletion of
`docs/adr/0005-otel-integration.md` and the update of all 7 references to it
(ROADMAP ×2; ADR-0004/0007/0008/0009 cross-refs). ADR-0004 is absorbed-for-record
and **retained** (D10), so it is not deleted by this feature. No ADR is deleted
before its decision is recorded in `research.md`.

**Post-Phase-1 re-check (2026-06-11)**: design artifacts (data-model, contract,
quickstart) introduce no new violations. The contract adds exported options and
documents the fail-open behavior change; both are governed by this feature, not a
new ADR. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/004-real-otel-export/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output; holds "Decision Records Absorbed" (ADR-0005 retired, ADR-0004 retained)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   └── telemetry-api.md
├── checklists/
│   └── requirements.md  # Spec-quality checklist (already present)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/telemetry/telemetry.go   # HEART: env-resolve config → AlwaysSample sampler
                                   #   → exporter selection (OTLP HTTP / stderr debug / none)
                                   #   → SimpleSpanProcessor → resource(service.name/version)
                                   #   → fail-open construction. Returns ctx + provider (+ no-op state).
telemetry.go                      # Public: new TelemetryOptions (WithTelemetryStderr,
                                   #   WithTelemetryServiceName/Version, WithTelemetryShutdownBudget);
                                   #   fail-open StartTelemetry; Telemetry.Shutdown unchanged in shape.
execute.go                        # Open root span around root.ExecuteContext; defer ordering
                                   #   (span.End before Shutdown); wire new telemetry options from cfg;
                                   #   DROP the fail-closed telemetry-error → ExitInternal branch;
                                   #   refine span name to cmd.CommandPath() in PersistentPreRunE;
                                   #   set span status from the command outcome.
http.go                           # UNCHANGED — otelhttp/otelgrpc propagation already wired;
                                   #   add an outbound-propagation coverage test (FR-011).
trace.go / logger.go              # UNCHANGED — tracingHook already reads e.GetCtx(); the root span
                                   #   now makes those IDs non-zero (no code change, new test).

telemetry_test.go                 # + root-span-active correlation, AlwaysSample-over-unsampled-parent,
execute_test.go                   #   + fail-open (malformed endpoint), no-op stdout-unchanged,
                                   #   + stream-separation across all telemetry modes
telemetry_export_test.go          # NEW — httptest OTLP receiver: span received before exit (SC-002, SC-008)
telemetry_debug_test.go           # NEW — AX_OTEL_DEBUG → span text on stderr, nothing on stdout (SC-005)
telemetry_fuzz_test.go            # NEW — FuzzTraceparentExtraction (Principle VII parser surface)

go.mod / go.sum                   # + otlptracehttp v1.44.0, + stdouttrace v1.44.0 (FR-014)
examples/integration/main.go      # doc OTEL_EXPORTER_OTLP_ENDPOINT + AX_OTEL_DEBUG in --help/Example
README.md                         # telemetry section: correlation-by-default + the two env vars
docs/adr/0005-otel-integration.md # DELETED as the final task; 7 references updated
ROADMAP.md                        # #2 → done; "no-op scaffold (partial)" line resolved
```

**Structure Decision**: Single-project Go library layout (constitution-mandated:
public package `ax` at module root, no `pkg/`/`src/`). The exporter/processor/
sampler mechanics live in `internal/telemetry`; the public package `ax` exposes
only the `StartTelemetry` constructor, its `TelemetryOption`s, and the
`Telemetry` lifecycle type. `Execute()` owns the root-span lifecycle so the
`span.End()`-before-`Shutdown()` ordering is visible in one place.

## Workstream Sequencing

```text
A (root span + AlwaysSample in Execute/telemetry)  ──┐  US1 (P1): correlation everywhere
                                                     ├─► B depends on A (needs a recording span to export)
B (SimpleSpanProcessor + OTLP/debug exporters,       │   US2 (P2): export-before-exit
   bounded timeout, fail-open)  ─────────────────────┤   US3 (P3): debug exporter
                                                     │
C (docs: README + integration example + ROADMAP) ───┤   runs concurrently with A/B
                                                     ▼
D (FINAL): retire ADR-0005 + update its 7 references (Governance)
```

- **A before B** is the hard ordering constraint: an exporter has nothing
  meaningful to export until a recording root span exists (spec US2 rationale).
- **A alone delivers US1** (correlation with no collector) — the highest-value,
  zero-infrastructure outcome, independently testable.
- **D is last** and only runs once `research.md` records ADR-0005's decisions
  (an ADR is never deleted before absorption — Governance).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations — table intentionally empty. The two new dependencies are a
**governed addition** under FR-014 (justified in research D9), not a Principle X
violation; the synchronous-export latency and the fail-open degradation are
**spec requirements** (FR-007, FR-008), not deviations; setting the OTel SDK
global provider from a constructor is the SDK's documented model and introduces
no ax-go package-level state (research D11).
