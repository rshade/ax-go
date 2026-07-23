# Phase 0 Research: Independently Opt-Out OTLP Export and gRPC Dial Adapter

**Feature**: `016-optional-grpc-otlp` | **Date**: 2026-07-22 | **Spec**: [spec.md](./spec.md)

## Decision Records Absorbed

**None.** This feature is not governed by any ADR in `docs/adr/`. The frozen ADR
log contains no record covering build-tag isolation, exporter optionality, or
public-surface inventory. No ADR-retirement task is required in `tasks.md`, and
the plan's ADR-absorption gate is satisfied vacuously.

Prior decisions this feature builds on live in earlier features' `research.md`,
not in ADRs:

- `specs/004-real-otel-export/research.md:239` — rejection of `otlptracegrpc` in
  favour of `otlptracehttp`. **This feature does not revisit that decision.**
  Research below shows the rejection was correct for its stated reasons and
  incidentally irrelevant to binary size: the HTTP exporter links gRPC anyway.
- `specs/010-import-isolated-contracts/` — the `ForbiddenImport` mechanism this
  feature extends rather than replaces.

---

## D0: Baseline Measurement (FR-028, SC-001, SC-002)

Reproduced independently on 2026-07-22 at `741a8d4`, rather than taken from
issue #143. Method: `git archive HEAD` into a scratch tree, test files stripped,
the two declines applied by hand, built against a fixture consumer calling
`ax.Execute` + `ax.NewLogger` + `ax.NewSchemaCommand`.

### Results — linux/amd64, `CGO_ENABLED=0`, `-ldflags="-s -w"`

| Variant | Stripped bytes | Δ vs. baseline | Total pkgs | grpc | protobuf | otlp-proto | gateway |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| baseline (today) | 14,897,314 | — | 410 | 68 | 36 | 4 | 3 |
| `ax_no_grpc` only | 14,893,218 | −4,096 (**−0.03%**) | 406 | 66 | 36 | 4 | 3 |
| `ax_no_otlp` only | 12,644,514 | −2,252,800 (**−15.1%**) | 386 | 66 | 33 | 0 | 0 |
| **both** | **5,460,130** | **−9,437,184 (−63.3%)** | **264** | **0** | **0** | **0** | **0** |

windows/amd64: baseline 15,405,056 → both declines 5,774,336 (−9,630,720, **−62.5%**).

### Findings

1. **The superlinear claim is real and reproduced.** Each decline alone leaves
   66 of 68 gRPC packages linked, because `otlptracehttp` and `otelgrpc` are two
   independent roots over one shared subtree. Only removing both collapses it.
   SC-001's 55% floor is comfortably met at 63.3% / 62.5%.
2. **Confirms SC-002 exactly**: all four forbidden trees reach a hard zero, and
   total package count lands on 264 — matching issue #143's figure precisely.
3. **A nuance issue #143 did not record**: `ax_no_otlp` alone already zeroes
   `grpc-gateway` and `otlp-proto` (3 → 0, 4 → 0) while leaving 66 gRPC and 33
   protobuf packages. So the gateway and OTLP-proto trees hang solely off the
   exporter; `otelgrpc` is what retains core gRPC and most of protobuf. This
   sharpens the sequencing argument: neither knob is individually sufficient for
   FR-015, and `ax_no_grpc` alone is worth **0.03%** — a rounding error, not the
   0.05% the issue estimated.
4. **Reproduction command** (recorded for FR-028) is captured in
   [quickstart.md](./quickstart.md) §"Reproducing the size measurement".

**Consequence for planning**: both tags MUST land in the same release. A staged
delivery shipping `ax_no_grpc` first would measure at −0.03% and read as a failed
change. Sequencing within the branch is fine; sequencing across releases is not.

---

## D1: Opt-Out Mechanism — Negative Build Tags, Not a Package Split

