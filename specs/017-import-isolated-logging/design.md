# Design: import-isolated `logging` package

Brainstorming artifact for issue #144. This is the validated design that the
formal Spec Kit `spec.md` will be written from. The #143 sequencing gate is
discharged — see
[Sequencing](#8-sequencing-resolved-by-re-measurement).

- **Issue:** #144
- **Branch:** `017-import-isolated-logging`, stacked on
  `016-optional-grpc-otlp` (PR #150, open) so #143's build-tag work is
  present without waiting for the merge
- **Related:** #78 (isolation pattern), #7 (Loki addon), #18 (internal audit),
  #119 (`WithFlushFunc`)
- **Governing:** Constitution Principles VI, VIII, X, XI

## 1. Problem

`logger.go` imports only `context`, `errors`, `io`, `os`, and `zerolog`.
Nothing about the logger needs a heavy dependency tree. But Go links at
**package** granularity, so a consumer importing `ax` for `NewLogger` links
everything package `ax` imports: `internal/telemetry` → `otlptracehttp` →
the OTLP protobuf and grpc-gateway trees → `google.golang.org/grpc`, plus
`internal/mcpserver` and Cobra.

The logger is guilty purely by package association.

## 2. Verified measurements

Re-measured 2026-07-22 on linux/amd64, `-trimpath -ldflags="-s -w"`,
reproducing the issue's figures before accepting them.

| Consumer | Bytes | Issue claimed |
|---|---|---|
| Root `ax` + `NewLogger` + one `Info()` | 12,017,929 | 12,017,929 (exact) |
| Simulated isolated logging package | 2,248,969 | 2,257,161 (within 8 KB) |
| **Saving** | **9,768,960 (−81.3%)** | −81.2% |

Component costs, measured standalone:

| Component | Bytes | Pulls `google.golang.org/grpc` |
|---|---|---|
| OTel trace **API** | 1,794,210 | 0 packages |
| OTel **SDK** | 4,518,153 | 0 packages |
| `otlptracehttp` exporter | 13,558,025 | **66 packages** |
| `google.golang.org/grpc` | 10,400,009 | — |

**Key finding:** gRPC enters the binary through the *HTTP* OTLP exporter, not
through `ax.GRPCDial`. The SDK alone pulls zero gRPC packages. This confirms
the issue's claim that deleting `GRPCDial` would barely move the total, and it
is why #143 (exporter opt-out) is the larger lever.

The split places the OTel **trace API** (needed by `tracingHook` for
`SpanContextFromContext`) on the isolated side and the **SDK** plus exporters
on the root-facade side. Measured, that is the correct side of the cost.

## 3. Package layout

```text
internal/logcore/          NEW — implementation
  exported:   Labels, Logger, Config, Option, Sink, LabelSanctioner,
              New, Flush, WithWriter, WithLevel, WithLabels
  unexported: zerologLogger, flusher, tracingHook, applyLabels
  deps:       stdlib, zerolog, go.opentelemetry.io/otel/trace, contract

logging/                   NEW — public, thin re-export
  Logger, Labels, LoggerOption      (type aliases → logcore)
  NewLogger, WithLoggerWriter, WithLoggerLevel,
  WithLoggerLabels, Flush           (thin wrappers)

ax (root)                  delegates; zero public API change
  type Logger = logging.Logger
  type Labels = logging.Labels
  type LoggerOption = logging.LoggerOption
  func NewLogger(...) Logger
  func Flush(ctx, Logger) error
```

Package named `logging`, not `log`, to avoid shadowing stdlib `log` at
consumer call sites.

## 4. Export decisions (and why)

The issue proposed keeping `logSink` and `flusher` **unexported** in
`logcore`, reasoning that root `ax` may import `internal/*`. That does not
compile. Two independent Go mechanisms were conflated:

- `internal/` is a **package path** rule — it controls *who may import*.
- Lowercase is an **export** rule — it controls *what is visible to any
  importer*.

Being permitted to import a package grants no additional visibility into it.
Verified by compilation:

```text
cannot refer to unexported field additionalSinks
```

And after exporting both the field and the interface type:

```text
*lokiWriter does not implement logcore.Sink (unexported method drain)
```

The second failure is the structural one: **unexported method names are
qualified by their defining package**. A `drain` method declared in package
`ax` is a different identifier from `drain` declared in `logcore`, so a type
outside `logcore` can never satisfy such an interface. This is the same
mechanism the stdlib uses to make interfaces deliberately un-implementable
from outside.

### What must be exported, and what need not be

| Symbol | Exported? | Reason |
|---|---|---|
| `Sink` | yes | `lokiWriter` lives in `ax` and must satisfy it |
| `LabelSanctioner` | yes | same |
| `Config` + fields | yes | `WithLokiFromEnv` writes to it from `ax` |
| `flusher` | **no** | `zerologLogger` moves to `logcore`; same-package satisfaction |
| `tracingHook`, `applyLabels`, `zerologLogger` | **no** | never referenced outside `logcore` |

**Guardrail (Principle VI).** With `Sink` exported, "no pluggable backend"
rests on the `internal/` path restriction: no module outside
`github.com/rshade/ax-go` can import `internal/logcore`, so no external caller
can register a backend. The public `logging` package never re-exports `Sink`.
This matches the `internal/mcpserver` precedent. The residual risk — an ax-go
*maintainer* adding a second backend without a compiler complaint — is
accepted and held by review, consistent with how Principle VI is enforced
elsewhere.

### Config fields stay exported fields, not accessor methods

Encapsulating `Config` behind an `AddSink()` method was considered and
rejected. `loki.go` constructs `errorWriter: &cfg.writer` — it captures the
**address** of the writer field so a later `WithLoggerWriter` is still
observed by the Loki error path. That pointer aliasing is load-bearing for
option-order independence and must be preserved byte-for-byte, so the fields
are exported.

## 5. The two interfaces replacing the type assertions

```go
// internal/logcore
type Sink interface {
    io.Writer
    Drain(ctx context.Context) error
}

// Optional, type-asserted. Deliberately NOT folded into Sink so a future
// non-Loki sink is not forced to implement label semantics it has no
// concept of — preserving the generic seam required by ADR-0006 decision D2
// (absorbed into specs/007-loki-direct-push/research.md).
type LabelSanctioner interface {
    SanctionLabels(Labels)
}
```

`logger.go:114` and `logger.go:161` — today `if lw, ok := s.(*lokiWriter)` —
become `if ls, ok := s.(LabelSanctioner); ok { ls.SanctionLabels(...) }`.

This is the entire Loki decoupling. It also corrects a second flaw in the
issue text: the proposed `interface{ sanctionLabels(Labels) }` is
uncompilable for the same unexported-method reason, and must be
`SanctionLabels`.

Note ADR-0006 D2 rejected a separate `internal/loki` package because it would
force CLI authors to wire the sink explicitly. That reasoning is respected:
**Loki does not move.** The base logger moves in the opposite direction, and
root `ax` still wires the sink on the author's behalf via `WithLokiFromEnv`.

## 6. Data flow (behavior unchanged)

```text
ax.NewLogger(ctx, ax.WithLokiFromEnv())
  └─► logging.NewLogger ─► logcore.New(ctx, opts...)
        1. apply all Options to *Config
        2. for each Config.AdditionalSinks:
             assert LabelSanctioner → SanctionLabels(cfg.Labels)
           (after all options, so label sanctioning is order-independent
            regardless of WithLokiFromEnv / WithLoggerLabels ordering)
        3. io.MultiWriter(cfg.Writer, sinks...)
        4. zerolog.New(w).Level(...).Hook(tracingHook{})
```

`WithLabels` performs the same `LabelSanctioner` assertion on the carried
sinks and returns a derived logger, exactly as today.

`ax.WithLokiFromEnv`, `AX_LOKI_URL`, `AX_LOKI_AUTH_TOKEN`, and the five-label
cardinality allowlist are untouched.

## 7. `Flush`

`logging.Flush` is exported for surface parity with root `ax`, and because the
sink seam is generic rather than Loki-specific.

Accepted trade-off, recorded honestly: Loki is the only `Sink` that exists and
is unreachable from `logging`, so for every currently-possible isolated
consumer `logging.Flush` takes the negative branch and returns `nil`. Its doc
comment must say so plainly.

`ax.Flush` **stays a `func`**, delegating to `logging.Flush`. It must not
become `var Flush = logging.Flush`: `go-apidiff` treats a func→var change as
breaking, and the project preference is additive-over-breaking.

## 8. Sequencing: resolved by re-measurement

The gate is discharged. This branch is stacked on `016-optional-grpc-otlp`,
and the §2 measurements were re-run against that branch on 2026-07-22:

| Scenario | Root facade + `NewLogger` | vs. isolated (2,248,969) |
|---|---|---|
| `main` today | 12,017,929 | −9,768,960 (−81.3%) |
| #143 branch, **default** build | 12,017,929 *(identical)* | −9,768,960 (−81.3%) |
| #143 branch, `-tags ax_no_otlp,ax_no_grpc` | 4,595,977 | −2,347,008 (−51.1%) |

**#143 does not erode this feature's benefit for default builds.** Its opt-out
is a *build tag*, both defaulting off, so a consumer who passes no tags sees a
byte-identical 12,017,929 binary. The erosion applies only to consumers who
opt in, where the remaining saving is 2,347,008 bytes (the issue predicted
~3.2 MB; actual is ~2.35 MB).

This reframes the "re-argue the case for another public package" question, and
it argues *for* the feature rather than against:

- **#143 requires the consumer to know about and pass two build tags.** It is
  a deliberate, documented opt-in.
- **#144 delivers the small binary by importing a package** — the action the
  consumer was going to take anyway. No flags, no tags, no knowledge required.

The two are complementary, not redundant: they reduce the same weight through
different mechanisms with different ergonomics, and a consumer can take both
(isolated `logging` needs neither tag, since it never links the trees the tags
decline).

## 9. Known trade-off

`LoggerOption` resolves to `func(*logcore.Config)`, so godoc for the public
`logging` package names an `internal/` type. External callers still cannot
author their own options — exactly as true today, since `loggerConfig` is
unexported — so this is behavior-preserving. It is a cosmetic wart, recorded
here rather than discovered in review.

## 10. Testing

| Test | Purpose |
|---|---|
| `logging/import_isolation_test.go` | forbids root `ax`, `internal/telemetry`, otel SDK, otel exporters, otelhttp, otelgrpc, `google.golang.org/grpc`, and `net/http` |
| Root-vs-isolated parity | identical option sets produce byte-identical log lines through `ax.NewLogger` and `logging.NewLogger` |
| `loki_test.go` | passes unmodified in substance, proving the addon is behaviorally untouched |
| No-Loki-symbol test | `logger.go` and `internal/logcore` reference no Loki identifier |
| `logger_bench_test.go` | the zero-allocation no-active-span path must not regress |
| `internal/cmd/sizecheck` | hardcoded per-surface byte ceilings; `make size-check` |
| `ExampleNewLogger` | on the `logging` surface, for `make doc-coverage` |

### Obligations inherited from #143

Merging onto the build-tag toolchain adds requirements the original issue
predates. All are consequences of `logging` becoming the **seventh** public
package.

- **Two allowlists must be updated together.** `PublicPackages` in
  `internal/cmd/surfacecheck/inventory.go` and `allowedPackages()` in
  `internal/cmd/apidiff-verdict/main.go` are duplicated by design and guarded
  by a test that parses one and compares. Adding `logging` to only one fails
  CI.
- **The surface matrix grows from 24 loads to 28** (7 packages × 4
  configurations × 6 GOOS/GOARCH profiles).
- **`logging` must be verified under all four configurations** — default,
  `ax_no_grpc`, `ax_no_otlp`, and both. Per AGENTS.md, a green default run
  covers none of the others, and `make test` / `make validate` / `make lint`
  iterate `BUILD_TAG_MATRIX`.
- **Parity assertions go in untagged files** so they run under every
  configuration. The root-vs-isolated byte-identical log-line test is a parity
  assertion and must be untagged.
- **The isolation guarantee is configuration-independent.** `logging` never
  links the trees `ax_no_otlp` / `ax_no_grpc` decline, so its import-isolation
  test must hold identically in all four configurations — a stronger and
  simpler claim than the root facade can make. Assert it, do not assume it.

`internal/testutil/imports.go` currently forbids `github.com/rs/zerolog` for
all isolated surfaces. `logging` needs its own rule set permitting `zerolog`
and the OTel trace API while keeping the SDK, exporters, gRPC, and the root
facade forbidden. The stale `github.com/rshade/ax-go/internal/loki` pattern in
that file — for a package that has never existed — is resolved to the real
boundary in the same change.

### Size gate

`internal/cmd/sizecheck` follows the established `covercheck` / `benchcheck` /
`surfacecheck` pattern: hardcoded Go constants for per-surface byte ceilings,
so every ceiling change is a reviewable commit auditable via `git blame`. It
builds a minimal consumer per isolated surface with
`-trimpath -ldflags="-s -w"` and fails on breach.

Two caveats to handle in implementation:

- Absolute byte sizes vary by Go toolchain version, so ceilings need headroom
  and a documented bump procedure.
- The new package faces the 25% default per-package coverage floor.

## 11. Out of scope

- Removing, reverting, relocating, or behaviorally changing the Loki
  direct-push addon from #7. It stays in root `ax` as shipped.
- Build tags or any conditional compilation. The package boundary is the
  isolation mechanism, consistent with #78 and `mcp`.
- A pluggable or second logger backend, a public sink-registration API, or
  `ax.WithLogger(...)`. `Logger` remains a migration seam only.
- Deprecating or removing any existing root-package symbol.
- Isolating `Execute`, telemetry lifecycle, or the transport helpers.

## 12. Acceptance criteria

- [ ] A consumer obtains a trace-correlated `Logger` importing only `logging`,
      with no root facade in its `go list -deps` graph
- [ ] `logging`'s dependency graph excludes root `ax`, `internal/telemetry`,
      the OTel SDK and exporters, otelhttp, otelgrpc,
      `google.golang.org/grpc`, and `net/http`
- [ ] A logging-only consumer measures under 3 MB on linux/amd64 with
      `-trimpath -ldflags="-s -w"`
- [ ] Every existing root logging symbol remains available with unchanged
      documented behavior; `go-apidiff` reports no breaking change
- [ ] `ax.WithLokiFromEnv`, `ax.Flush`, `AX_LOKI_URL`, `AX_LOKI_AUTH_TOKEN`,
      and the five-label allowlist behave identically
- [ ] `logger.go` contains no reference to `lokiWriter` or any Loki symbol
- [ ] No public API permits selecting or registering a logging backend
- [ ] `examples/integration` still exercises the root path including
      `WithLokiFromEnv` and `Flush`; a second example exercises `logging`
      in isolation
- [ ] `internal/cmd/surfacecheck` baseline reviewed and updated
- [ ] `go test -race ./...`, `golangci-lint run`, `make doc-coverage`,
      `make cover-check`, `make surface-check`, `make size-check` all clean
