# Research: Live-Resolving Metadata Accessor for Root ax

**Feature**: `025-metadata-from-context` | **Date**: 2026-09-07

**Decision Records Absorbed**: **N/A.** No ADR governs context-based metadata
composition. `trace.go`'s existing `TraceIDFromContext`/`SpanIDFromContext`
shadows of the isolated `contract` accessors were themselves added without an
ADR; this feature extends the same established, non-ADR-governed pattern with
a third function. No ADR is absorbed or retired.

All Technical Context questions are resolved below; no `NEEDS CLARIFICATION`
markers remain.

## D1 — Public API shape and implementation

**Decision**: Add exactly one root-package function to `trace.go`:

```go
// MetadataFromContext returns the envelope metadata ax would emit for ctx:
// the live OpenTelemetry trace and span IDs (ZeroTraceID and ZeroSpanID when
// no span is active) merged with the dry-run state and idempotency key that
// Execute stores. A trace or span ID a caller stored explicitly through
// contract.WithMetadata is superseded by the live span, exactly as NewEnvelope
// and NewError already behave.
//
// contract.MetadataFromContext cannot see the active span — this package is
// import-isolated from the OpenTelemetry SDK — and returns zero IDs for a
// root-ax context. Use this function instead when calling from root ax code.
func MetadataFromContext(ctx context.Context) Metadata {
	return contract.MetadataFromContext(withTraceMetadata(ctx))
}
```