**Decision**: Two negative (opt-out) build constraints, `ax_no_otlp` and
`ax_no_grpc`, expressed as `//go:build !ax_no_otlp` / `//go:build !ax_no_grpc`
on the files carrying the gated code.

**Rationale**: FR-005 requires the default build's exported surface to be
byte-identical, with no `breaking-change-approved` label and no forced
`0.MINOR.0`. Under Constitution Principle XI, removing an exported identifier
from package `ax` is breaking. A package split that relocated `GRPCDial` to a
subpackage is precisely such a removal. Negative tags keep the default surface
untouched and make the reduced surface an explicit consumer choice — the whole
change becomes additive.

**Alternatives considered**:

| Alternative | Rejected because |
| --- | --- |
| Package split — move `GRPCDial` to `ax/grpcx` | Fits the repo's existing import-isolation pattern better, and is the cleaner long-term shape. But it *removes* `ax.GRPCDial`, which is breaking under Principle XI, requires the label and a minor bump, and forces every consumer to edit imports. It also solves only half the problem — the exporter root cannot be package-split out of `internal/telemetry` without restructuring `Start`. Rejected on FR-005. |
| Positive (opt-in) tags, e.g. `ax_grpc` | Inverts the default: the plain build would *lack* `GRPCDial`, which is the same breaking removal, applied to everyone rather than to volunteers. Strictly worse. |
| Runtime flag / env var | Cannot remove a package from the link graph. Delivers zero size benefit — the imports remain. |
| A separate `ax-lite` module | Doubles the maintenance surface and the release process for one dependency axis. Disproportionate. |

**Consequences**: This introduces the repository's **first production build
constraints** (verified: zero `//go:build` lines exist anywhere today, including
tests). That is a genuinely new mechanism and drives D6 and D7 below. Tag count
is held at exactly two; no third knob.

**Deferred, not rejected**: the package split remains the better shape for a
future `1.0`. Record it as a candidate for the v1 API review rather than
litigating it again.

---

## D2: Tag Names

**Decision**: `ax_no_otlp` (declines trace export) and `ax_no_grpc` (declines the
outbound dial adapter).

**Rationale**: The `ax_` prefix namespaces them against any consumer's own tags —
build tags share one global namespace across the whole build, so an unprefixed
`no_grpc` would collide. The `no_` infix makes the negative polarity legible at
the call site: `-tags=ax_no_grpc` reads as what it does. Go build tags cannot
contain hyphens, so underscores are forced.

**Alternatives considered**: `axnogrpc` (unreadable); `ax.no.grpc` (dots are not
valid in tag identifiers); `noax_grpc` (ambiguous — reads as declining ax, not grpc).

---

## D3: Code Seams

Verified by reading `internal/telemetry/telemetry.go` (257 lines) and `http.go`
(73 lines) in full.

### `internal/telemetry` — one-function seam

`otlptracehttp` is referenced by **exactly one function**:
`newOTLPExporter(ctx, cfg) (sdktrace.SpanExporter, error)` at
`internal/telemetry/telemetry.go:139-162`, called from exactly one site,
`Start` at `:84`. That is the entire seam.

| File | Constraint | Contents |
| --- | --- | --- |
| `telemetry.go` | *(none)* | Everything else, unchanged |
| `otlp.go` | `//go:build !ax_no_otlp` | `newOTLPExporter` + the `otlptracehttp` import |
| `otlp_disabled.go` | `//go:build ax_no_otlp` | `newOTLPExporter` returning a sentinel error |

**Stays unconditional** (verified to reference zero OTLP packages):
`normalizeOTLPEndpoint` (`:164-180`, uses only `net/url`/`path`),
`diagnosticExporter` (`:182-211`, still needed by the `AX_OTEL_DEBUG` path),
`writeDiagnostic` (`:235-244`), `SanitizeDiagnostic` (`:249-256`),
`lockedWriter`, `Config`, `Start`, `telemetryResource`, `DefaultShutdownBudget`,
and the whole `stdouttrace` branch (`:93-109` — `stdouttrace` pulls zero gRPC).

