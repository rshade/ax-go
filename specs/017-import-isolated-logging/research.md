# Phase 0 Research: Import-Isolated Logging Package

**Feature**: `017-import-isolated-logging` | **Date**: 2026-07-22

All Technical Context unknowns are resolved. No `NEEDS CLARIFICATION` markers
remain.

## R1: Where does the weight actually come from?

**Decision**: Split on the OTel **trace API** / **SDK** line. The trace API
travels with the logger; the SDK, the exporters, and everything they drag stay
in root `ax`.

**Rationale**: Measured on linux/amd64 with `-trimpath -ldflags="-s -w"`,
re-run 2026-07-22 rather than inherited from the issue.

| Component (standalone binary) | Bytes | `google.golang.org/grpc` packages pulled |
|---|---|---|
| OTel trace **API** | 1,794,210 | **0** |
| OTel **SDK** | 4,518,153 | **0** |
| `otlptracehttp` exporter | 13,558,025 | **66** |
| `google.golang.org/grpc` | 10,400,009 | — |

The counter-intuitive result is that **gRPC enters through the *HTTP* OTLP
exporter**, not through `ax.GRPCDial`. `otlptracehttp` transitively imports the
OTLP protobuf types and the grpc-gateway runtime, which import gRPC proper. The
SDK on its own pulls zero gRPC packages.

`tracingHook` needs `trace.SpanContextFromContext` to stamp `trace_id` and
`span_id`, which is API-only. So the logger can keep full trace correlation
while shedding the entire exporter tree.

**Alternatives considered**: Dropping trace correlation from the isolated
surface to avoid the OTel dependency entirely. Rejected — correlation is the
main reason to use ax's logger rather than plain zerolog, and at 1.79 MB the
API is affordable. Reimplementing span-context extraction to avoid the
dependency was rejected under Principle X (don't reimplement a battle-tested
library) and would break ADR-0004's format contract.

## R2: End-to-end size, and the effect of the sibling feature

**Decision**: Proceed. The benefit is undiminished for default builds.

**Rationale**: Issue #144 warned that if `016-optional-grpc-otlp` landed first,
this feature's benefit would fall from ~9.76 MB to ~3.2 MB, and asked for
re-measurement before starting. Measured against that feature's branch:

| Scenario | Root facade + `NewLogger` | vs. isolated (2,248,969) |
|---|---|---|
| `main` today | 12,017,929 | −9,768,960 (−81.3%) |
| `016` branch, **default** build | 12,017,929 *(identical)* | −9,768,960 (−81.3%) |
| `016` branch, `-tags ax_no_otlp,ax_no_grpc` | 4,595,977 | −2,347,008 (−51.1%) |

That feature's opt-out is a **build constraint defaulting off**, so a default
build is byte-identical to today. The erosion the issue feared applies only to
consumers who opt in, where the remaining saving is 2,347,008 bytes.

The two features are complementary rather than redundant, and the difference is
ergonomic: `016` requires a consumer to know and pass two tag names; this
feature delivers the reduction by importing a package they were going to import
anyway. A consumer can take both — the isolated surface needs neither tag,
because it never links the trees those tags decline.

**Alternatives considered**: Closing this issue in favour of `016` alone.
Rejected on the measurement above — `016` alone changes nothing for a consumer
who passes no tags, which is the default and the common case.

## R3: Can `internal/logcore` keep its sink interfaces unexported?

**Decision**: No. `Sink`, `LabelSanctioner`, and `Config`'s fields must be
exported. `flusher` can stay unexported.

**Rationale**: Issue #144 proposed keeping `logSink` and `flusher` unexported
in `logcore`, reasoning that "root `ax` can register the Loki sink because root
may import `internal/*`." That conflates two independent Go mechanisms:

- `internal/` is a **package path** rule — it governs *who may import*.
- Lowercase is an **export** rule — it governs *what is visible to any
  importer*.

Being permitted to import a package grants no extra visibility into it. Proven
by compilation, not asserted. With an unexported field:

```text
cannot refer to unexported field additionalSinks
```

After exporting both the field and the interface type, a second and deeper
failure appears:

```text
*lokiWriter does not implement logcore.Sink (unexported method drain)
```

**Unexported method names are qualified by their defining package.** A `drain`
method declared in package `ax` is a different identifier from `drain` declared
in `logcore`, so a type outside `logcore` can *never* satisfy such an
interface. This is the same mechanism the standard library uses to make
interfaces deliberately un-implementable from outside.

The same reasoning invalidates the issue's proposed replacement for the Loki
type assertion, `interface{ sanctionLabels(Labels) }`. It must be
`LabelSanctioner` with an exported `SanctionLabels`.

