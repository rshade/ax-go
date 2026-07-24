# Implementation Plan: Import-Isolated Logging Package

**Branch**: `017-import-isolated-logging` | **Date**: 2026-07-22 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/017-import-isolated-logging/spec.md`

## Summary

Extract the zerolog-backed logger from the root package `ax` into a new
internal implementation package `internal/logcore`, and expose it through a new
import-isolated public package `logging`. Root `ax` keeps every existing symbol
by type-aliasing and thin delegation **onto `internal/logcore` directly, never
through the public `logging` package** — the two public surfaces are siblings
over one implementation, not a chain. Root `ax` therefore does not import
`logging`, which is what lets `logging`'s parity and identity tests import root
`ax` without an import cycle. The change is non-breaking. The Loki
direct-push addon stays in root `ax` untouched; the two `*lokiWriter` type
assertions currently in `logger.go` are replaced by a general-purpose
interface, which also remediates a standing Principle VIII violation.

Measured outcome: a logging-only consumer drops from 12,017,929 bytes to
~2,250,000 bytes (−81.3%) on linux/amd64 with `-trimpath -ldflags="-s -w"`.

## Technical Context

**Language/Version**: Go 1.26.5

**Primary Dependencies**: `github.com/rs/zerolog`;
`go.opentelemetry.io/otel/trace` (trace **API** only — not the SDK); the
existing `contract` package. No new third-party dependency is introduced.

**Storage**: N/A

**Testing**: `go test -race` with table-driven tests, golden files,
`ExampleXxx` (gated by `make doc-coverage`), `testing.B -benchmem`, and
`go list -deps`-based import-isolation assertions via
`internal/testutil.AssertContractPackageIsolated`.

**Target Platform**: Cross-platform library. Size assertions are pinned to
linux/amd64; the public-surface gate runs 6 GOOS/GOARCH profiles × 4 build
configurations.

**Project Type**: Go library (single module, public packages at module root).

**Performance Goals**: No regression. The no-active-span emission path must
remain allocation-free, as asserted today by `BenchmarkLogger`; tracked
benchmarks face the existing budget of 5% `ns/op` and +1 `allocs/op`.

**Constraints**: `logging`'s dependency graph must exclude root `ax`,
`internal/telemetry`, `go.opentelemetry.io/otel/sdk`,
`go.opentelemetry.io/otel/exporters/...`, `otelhttp`, `otelgrpc`,
`google.golang.org/grpc`, and — critically for size — `net/http` and
`crypto/tls`. A logging-only consumer must stay under 3 MB stripped.

**Scale/Scope**: Two new packages, two new example commands (`examples/logging`
and its root-facade counterpart `examples/rootlogging`, which is the denominator
of the SC-002 reduction ratio), one new internal command
(`internal/cmd/sizecheck`), edits to `logger.go`, `loki.go`, `trace.go`,
`internal/testutil/imports.go`, `internal/cmd/doccover` (package-qualified
required symbols, so the new surface's gated example is genuinely gated), two
public-package allowlists, the surface baseline, and documentation.

**Governing ADR(s)**: N/A. See [ADR governance finding](#adr-governance-finding)
for the reasoning on ADR-0004, which this feature touches but is not governed
by.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Verdict | Notes |
|---|---|---|
| I. Stream Separation | **PASS** | All log output continues to `stderr`; the payload stream is untouched. FR-008 asserts it. |
| II. Deterministic Output & Exit Codes | **PASS** | No payload or exit-code change. FR-006 requires byte-identical output across both surfaces. |
| III. `__schema` Discoverability | **N/A** | No command-tree or schema surface change. |
| IV. Agent-Safety Primitives | **N/A** | No change to idempotency, dry-run, or mode resolution. |
| V. Asymmetric JSON I/O | **N/A** | No parser surface added. |
| VI. Library Scope / No Pluggable Backend | **PASS** | FR-011. No second backend, no runtime selection, no `ax.WithLogger`. See [guardrail note](#guardrail-note). |
| VII. Test-First Discipline | **PASS (binding)** | Tests land before implementation. `ExampleNewLogger` on the new primary surface is required by `make doc-coverage`; parity, isolation, and benchmark assertions are specified in Phase 1. |
| VIII. Observability & ID Discipline | **PASS via amendment 1.2.1 — and remediates an existing violation** | Two separate clauses. The named-constructor clause required a constitution amendment; see [named-constructor finding](#principle-viii-named-constructor-clause). The Loki-coupling clause is currently violated and this feature restores compliance; see [coupling finding](#principle-viii-coupling-finding). |
| IX. Security & Resource Safety | **PASS** | No new I/O, no new user input, no TLS surface. Removing `net/http` from the isolated graph strictly reduces attack surface. |
| X. Idiomatic Go & Dependency Minimalism | **PASS** | No new dependency. New non-public mechanics live under `internal/`. The new public package is authorized by this Spec Kit feature, as AGENTS.md requires. |
| XI. Stability & SemVer | **PASS** — see [apidiff finding](#principle-xi-apidiff-finding) | Additive only. Type aliases preserve identity so existing code compiles unchanged; `go-apidiff` needed a narrow false-positive correction to agree, matching the project's own v0.1.0 → v0.2.0 precedent (FR-004, FR-005, SC-003). |
| XII. Deprecation Lifecycle | **N/A** | Nothing is deprecated or removed. |

### Principle VIII named-constructor clause

Principle VIII's second bullet named a specific symbol: "Logging MUST go through
`ax.NewLogger(ctx)` (zerolog)." This feature adds `logging.NewLogger` as an equal
public entry point, so read literally the constitution forbade it.

The substance of the rule is satisfied — the alias chain means both names reach
one constructor, one implementation, one backend, and one trace-correlation hook,
so no consumer can obtain a logger that behaves differently. But the constitution
is supreme and its literal text governs, so this was resolved by **amendment
rather than reinterpretation**: constitution `1.2.0 → 1.2.1` (PATCH,
clarification) reworded the clause to admit an identity-preserving alias exposed
by an import-isolated surface, and made the no-second-backend half explicit
instead of implied. The Sync Impact Report at the head of
`.specify/memory/constitution.md` records it.

The amendment is deliberately narrow. It sanctions **one constructor reachable by
two names**; it does not sanction a second logger, a second backend, or a second
implementation. Anything of that shape remains forbidden by Principle VI, and the
amended text now says so at the point of temptation.

AGENTS.md's "The canonical constructor is `ax.NewLogger(ctx)`" is still true and
was deliberately left unchanged: `ax.NewLogger` remains canonical, and describing
`logging.NewLogger` in a derived doc before the package exists would put the repo
in an inconsistent state mid-branch. The alias is reconciled into AGENTS.md by
this feature's own documentation task, when the package it describes is real.

### Principle VIII coupling finding

Principle VIII states that Loki direct push "MUST live as a separate addon,
**never coupled into `logger.go`**."

`logger.go` currently violates this. Lines 114 and 161 both perform
`if lw, ok := s.(*lokiWriter); ok` — a type assertion against a Loki-specific
concrete type, inside `logger.go`. The coupling runs from the logger into the
addon, which is the exact direction the principle forbids.

FR-010 removes it. This feature therefore does not merely avoid a violation;
it **restores compliance** with a principle the codebase is currently
breaking. Recorded here so the change is understood as remediation rather than
incidental refactoring.

### Guardrail note

Principle VI forbids a pluggable logger backend. The design exports `Sink` and
`LabelSanctioner` from `internal/logcore` because Go's export rules make the
alternative uncompilable (proven in `research.md`). The guardrail is therefore
carried by the `internal/` import restriction — toolchain-enforced against
every external module — rather than by lowercase identifiers.

This is a **narrowing** of enforcement, honestly recorded: an ax-go maintainer
could now add a second backend without a compiler complaint, where previously
the type system would have objected. No external consumer gains any ability.
The residual risk is held by review and listed in
[Complexity Tracking](#complexity-tracking) rather than hidden.

### Principle XI apidiff finding

**The change is non-breaking. The gate disagreed, and the gate was wrong.**

This plan asserted that identity-preserving aliases would keep `go-apidiff`
silent. They do not: the tool keys type identity on the **declaring package**, so
relocating `Logger`, `Labels`, and `Option` into `internal/logcore` produces nine
raw `Incompatible changes` entries — including a `Flush` entry whose two
renderings are textually identical, which is the clearest possible evidence that
what changed is a referenced type's identity path and not any signature a
consumer writes.

The decisive evidence is the repository's own history. The v0.1.0 → v0.2.0
release performed this exact refactor for `Error`, `Mode`, `Envelope`, `Schema`,
and the config/schema option types, shipped as a plain `feat:`, and was a no-op
for adopters. `go-apidiff` across that tag boundary reports **37 findings of the
same class today**. The gate landed later (PR #82) and had never been run against
the pattern the project had already blessed.

The resolution is a narrow type-relocation classifier in
`internal/cmd/apidiff-verdict`, not a label and not a `feat!:`. It excuses only
textually-identical renderings and same-name (or prefix-stripped) relocations
**within this module**; removals, renames, signature and member changes, and
out-of-module relocations stay breaking. Excused findings are reported in their
own section rather than dropped, and `surfacecheck` continues to gate structural
changes to the relocated types across all 24 loads — so the two gates remain
complementary rather than overlapping into a hole.

Its acceptance test is the shipped release history: the classifier must rule the
v0.1.0 → v0.2.0 diff non-breaking, and does.

### ADR governance finding

`docs/adr/` retains ADR-0004 (Trace ID Format) and ADR-0008 (CLI Framework —
Cobra). Neither governs this feature, so no ADR is absorbed or retired here,
and `tasks.md` carries no ADR-retirement task.

- **ADR-0008 (Cobra)** is untouched. This feature removes Cobra from one
  consumer's link graph as a consequence of package boundaries; it makes no
  decision about the CLI framework.
- **ADR-0004 (Trace ID Format)** decides the *format* of trace and span IDs —
  32- and 16-character lowercase hex, generated by OTel's native
  `IdGenerator`. This feature changes no format and makes no competing
  decision, so it is not governed by it and must not retire it. Retiring it
  would require absorbing consequences well outside this scope, such as
  `TRACEPARENT` ingestion and error-envelope metadata.

  It does **interact** with one stated consequence: "ax-go takes a direct
  dependency on `go.opentelemetry.io/otel/sdk`." After this feature, the
  `logging` surface depends on the OTel trace **API** only. That is compatible
  — the SDK is required for ID *generation*, whereas the logger only *reads*
  an existing span context — but the interaction is recorded in `research.md`
  so a future reader does not mistake it for drift.

  ADR-0004's behavioral consequence that "when no span is active, fields carry
  zero-value valid hex strings so consumer parsers do not branch on absence"
  is a binding invariant on the relocated hook, carried into `research.md` as
  a constraint.

**Incidental defect noted, not fixed here**: `logger.go:213` cites "ADR-0009"
in a doc comment, but no such file exists in `docs/adr/` — a stale reference
left by an earlier retirement. Out of scope; worth a separate cleanup.

**Gate result: PASS.** One narrowing of enforcement is recorded in Complexity
Tracking. No unjustified violation.

## Project Structure

### Documentation (this feature)

```text
specs/017-import-isolated-logging/
├── design.md            # Pre-Spec-Kit brainstorming artifact (authoritative input)
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── logging-package.md
│   └── logcore-package.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/logcore/                 # NEW — logger implementation
├── logcore.go                    # Labels, Logger, Config, Option, New
├── sink.go                       # Sink, LabelSanctioner, flusher, Flush
├── tracing.go                    # tracingHook, applyLabels
├── logcore_test.go
└── example_test.go