Keeping `normalizeOTLPEndpoint` unconditional is deliberate: it keeps its
8-case table test and its fuzz target tag-agnostic (D8).

### Root `http.go` — clean separation confirmed

`GRPCDial` (`http.go:63-72`) is the only grpc-touching production code in the
repository. `DefaultHTTPTimeout`, `HTTPClientOption`, `httpClientConfig`,
`WithHTTPTimeout`, `HTTPClient`, and `NewHTTPClient` reference **zero** grpc or
otelgrpc types — verified line by line. `otelhttp` stays unconditional.

Note `context` (`http.go:4`) is imported *solely* for `GRPCDial`'s `ctx`
parameter and must move with it.

| File | Constraint | Contents |
| --- | --- | --- |
| `http.go` | *(none)* | Lines 1-61, imports narrowed to `net/http`, `time`, `otelhttp` |
| `grpc.go` | `//go:build !ax_no_grpc` | `GRPCDial` + `context`, `grpc`, `otelgrpc` imports |
| `grpc_disabled.go` | `//go:build ax_no_grpc` | Doc-only explanation (D5) |

### Public surface is otherwise tag-safe

The root telemetry wrapper (`telemetry.go`: `Telemetry`, `StartTelemetry`,
`Shutdown`, the five `WithTelemetry*` options) exposes **no** grpc, otelgrpc,
otlptracehttp, or otlp-proto type. The only OTel type in the public surface is
`*sdktrace.TracerProvider`, which contributes zero gRPC packages and stays
unconditional per the spec's Out of Scope.

**Therefore `ax.GRPCDial` is the single public identifier that disappears under
any decline.** This is the key input to the surface baseline (D6).

---

## D4: Fail-Open Diagnostic Semantics — **spec precision issue found**

**Finding**: The existing "once" mechanism is a `sync.Once` **struct field** on
`diagnosticExporter` (`internal/telemetry/telemetry.go:182-186`), shared between
`ExportSpans` and `Shutdown`. It is **once per `Start` call**, not once per
process — each `Start` constructs a fresh `diagnosticExporter`.

Separately, the construction-failure diagnostic at `:86`
(`writeDiagnostic(cfg.Stderr, "otel exporter disabled", err)`) is emitted
inline in `Start`, so it too fires once per `Start` call.

**Conflict**: FR-008 and SC-006 say "exactly one diagnostic **per process**",
with SC-006 proposing to verify by "counting diagnostics across repeated
invocations within a single process". Taken literally that requires
process-global state.

**Decision**: Keep per-`Start` semantics; do **not** introduce a package-level
`sync.Once`.

**Rationale**: Constitution Principle X and AGENTS.md both forbid mutable
package-level state outright, and Principle VI names it explicitly. A
process-global once would also be untestable in the repo's parallel `go test
-race` runs and would leak state between test cases. Crucially, the distinction
is invisible in the real lifecycle: `Execute` calls `StartTelemetry` exactly
once per process (`execute.go:121`), so for every actual CLI invocation
once-per-`Start` **is** once-per-process.

**Consequence**: FR-008 and SC-006 should be reworded from "per process" to
"per telemetry start", noting that `Execute` starts telemetry once per process.
This is a precision fix, not a behaviour change — no user-visible difference.
Recorded here rather than silently implemented differently from the spec.

**Diagnostic string**: the disabled path reuses the existing
`"otel exporter disabled"` message at `:86` verbatim. This matters beyond
tidiness — `execute_test.go:126` (`TestExecuteTelemetryFailOpen`) asserts
`strings.Contains(stderr, "ax: otel")`, so reusing the prefix keeps that test
passing under both configurations with no change.

---

## D5: Declined-Identifier Discoverability (FR-022, FR-023, FR-024)

**Decision**: `grpc_disabled.go` carries a **file-level doc comment only** — no
exported symbol, no stub function, no reference to any grpc type.

**Rationale and honest limitation**: The user chose "absent + guiding error" over
bare absence. Research confirms Go gives no hook to customise the compiler's
diagnostic: a consumer calling the declined helper gets exactly
`undefined: ax.GRPCDial`, and nothing in the package can change that string.
What *can* be delivered is discoverability at the point a developer investigates:
a documented file explaining which tag removed the identifier and how to restore
it, plus the same statement in `README.md` and the package doc.

Rejected sub-alternatives:

- **An exported stub constant** (e.g. `GRPCDialUnavailableUnderAxNoGRPC`) —
  would appear in the declined build's exported surface, meaning the declined
  build both *loses* and *gains* an identifier, complicating the baseline for no
  real benefit. `godoclint`'s `require-doc` would also demand a doc comment on
  it, adding noise.
- **A stub `GRPCDial` returning an error** — rejected in the spec: retaining the
  signature retains `grpc.DialOption` and `*grpc.ClientConn`, and therefore the
  entire dependency tree. This is exactly FR-024's constraint.
- **A `//go:build ax_no_grpc` file with a deliberate compile error** — would
  break the declined build outright, which is the opposite of the goal.