**Rationale**: This is byte-for-byte the composition `NewEnvelope` (json.go:18)
and `NewError` (error.go:23) already perform, so equality with
`NewEnvelope(ctx, x).Meta` is guaranteed by construction rather than asserted
by coincidence — the regression test in D4 pins that guarantee. Placing it in
`trace.go` puts it next to its two siblings (`TraceIDFromContext`,
`SpanIDFromContext`), which already shadow isolated `contract` accessors with
live-resolving root versions of the same name; this is the third and final
member of that family (`contract.MetadataFromContext` was the one getter root
did not yet shadow, per issue #212).

**Alternatives considered**:

- Reimplement metadata composition independently in the new function —
  rejected; it would duplicate `withTraceMetadata`'s logic and could drift
  from `NewEnvelope`/`NewError` over time, reintroducing exactly the kind of
  inconsistency this feature fixes.
- Have `Execute` stamp trace metadata into the context once, so any reader
  (including a plain `contract.MetadataFromContext(ctx)` call) sees live IDs —
  rejected per the issue's explicit constraint: a handler that opens a child
  span would then read a stale root-span ID from the earlier stamp. Resolution
  must stay at read time.
- Name it `LiveMetadataFromContext` or similar to disambiguate from
  `contract.MetadataFromContext` — rejected; `TraceIDFromContext` and
  `SpanIDFromContext` already establish the convention of shadowing the
  isolated name unqualified within the root package, and Go's package
  qualification (`ax.MetadataFromContext` vs `contract.MetadataFromContext`)
  already disambiguates at the call site.

## D2 — Precedence semantics and documentation

**Decision**: Document, but do not change, the existing precedence: when a
context carries both an explicitly stored trace/span ID (via
`contract.WithMetadata`) and an actively executing span, the live span wins.
This is not new behavior — `contract.normalizeMetadata` plus
`withTraceMetadata`'s unconditional overwrite already produce it for
`NewEnvelope`/`NewError` today — but it was previously undocumented at the
call-site level for a hypothetical direct caller. The new function's doc
comment states it explicitly (D1); no source change beyond the doc comment
is needed to satisfy FR-006.

**Rationale**: Naming an existing, tested behavior in the new function's
contract costs nothing and prevents a future caller from being surprised the
same way `contract.MetadataFromContext`'s undocumented isolation boundary
surprised issue #212's reporter.

**Alternatives considered**:

- Add a new precedence toggle (e.g., prefer explicit metadata over live span)
  — rejected as out of scope; the issue is explicit that `NewEnvelope`'s
  existing behavior is correct and should be mirrored, not changed.

## D3 — `contract.MetadataFromContext` doc-comment update

**Decision**: Extend `contract/context.go`'s `MetadataFromContext` doc
comment (currently lines 66-68) with a warning paragraph parallel to the one
`TraceIDFromContext` already carries (lines 84-93):

```go
// MetadataFromContext returns explicit metadata from ctx merged with dry-run
// and idempotency-key context helpers.
//
// This does not resolve an active OpenTelemetry span context: this package is
// import-isolated from the OpenTelemetry SDK and provides no live tracing, so
// the TraceID and SpanID fields reflect only metadata a caller already stored
// with WithMetadata, defaulting to ZeroTraceID/ZeroSpanID otherwise. Live
// tracing comes from the root ax package: use ax.MetadataFromContext instead
// when calling from root ax code.
func MetadataFromContext(ctx context.Context) Metadata {
```

**Rationale**: `TraceIDFromContext` and `SpanIDFromContext` already disclose
this exact limitation; `MetadataFromContext` was the one getter in the family
missing it (issue #212's stated root cause for why the bug went unnoticed).
Matching the existing warning's wording keeps the three doc comments
consistent and is a doc-only change with zero surface or behavior impact.

**Alternatives considered**:

- Leave `contract.MetadataFromContext` undocumented and rely on the new root
  function's doc comment alone to steer users — rejected; a user who never
  looks at the root package (a `contract`-only consumer, which is exactly
  what the import-isolated surface exists to support) would never see the
  warning otherwise.

## D4 — Test strategy

**Decision**: Add table-free, scenario-named tests to `trace_test.go`
(mirroring its existing style, which is one function per scenario rather than
table-driven, because each scenario needs distinct `Execute`/telemetry
setup):

1. `TestMetadataFromContextInsideExecuteMatchesNewEnvelope` — run a command
   through `ax.Execute` with `--dry-run`, read
   `ax.MetadataFromContext(cmd.Context())` and
   `ax.NewEnvelope(cmd.Context(), struct{}{}).Meta` from inside `RunE`, and
   assert the two are equal field-for-field, with non-zero `TraceID`/`SpanID`.
   This is the regression test for issue #212's reproduction.
2. `TestMetadataFromContextWithNoSpanReturnsZeroIDs` — call with
   `context.Background()` (no `Execute`, no span) and assert
   `ZeroTraceID`/`ZeroSpanID`, mirroring
   `TestTraceIDFromContextWithNoSpanReturnsZeroTraceID`.
3. `TestMetadataFromContextNilContextFallsBackToBackground` — call with a
   nil-valued `context.Context` variable and assert no panic and zero-value
   IDs, mirroring `TestWithTraceMetadataNilContextFallsBackToBackground`.

Add `ExampleMetadataFromContext` to `example_test.go`, demonstrating the
function's use with a background context (deterministic output: zero IDs),
consistent with how other simple accessors are exemplified in this package.

**Rationale**: `MetadataFromContext` is not in doccover's `requiredSymbols()`
(it enumerates only the primary constructors), so a gated example is not
required — but golangci-lint's `require-doc` still gates the doc comment, and
an encouraged, verified example gives agents/humans a copyable snippet. The
three tests map directly to the spec's acceptance scenarios and edge cases
(non-zero/matching IDs under an active span, zero IDs with none, no panic on
nil).

**Alternatives considered**:

- Table-driven single test function — rejected; each scenario needs different
  fixture setup (`ax.Execute` harness vs. bare context vs. nil context), so a
  table would need per-row setup closures that add indirection without
  reducing duplication, contrary to this repository's stated preference for
  table-driven tests only when cases share one input shape.
- Skip the `ExampleMetadataFromContext` since it is not gated — rejected;
  AGENTS.md encourages non-gated examples "where they add clarity," and this
  function's entire value proposition is best shown, not just described.

## D5 — Public surface and release classification

**Decision**: Classify `func:MetadataFromContext` as supported/live in the
permanent root surface audit
(`specs/023-internalize-helpers/public-surface-audit.json`) with signature
`func(context.Context) Metadata`, following the exact record shape already
used for `func:WithFlushFunc`. Run `make surface-update` to regenerate
`internal/cmd/surfacecheck/baseline.json` and review the diff line by line —
the new entry should show `configurations: "all"` and `profiles: "all"` since
`trace.go` is untagged and the function has no platform-specific behavior.
Land as a non-breaking `feat:` so release-please selects the next pre-v1
minor per Constitution Principle XI.

**Rationale**: This is an intentional, adopter-facing addition to root `ax`'s
already-approved public surface, not an implementation leak — directly
analogous to `WithFlushFunc`'s classification in feature 024. `contract`'s
doc-comment change carries no surface-check impact since it changes no
signature.

**Alternatives considered**:

- Skip the audit/baseline update — rejected; `make surface-check` fails
  closed on any unreviewed `added` drift.
- Treat the addition as a patch release — rejected by Constitution Principle
  XI: a non-breaking feature addition is a pre-v1 minor, not a
  bug-fixes-only patch.

## D6 — Documentation contract

**Decision**: Mention `ax.MetadataFromContext` in the README section that
introduces `ax.NewEnvelope`, and in the `build-your-first-cli` tutorial at
the point where `NewEnvelope` is first introduced, framed as "if you need the
same metadata outside of building an envelope." Do not create a new
standalone documentation page or guide section.

**Rationale**: Discoverability (spec User Story 3 / FR-011) is satisfied by
placing the mention exactly where a reader already learns about envelope
metadata, rather than by adding a new page nobody would think to visit for a
small accessor function.

**Alternatives considered**:

- Add a new dedicated guide page for context metadata — rejected as
  disproportionate to a single small accessor function; the existing
  envelope documentation is the natural and sufficient home.
- Skip documentation updates and rely on the godoc comment alone — rejected;
  the issue's acceptance criteria explicitly require the README and tutorial
  mention, and FR-011 makes it a testable requirement of this spec.
