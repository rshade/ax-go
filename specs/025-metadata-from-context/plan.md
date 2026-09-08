# Implementation Plan: Live-Resolving Metadata Accessor for Root ax

**Branch**: `025-metadata-from-context` | **Date**: 2026-09-07 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/025-metadata-from-context/spec.md`

## Summary

Export `ax.MetadataFromContext(ctx context.Context) Metadata` from the root
package: `return contract.MetadataFromContext(withTraceMetadata(ctx))`. This is
byte-for-byte the composition `NewEnvelope` and `NewError` already use, so the
result is guaranteed to equal `NewEnvelope(ctx, x).Meta` for the same context.
Today's only exported function of that name — `contract.MetadataFromContext`
— is import-isolated from OpenTelemetry and always returns
`ZeroTraceID`/`ZeroSpanID` when called from root-package code, because it
cannot see the active span; a root user reaching for the obviously-named
function silently gets a partially wrong answer. `contract.MetadataFromContext`
also gains a doc-comment warning paragraph identical in spirit to the one
`contract.TraceIDFromContext` already carries, so the isolation boundary is
disclosed rather than discovered by surprise.

## Technical Context

**Language/Version**: Go 1.27.1

**Primary Dependencies**: Existing dependencies only — stdlib `context`;
`github.com/rshade/ax-go/contract` (already imported by every file in the
root package). No new module dependency. Implementation reuses the existing
unexported `withTraceMetadata` seam (`trace.go:66`); no new dependency on
`go.opentelemetry.io/otel` beyond what `trace.go` already imports.

**Storage**: N/A — a pure function with no state.

**Testing**: Test-first, mirroring the existing `trace_test.go` patterns:
an equality test against `NewEnvelope(...).Meta` from inside a command run
through `ax.Execute` with an active span, `--dry-run`, and an
auto-generated idempotency key; a no-active-span test asserting
`ZeroTraceID`/`ZeroSpanID`; a nil-context test mirroring
`TestWithTraceMetadataNilContextFallsBackToBackground`. Required
verification: `go test -race ./...` across all four build-tag combinations,
`go vet ./...`, `golangci-lint run`, `make doc-coverage`, `make cover-check`,
`make surface-check`. `make bench-check` is not required — this is a trivial
accessor, not a tracked hot path. No golden envelope or `__schema` shape
changes are expected, since `Metadata`'s shape does not change.

**Target Platform**: All supported ax-go consumer targets and all surface
gate profiles (`linux`, `darwin`, `windows` × `amd64`, `arm64`), under
default, `ax_no_grpc`, `ax_no_otlp`, and combined build configurations. The
new function lives in untagged `trace.go`/a new untagged file, so it is
present in all four configurations identically.

**Project Type**: Go library with a runnable Cobra integration example and
Starlight documentation site.

**Performance Goals**: No allocation or latency claim. The function performs
one context-value composition, equivalent in cost to what `NewEnvelope`
already does per call. Not a tracked benchmark.

**Constraints**:

- Resolution happens at read time inside the new function. `Execute` MUST NOT
  be changed to pre-stamp trace metadata into the context — a handler that
  opens a child span would then read a stale root-span ID (spec Edge Cases,
  FR-002).
- `contract.MetadataFromContext`'s return value MUST NOT change for any
  input; only its doc comment gains a warning paragraph (FR-008, FR-009).
- The `ax.Metadata` type alias and the `Envelope`/`Error` JSON shapes MUST NOT
  change.
- The new function's doc comment MUST state that an explicit trace/span ID
  set via `contract.WithMetadata` is superseded by an active live span,
  matching `NewEnvelope`/`NewError`'s existing (undocumented but tested)
  behavior — this feature does not change that precedence, only names it in
  the new function's contract (FR-006, FR-007).
- Keep the change root-package-only and build-tag-independent; no declined
  build may lose or re-type the export.
- Update the permanent audit and live surface baseline deliberately via
  `make surface-update`; review the diff line by line rather than trusting
  the regeneration blindly.

**Scale/Scope**: One exported function in the root package, one doc-comment
update in `contract/context.go`, table-driven tests, one `ExampleXxx`,
documentation touches (README, build-your-first-cli tutorial), one live
surface-baseline entry, one permanent-audit row. No new package, flag,
environment variable, command, payload field, dependency, goroutine, or
config type.

**Governing ADR(s)**: N/A. No ADR governs context metadata composition; this
extends the pattern root already uses for `TraceIDFromContext`/
`SpanIDFromContext` (trace.go:18-34), which itself was not ADR-governed.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | PASS | Pure accessor; touches neither stdout nor stderr. |
| II. Deterministic Output & Exit Codes | PASS | No payload/envelope shape change; `Metadata` fields and their semantics are unchanged, only a new read path is added. |
| III. Machine Discoverability via `__schema` | PASS | No command, flag, or schema metadata changes; existing goldens are unaffected. |
| IV. Agent-Safety Primitives | PASS | No idempotency, dry-run, confirmation, or mode behavior changes — the function only reads state those primitives already store. |
| V. Asymmetric JSON I/O | PASS | No input parsing or output encoding changes. |
| VI. ADR-Governed Scope — Library, Not Application | PASS | A composed context read is foundation-library scope, consistent with the existing `TraceIDFromContext`/`SpanIDFromContext` shadows; no orchestration or new ADR. |
| VII. Test-First Discipline | PASS | Failing equality/zero-value/nil-context tests precede implementation; a verified `ExampleMetadataFromContext` is added even though not gated by doc-coverage. |
| VIII. Observability & ID Discipline | PASS | Exposes existing trace/span ID resolution through a new named path; does not mix observability IDs with resource/entity IDs, and does not add a logging or Loki surface. |
| IX. Security & Resource Safety | PASS | No I/O, no new panic path, no unbounded read. |
| X. Idiomatic Go & Dependency Minimalism | PASS | `context.Context` first parameter; no new dependency; reuses the existing `withTraceMetadata` seam rather than duplicating its logic. |
| XI. Stability & SemVer | PASS | One additive exported function; `contract.MetadataFromContext`'s behavior is unchanged (doc-only edit), so no caller is affected. Non-breaking `feat:`, pre-v1 minor release. |
| XII. Deprecation Lifecycle | PASS | No symbol is deprecated, renamed, or removed. |

**ADR absorption gate (Constitution §Governance)**: PASS — Governing ADR(s) =
N/A. No ADR is touched or retired by this feature.

**Post-design re-check**: PASS. The research and contract confirm the
implementation is a direct reuse of the existing `withTraceMetadata`
composition with no new dependency, no payload shape change, and no
ambiguity in precedence rules (live span always wins, matching
`NewEnvelope`/`NewError`). No complexity exception is needed.

## Project Structure

### Documentation (this feature)

```text
specs/025-metadata-from-context/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── public-api.md
├── checklists/
│   └── requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
trace.go                            # ADD exported MetadataFromContext function
trace_test.go                       # ADD equality/zero-value/nil-context tests
contract/context.go                 # UPDATE MetadataFromContext doc comment (no-live-span warning)
example_test.go                     # ADD ExampleMetadataFromContext
README.md                           # UPDATE: mention alongside NewEnvelope
docs/src/content/docs/
└── tutorials/build-your-first-cli.md   # UPDATE: mention where NewEnvelope is introduced
internal/cmd/surfacecheck/baseline.json          # ADD reviewed MetadataFromContext row (via make surface-update)
specs/023-internalize-helpers/
└── public-surface-audit.json                    # ADD supported/live decision row
```

**Structure Decision**: Extend the existing root `ax` trace-metadata surface
(`trace.go`), which already hosts the two sibling live-resolving shadows
(`TraceIDFromContext`, `SpanIDFromContext`) and the shared `withTraceMetadata`
composition seam. No new file, package, or directory is needed — this is the
natural home for a third function in the same family.

## Complexity Tracking

*No violations — table intentionally empty.*