**Note for `doccover`**: `internal/cmd/doccover/main.go:99-135` parses every
`.go` file in a directory and **ignores build constraints entirely**, so
`grpc_disabled.go` is parsed regardless of tags. Harmless — it declares no
symbol in `requiredSymbols()`. Verified: `GRPCDial` is absent from
`requiredSymbols()` (`:46-73`) and no `ExampleGRPCDial` exists, so tagging it out
**does not break the doc-coverage gate**.

---

## D6: Surface Inventory Gate (FR-018, FR-019, FR-020)

**Decision**: New `internal/cmd/surfacecheck`, built on
`golang.org/x/tools/go/packages` with `Mode: NeedName | NeedTypes`, sweeping
**4 tag combinations × 6 GOOS/GOARCH profiles = 24 loads**, diffed against a
committed `baseline.json`.

### D6a: Superseded — unified with the root surface gate (PR #148)

**Context**: this feature was specified against a repository with no surface
gate at all (spec.md § Resolved Clarifications records the discrepancy with
issue #143). While it was in flight, PR #148 landed a *different*
`internal/cmd/surfacecheck` on `main`: a root-package-only gate that scans via
compiler export data (`go list -deps -export` → `importer.ForCompiler`), diffs
against `baseline.json` **and** a permanent public-surface audit with a
lifecycle state machine, and reports failures as `ax.Error` envelopes. The
rebase surfaced this as an add/add conflict on all three files.

**Decision**: unify into one gate rather than ship two at one import path, or
drop either capability. The two tools had zero feature overlap and mutually
incompatible baseline schemas, but their mechanisms composed cleanly:

- **Kept from #148**: the export-data scanner (strictly deeper — it sees
  promoted fields, complete interface method sets, alias attribution, and
  reachable hidden concrete types, none of which `NeedName|NeedTypes` exposes),
  the ID/signature split, the audit cross-validation, the `Deprecated:` notice
  check, and the `ax.Error` stream and exit contract.
- **Kept from this feature**: the build-tag configuration axis, the
  six-package scope, `-update` with a reviewable indented baseline, the
  `"all"` presence sentinel, the package-list consistency guard against
  `apidiff-verdict`, and the fail-closed assertion on a bogus profile.

**Rationale for the presence model**: #148's `reconcile` treated *any*
cross-profile variance as `profile-divergent` drift. That invariant cannot
survive the configuration axis, whose entire purpose is that `ax.GRPCDial`
disappears under `ax_no_grpc`. The two were split instead:

- **Signature stays a hard invariant** — one feature has exactly one
  signature; divergence fails closed (`signature-divergent`).