`flusher` is exempt because `zerologLogger` moves into `logcore` alongside it —
same-package satisfaction needs no export.

**Alternatives considered**:

1. **Sinks stay entirely in root `ax`**; `logcore` takes a plain `io.Writer`
   and root decorates the returned logger. Rejected: needs a decorator that
   re-wraps on every `WithLabels`, and diverges the two constructors, which
   undermines the FR-006 byte-identical parity claim.
2. **Compile-time seal** — exported `Sink` carrying an unexported marker method
   satisfied only by embedding a `logcore.SinkBase`. Rejected as
   over-engineering for a package the toolchain already walls off, and it
   forces `lokiWriter` to embed a foreign struct.

## R4: How is Principle VI's guardrail preserved once `Sink` is exported?

**Decision**: Rely on the `internal/` import restriction. The public `logging`
package never re-exports `Sink` or `LabelSanctioner`.

**Rationale**: No module outside `github.com/rshade/ax-go` can import
`internal/logcore` — the Go toolchain refuses it. So no external consumer can
register a backend, which is exactly what Principle VI forbids. This mirrors
`mcp` over `internal/mcpserver`.

**Consequence, recorded rather than glossed**: enforcement narrows from
type-level to path-level. An ax-go maintainer could add a second backend
without a compiler complaint. Accepted and held by review; see the plan's
Complexity Tracking.

## R5: Why must `Config`'s fields be exported rather than hidden behind methods?

**Decision**: Export the fields.

**Rationale**: `loki.go` constructs `errorWriter: &cfg.writer` — it captures
the **address** of the writer field, so a `WithLoggerWriter` applied later is
still observed by the Loki error path. That pointer aliasing is what makes
option order irrelevant, and it is load-bearing for the FR-009 guarantee that
configuration order does not change behavior. An `AddSink()`-style accessor
API cannot preserve it without additional indirection that buys nothing inside
an `internal/` package.

**Alternatives considered**: `func (c *Config) AddSink(Sink)` with unexported
fields — tighter encapsulation, but breaks the `&cfg.writer` aliasing and
therefore risks a silent option-order regression.

## R6: Should `LabelSanctioner` be folded into `Sink`?

**Decision**: Keep it separate and type-asserted.

**Rationale**: `specs/007-loki-direct-push/research.md` (which absorbed
ADR-0006) requires that the additional-sink seam stay fully generic and not
become Loki-specific. A future sink — a file rotator, a ring buffer — has no
concept of label promotion. Folding label sanctioning into `Sink` would force
every sink to implement a Loki-shaped method, which is precisely the coupling
that requirement forbids. FR-010 encodes this.

**Alternatives considered**: A single fat `Sink` interface. Rejected as a
direct violation of the absorbed ADR-0006 D2 requirement.

## R7: Should `logging` export `Flush`?

**Decision**: Yes, for surface parity. `ax.Flush` stays a `func` that delegates
to `logcore.Flush`.

**Delegation direction (binding)**: both public surfaces are **siblings over
`internal/logcore`**, not a chain. `ax.Flush` calls `logcore.Flush`, and
`logging.Flush` calls `logcore.Flush`; `ax.Logger` aliases `logcore.Logger`, and
`logging.Logger` aliases `logcore.Logger`. Root `ax` MUST NOT import the public
`logging` package. Two reasons: a public package is a worse dependency for the
root runtime than an internal one, and root importing `logging` would make
`logging`'s own parity test — which must import `ax` to compare the two surfaces
— an import cycle. Identity is preserved either way, so this is a structural
choice, not a semantic one; it is recorded because the wrong reading breaks the
build rather than merely reading oddly.

**Rationale**: The sink seam is generic, not Loki-specific, so a drain
operation belongs on the surface that owns the logger. Parity also keeps the
two surfaces teachable as one API.

**Accepted trade-off**: Loki is the only `Sink` that exists and is unreachable
from `logging`, so for every currently-possible isolated consumer
`logging.Flush` takes the negative branch and returns `nil`. FR-012 requires
the doc comment to say so plainly, so no caller concludes their logs shipped.

**Implementation constraint**: `ax.Flush` must remain a **function**, not
become `var Flush = logging.Flush`. `go-apidiff` classifies a func→var change
as breaking, which would violate SC-003 and the project's
additive-over-breaking preference.

**Alternatives considered**: Root-only `Flush`, with `logging` exporting none.
Simpler and ships no provably-inert API, but leaves the surfaces asymmetric.
Parity was chosen deliberately.

## R8: Interaction with ADR-0004 (Trace ID Format)

**Decision**: ADR-0004 does not govern this feature. It is neither absorbed nor
retired. Two of its consequences are carried forward as constraints.