logging/                          # NEW — public import-isolated surface
├── logging.go                    # aliases + thin wrappers
├── doc.go
├── example_test.go               # ExampleNewLogger (gated)
├── import_isolation_test.go
├── parity_test.go                # untagged, package logging_test: byte-identical vs root ax
└── identity_test.go              # package logging_test: alias chain compiles

internal/cmd/sizecheck/           # NEW — binary-size gate (ceiling AND ratio)
├── main.go
└── main_test.go

examples/logging/                 # NEW — isolated-surface consumer; sizecheck subject
├── main.go
└── main_test.go

examples/rootlogging/             # NEW — root-facade counterpart; the ratio denominator
├── main.go
└── main_test.go

logger.go                         # MODIFIED — delegates; no Loki symbol remains
loki.go                           # MODIFIED — lokiWriter implements logcore.Sink
trace.go                          # MODIFIED — traceIDs shared with logcore
internal/testutil/imports.go      # MODIFIED — per-surface forbidden-import sets
internal/cmd/surfacecheck/inventory.go        # MODIFIED — 7th public package
internal/cmd/apidiff-verdict/main.go          # MODIFIED — matching allowlist
internal/cmd/surfacecheck/baseline.json       # MODIFIED — reviewed baseline
examples/integration/                          # MODIFIED — second example
Makefile                                       # MODIFIED — size-check target
README.md, CONTEXT.md, AGENTS.md, ROADMAP.md   # MODIFIED — docs
```

**Structure Decision**: Single Go module, public packages at the module root
per Principle X (no `pkg/` or `src/`). `logging` joins `config`, `contract`,
`id`, `mcp`, and `schema` as the seventh gated public package; implementation
lives under `internal/logcore` exactly as `mcp` sits over `internal/mcpserver`.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Principle VI's no-pluggable-backend guardrail moves from type-enforced to path-enforced (`Sink`, `LabelSanctioner`, and `Config` exported from `internal/logcore`) | `lokiWriter` stays in package `ax` and must satisfy the sink contract across a package boundary. Go qualifies unexported method names by their defining package, so a type outside `logcore` can never satisfy an interface with an unexported method — the alternative does not compile (proven in `research.md`). | **Keep sinks entirely out of `logcore`** (root `ax` retains `logSink`/`flusher` and decorates `logcore.Logger`): requires a decorator re-wrapping on every `WithLabels`, diverges `ax.NewLogger` from `logging.NewLogger`, and makes the FR-006 byte-identical parity assertion substantially harder to make honestly. **Compile-time seal** (unexported marker method plus an embedded `SinkBase`): restores compiler enforcement but forces `lokiWriter` to embed a foreign struct, and is over-engineering for a package the toolchain already walls off. External consumers gain nothing under the chosen option; only maintainer discipline substitutes for the compiler. |
| A seventh public package | The isolation boundary *is* the deliverable; Go links at package granularity, so no in-package mechanism achieves it. | **Build tags** were rejected by the spec's Out of Scope and by precedent (#78, `mcp`): they require every consumer to know and pass a flag, whereas a package boundary delivers the reduction by import alone. Measured: the sibling feature's tag-based opt-out leaves a default build byte-identical to today. |