- **Presence becomes a recorded fact** — diffed against the reviewed baseline,
  so an unreviewed change in *where* a feature exists is still
  `presence-changed` drift. This strictly subsumes #148's guarantee: a newly
  platform-divergent symbol still fails, because its baseline says `"all"`.

**Baseline schema version 2** nests features under their package and carries a
`configurations` and `profiles` presence set each. Presence is stored as the
*product* of the two sets; `-update` verifies the factorisation is exact and
fails with `presence-unfactored` rather than recording a non-rectangular
pattern lossily, which would let the gate later accept an unreviewed surface.

**Verified**: 285 features across 6 packages, 160 audit records, 24 loads in
~12s; `func:GRPCDial` is the only feature whose presence is not `"all"`
(`configurations: ["default", "no-otlp"]`), and baseline regeneration is
byte-identical.

### The crux question, verified empirically

**Can you type-check darwin/arm64 code on a linux/amd64 runner? Yes — confirmed.**
`packages.Config{Mode: NeedName|NeedTypes, BuildFlags: []string{"-tags=..."},
Env: append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")}` loads
all six public packages with zero errors across linux/amd64, darwin/arm64,
windows/amd64, and even js/wasm and plan9/386. The module is pure Go
(`crosscompile.yml:5-8` states this), so no native toolchain is needed.

Measured cost: ~7.3s for the first combination on a cold `GOCACHE`, then
90–450ms per subsequent combination. With `setup-go`'s `cache: true`, the full
24-combination sweep is cheap.

### ⚠️ Fail-closed requirement

**Verified gotcha**: an invalid `GOOS` returns `err == nil` **and**
`len(pkgs) == 0`. `packages.Load` swallows the underlying `go list` failure when
`Config.Logf` is unset. The gate MUST assert that every *requested* import path
came back with `p.Types != nil` and `len(p.Errors) == 0` — never merely that
`err == nil`. Without this the gate passes vacuously on a misconfigured matrix,
which is the worst possible failure mode for a drift gate.

### Alternatives considered

| Alternative | Verdict |
| --- | --- |
| `go/build.Context` + `go/parser` (stdlib only, zero deps) | **Viable fallback.** Verified working cross-GOOS in 337µs, and it is the technique `doccover` already uses. Rejected because it has no type resolution — the root package's type aliases (`type CommandSchema = schema.CommandSchema`), promoted methods, and inferred constant types are invisible — and, decisively, **it cannot detect that a tag combination fails to compile**. That compile-validation is half the gate's value. |
| `go list -json -e -tags=…` | Insufficient alone. Gives file lists and import graphs, never exported identifiers. Useful only as the file-selection half of the stdlib approach. |
| `golang.org/x/exp/apidiff` | Wrong tool — it *compares* two `*types.Package` values, it does not load them, so x/tools is still required. Also `x/exp` (no compatibility promise) and `.golangci.yml:63` enables `exptostd`, which discourages `x/exp`. |
| `go doc -all` | Rejected. Works cross-platform, but the output is human prose interleaved with doc comments — the baseline would churn on every doc-comment edit. No stable machine contract. |

### Dependency cost (Constitution Principle X)

Measured by running `go mod tidy` on a throwaway copy with a stub importing
`x/tools/go/packages`:

```
go.mod: +golang.org/x/tools v0.47.0            (direct)
        +golang.org/x/mod   v0.37.0 // indirect
        +golang.org/x/sync  v0.22.0 // indirect
go.sum: 111 → 115 lines (+4)
```

**This is a promotion, not a new supply-chain surface.** `x/tools v0.47.0` is
already the selected version in the module graph (pulled by `x/text` and the MCP
SDK) and already carries an `h1:` hash in `go.sum:97-98`. No existing dependency
version changes; both new indirects are first-party `golang.org/x/*`.
`depguard` (`.golangci.yml:203-220`) does not block it.

Justification under Principle X: the stdlib genuinely does not cover cross-GOOS
type-checking, and `x/tools/go/packages` is the canonical and only supported
entry point. `golang.org/x/perf` is existing precedent for a direct `x/*`
dependency serving one internal gate (`benchcheck`).