**Rationale**: ADR-0004 decides the *format* of trace and span IDs — 32- and
16-character lowercase hex from OTel's native `IdGenerator`. This feature makes
no competing decision. Retiring it would demand absorbing consequences far
outside this scope (`TRACEPARENT` ingestion, error-envelope metadata).

**Constraints carried forward**:

1. ADR-0004 states "ax-go takes a direct dependency on
   `go.opentelemetry.io/otel/sdk`." After this feature the `logging` surface
   depends on the trace **API** only. This is compatible — the SDK generates
   IDs, while the logger only *reads* an existing span context — but it is
   recorded so a future reader does not mistake it for drift. Root `ax` retains
   the SDK dependency unchanged.
2. ADR-0004 states that "when no span is active, fields carry zero-value valid
   hex strings so consumer parsers do not branch on absence." The relocated
   `tracingHook` MUST preserve this. It does so today via the `ZeroTraceID` /
   `ZeroSpanID` constants sourced from `contract`, which is also what keeps the
   no-active-span path allocation-free. This is a binding invariant on the
   move, and the reason `contract` is in `logcore`'s allowed dependency set.

**Incidental finding**: `logger.go:213` cites "ADR-0009" in a doc comment, but
no such file exists in `docs/adr/`. A stale reference from an earlier
retirement. Out of scope here; flagged for separate cleanup.

## R9: Obligations inherited from the sibling feature

**Decision**: Treat `016-optional-grpc-otlp`'s build-constraint matrix and
expanded surface gate as the baseline.

**Rationale**: This branch is stacked on it, so its rules bind. Adding
`logging` as the **seventh** public package means:

- **Two allowlists must change together**: `PublicPackages` in
  `internal/cmd/surfacecheck/inventory.go` and `allowedPackages()` in
  `internal/cmd/apidiff-verdict/main.go`. They are duplicated by design and
  guarded by a test that parses one and compares. Updating only one fails CI.
- **The surface matrix stays at 24 loads.** A *load* is one (configuration,
  profile) combination, and `scanCombination` loads **every** requested public
  package within that single combination (`internal/cmd/surfacecheck/inventory.go`
  — `go list -deps -export -json` over all requested paths at once). 4 × 6 = 24
  regardless of how many packages are requested. What changes is the *content* of
  each load, not their count: the doc comment at
  `internal/cmd/surfacecheck/main.go:14` and the matching AGENTS.md sentence say
  "24 loads of the **six** public packages" and become "**seven**". An earlier
  draft of this document claimed the count grew to 28; that was wrong and is
  corrected here so the false number is not copied into a code doc comment and
  into governance docs.
- **All four build configurations must be exercised** — default, `ax_no_grpc`,
  `ax_no_otlp`, and both. Per AGENTS.md, a green default run covers none of the
  others.
- **Parity assertions belong in untagged files** so they run under every
  configuration.
- **The isolation guarantee is configuration-independent** and must be asserted
  as such: `logging` never links the trees those constraints decline, so its
  isolation holds identically in all four — a stronger and simpler claim than
  the root facade can make (FR-014).

**Inherited risk, not caused here**: PR #150 currently fails Lint —
`actionlint`/shellcheck SC2086 on an unquoted variable in
`.github/workflows/crosscompile.yml` (lines 73, 81, 87). Noted so it is not
misdiagnosed as a defect of this feature. The naive fix is wrong: quoting the
variable passes an empty argument when it is unset.

## R10: How should the size guarantee be enforced?

**Decision**: A new `internal/cmd/sizecheck`, following the established
`covercheck` / `benchcheck` / `surfacecheck` pattern — hardcoded Go constants
for per-surface byte ceilings, so every ceiling change is a reviewable commit
auditable via `git blame`.

**Rationale**: The repository already gates coverage, benchmarks, and public
surface this way, and the import-isolation test alone defends the *cause* but
not gradual creep from permitted dependencies (zerolog itself growing, for
instance). SC-001 states a number; a gate makes it true rather than aspirational.

**The gate enforces two things, not one.** An absolute ceiling alone leaves
SC-002 (the ≥75% reduction) unenforced, and the ceiling is the half that fails
*loudly* — a toolchain bump pushes the binary over the line and someone raises
the constant deliberately. The **ratio** is the half that rots *silently*: nobody
re-measures the root-facade baseline, so the headline claim drifts from reality
with no signal. `quickstart.md` already calls the ratio "the durable claim", so
the gate measures both:

1. `examples/logging` (isolated) stripped size ≤ its hardcoded ceiling — SC-001.
2. `1 − isolated/rootfacade` ≥ the hardcoded minimum reduction — SC-002.