### Baseline format and determinism

`types.Scope.Names()` is documented sorted, and `types.ObjectString` with a
`types.RelativeTo` qualifier yields stable output — both required for the
byte-deterministic baseline Principle II demands. Package-scope objects alone
miss methods and struct fields, so the walk must also cover
`types.NewMethodSet(types.NewPointer(named))` and `*types.Struct` fields.

Today the surface is **identical across all profiles** (98 exported objects in
root `ax`, 10 in `mcp`, 173 total across the six public packages). Architecture-
dependent constants are a legitimate drift signal, not noise, and must not be
normalised away.

### House-style conventions to match

From `covercheck`/`benchcheck`/`doccover`: `run(...) int` as a pure testable
function; exit codes `0` OK / `1` violation / `2` bad input; violations to
stderr, pass message to stdout; `FAIL  N …` / `PASS  …` verdict shape; policy
(tag combos, profiles) as hardcoded Go constants so changes are `git blame`-able;
input size caps via `io.LimitedReader`.

`allowedPackages()` (`internal/cmd/apidiff-verdict/main.go:67-76`) is the
declared single source of truth for the public package list but is a private
`main`-package function and cannot be imported. Either extract it to a shared
`internal/publicapi` package or duplicate it **with a test asserting the two
lists agree** — silent divergence between the two gates would be a real hazard.

### CI placement

Surfacecheck owns its whole matrix **inside one process** (it sets `GOOS`/`GOARCH`
per load itself). It therefore belongs as a single step in `ci.yml`'s `validate`
job, next to the existing `go run ./internal/cmd/doccover` (`ci.yml:192`).
It must **not** go in `crosscompile.yml`, whose job-level `GOOS`/`GOARCH` env
would fight the tool's own per-combination environment.

---

## D7: The Lint/Vet/Test Blind Spot — a genuine hazard this feature creates

**Finding**: `.golangci.yml` has **no `run:` section at all**, therefore no
`run.build-tags`. `Makefile:124` (`go vet ./...`) and `Makefile:35`
(`go test -race ./...`) carry no `-tags`. `crosscompile.yml:62` vets per
GOOS/GOARCH but passes no tags.

**Consequence**: files behind `//go:build ax_no_grpc` would be invisible to
every one of the ~90 enabled linters, to `go vet`, and to the test suite. This
feature would ship un-linted, un-vetted, un-tested code by construction.

**Decision**: Treat this as in-scope, not incidental.

1. `go test` gains tagged runs for the affected configurations (FR-021 already
   requires behaviour-parity assertions under every configuration, so the tests
   must run under tags regardless).
2. `go vet ./...` gains a tagged pass per combination.
3. `golangci-lint` can only run one tag set per invocation, so full linting
   requires a small matrix of `lint` invocations. Given only two tags, four
   invocations covers it.
4. `surfacecheck`'s `p.Errors` check (D6) provides compile-validation across all
   24 combinations as a backstop.

**Consequence for tasks**: the `Makefile` and `ci.yml` changes are not
one-liners. Budget for them explicitly.

---

## D8: Test Gating Plan

Verified by reading the test files. **No test in `internal/telemetry` constructs
a real OTLP exporter** — its 9 tests all use `stubExporter` or call `Start` with
an empty `Config`. If `normalizeOTLPEndpoint` stays unconditional (D3), **zero
tests in that package need gating.**

The real work is in the root package:

| File | Action | Reason |
| --- | --- | --- |
| `telemetry_export_test.go` | Gate `!ax_no_otlp` | Imports `otlp/collector/trace/v1`, `otlp/trace/v1`, `protobuf/proto` (`:15-17`) — the **only** place the forbidden trees enter the test graph |
| `telemetry_debug_test.go:46` | Gate that one test | Uses `newOTLPTraceReceiver`; other three tests are tag-agnostic |
| `telemetry_security_test.go:20` | Gate `!ax_no_otlp` | Asserts `"ax: otel export failed"`, unreachable under the decline |
| `execute_test.go:179,205` | Gate | Use the receiver / a real endpoint |
| `execute_test.go:126` | **No change** | Asserts `strings.Contains(stderr, "ax: otel")` — survives because D4 reuses the `ax: otel` prefix |
| `http_test.go` | **Split** | 3 `GRPCDial` tests (`:136,152,179`) → `!ax_no_grpc` file; 4 HTTP tests stay common |

⚠️ **Ordering hazard**: the shared helper `executeTelemetryCommand` lives at
`telemetry_export_test.go:217` and is used by `execute_test.go`. It **must be
moved to a common file before** that file is gated, or the declined build's test
compilation breaks. This is a real sequencing constraint for `tasks.md`.

---

## D9: `testutil` Tag Support (FR-016)

**Finding**: `ResolvePackageImports` (`internal/testutil/imports.go:39`) builds a
fixed argv at `:46` with no `-tags` and never sets `cmd.Env`.

**Decision**: Add a variadic tail — `ResolvePackageImports(ctx, moduleDir,
importPath string, tags ...string)` and likewise on `AssertNoForbiddenImports` —
splicing `"-tags", strings.Join(tags, ",")` when non-empty. All four existing
call sites compile unchanged. `internal/testutil` is an `internal/` package and
therefore exempt from the apidiff gate (Principle XI), so the signature change is
free.

**Important**: `ForbiddenRuntimeImports()` is **not** reusable as-is for this
gate. It forbids the whole `go.opentelemetry.io/otel/exporters/` prefix
(`imports.go:15`), which would also catch `stdouttrace` — legitimately present in
the declined build. The new rule set is a separate `[]ForbiddenImport` built from
the same primitives:

```
google.golang.org/grpc
google.golang.org/protobuf
go.opentelemetry.io/proto/otlp
github.com/grpc-ecosystem/grpc-gateway/v2
```

`matchesForbiddenImport` (`:202`) already prefix-matches, so
`google.golang.org/grpc` catches `grpc/status`, `grpc/codes`, etc.

---

## D10: Coverage Floors (SC-012)

Measured at `741a8d4` — every package has headroom, so no floor needs lowering:

| Package | Floor | Measured | Headroom |
| --- | ---: | ---: | ---: |
| `github.com/rshade/ax-go` | 80.0 | 88.2 | +8.2pp |
| `internal/telemetry` | 60.0 | 65.8 | +5.8pp |

The default build's `coverage.out` is **unaffected** by tagging — constrained-out
files are never compiled and never appear in the profile. The only new exposure
is whatever lands in the *common* files, which must itself be tested.

New `internal/cmd/surfacecheck` faces the **25% default floor**
(`covercheck/main.go:141-152` has no entry; `excluded` is empty at `:153`).
SC-012 asks for an explicit floor — one new line in that map, in the same PR,
structured as `run(...) int` with pure helpers so it is cheaply testable.

---

## Open Risks Carried Into `tasks.md`

1. **Two workstreams, one feature.** The opt-out mechanism (D1–D5, D8–D9) and the
   surface gate (D6) are largely independent. Sequence so the size win is not
   blocked on the gate. `effort/large` is an understatement.
2. **D4 spec wording fix** — FR-008/SC-006 "per process" → "per telemetry start".
   Must be applied to `spec.md`, not silently diverged from.
3. **D8 ordering hazard** — move `executeTelemetryCommand` before gating its file.
4. **D6 fail-closed assertion** is easy to omit and silently defeats the gate.
   It needs its own test that a bogus profile fails rather than passes.
5. **Public package list duplication** between `apidiff-verdict` and
   `surfacecheck` needs a consistency test.
6. **D7 lint/vet/test tag coverage** is real, non-trivial work, not a footnote.