The denominator is a committed program, `examples/rootlogging`: the same source
as `examples/logging` with `logging.NewLogger` swapped for `ax.NewLogger`. It is
committed rather than synthesised into a temp module because a synthesised module
needs its own resolvable `require` graph, whereas an in-module program is built
with the repository's own `go.mod` — no network, no `replace` stanza, and the
comparison stays reviewable in `git diff`. It also documents the two surfaces
side by side.

**Implementation constraints**:

- Absolute sizes vary by Go toolchain version, so the ceiling needs headroom and
  a documented adjustment procedure. The **ratio** does not: it is stable across
  toolchain versions precisely because both sides move together, which is why it
  is the durable claim.
- The two new command packages and the two new example packages take explicit
  `covercheck` floors calibrated to measured coverage, per the established
  convention. An earlier draft of this document said `sizecheck` would simply
  face the 25% default with no override; that is superseded.
- The check builds each probe with `-trimpath -ldflags="-s -w"`, so it must
  handle a build failure distinctly from a ceiling breach, and distinctly again
  from a ratio breach — three outcomes, three messages.

**Alternatives considered**: Import-isolation test only — cheaper and free of
toolchain-version flakiness, since a size regression *requires* a forbidden
import. Rejected because it cannot see creep from permitted dependencies.

## R11: Naming

**Decision**: `logging`, not `log`.

**Rationale**: A package named `log` would shadow the standard library's `log`
at consumer call sites, forcing import aliases in exactly the files most likely
to import both. `logging` is unambiguous and reads naturally as
`logging.NewLogger(ctx)`.

**Alternatives considered**: `axlog` (stutters as `axlog.NewLogger` next to the
`ax` import path); `logger` (names the type, not the domain, and collides with
common local variable names).

## R12: How is the new surface's gated example genuinely gated?

**Decision**: Make `doccover`'s required-symbol set **package-qualified**, and
qualify `baseline.txt` entries with it. Scanning a second directory is necessary
but nowhere near sufficient.

**Rationale**: `internal/cmd/doccover/main.go` gates a hand-curated
`requiredSymbols()` list of **bare** names — `"NewLogger"`, `"Logger"`,
`"Flush"` — against `scanPackage(root)`, which returns flat sets of exported
symbol names and verified `ExampleXxx` names from one directory.

Root already has `ExampleNewLogger`. So the obvious implementation — scan
`logging` as well and union the results — would let root's existing example
satisfy the requirement for `NewLogger`, and `logging`'s example would be
**required by the contract but not actually enforced by the gate**. The contract
would be documentation, not verification, which is exactly the failure mode
Principle VII exists to prevent. The gate would still be *green*, which is worse
than a red one.

The fix is small but touches the artifact format: required symbols become
`ax.NewLogger` / `logging.NewLogger`, the scan is per-package, and `baseline.txt`
lines carry the same qualification. Existing baseline lines migrate by prefixing
`ax.`.

**Alternatives considered**: keeping bare names and requiring the example to be
named distinctly (e.g. `ExampleNewLogger_logging`). Rejected — the example's name
is the godoc contract, `ExampleNewLogger` is the correct name in its own package,
and bending the public artifact to fit the gate's internal data model is
backwards.

## R13: Benchmark naming across two packages

**Decision**: `logcore`'s benchmarks take **distinct names** (`BenchmarkLogcore*`).
Root's `BenchmarkLogger*` benchmarks are neither renamed nor moved.

**Rationale**: `benchcheck` has no hardcoded tracked-benchmark list; it compares
whatever rows appear on both sides, and its missing-benchmark detection keys on
the benchmark **name alone**, ignoring the package group
(`internal/cmd/benchcheck/main.go`, `missingBenchmarks`). Duplicating
`BenchmarkLoggerEmit` into a second package therefore puts two same-named rows in
the current run against one in the baseline — an ambiguity the gate was never
designed to resolve, and one whose failure mode (a merged or dropped row) hides a
regression rather than reporting it.

Distinct names cost nothing here. SC-006's hot-path coverage is unaffected: root's
`BenchmarkLogger*` benchmarks stay exactly where they are and, after the
extraction, exercise the relocated code **through** the delegation, so the tracked
path is still measured. `logcore`'s benchmarks are additional direct coverage of
the same code, not a replacement.

**Alternatives considered**: identical names, relying on benchstat's `pkg:` group
to disambiguate. Rejected — it makes a correctness property of the performance
gate depend on an internal detail of a deprecated upstream library, for the sole
benefit of reusing a name.

## Decision Records Absorbed

None. No ADR governs this feature. ADR-0004 and ADR-0008 remain in
`docs/adr/`; the reasoning is in R8 and in the plan's ADR governance finding.
The ADR-0006 decisions relevant here were absorbed previously into
`specs/007-loki-direct-push/research.md` and are honoured by R6.
