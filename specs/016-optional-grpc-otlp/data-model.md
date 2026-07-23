# Data Model: Independently Opt-Out OTLP Export and gRPC Dial Adapter

**Feature**: `016-optional-grpc-otlp` | **Date**: 2026-07-22

This feature introduces no runtime persistence and no new machine payload. The
"data model" is (a) the build-configuration matrix that becomes a first-class
concept, and (b) the surface-baseline artifact the new gate reads and writes.

---

## 1. Build Configuration (conceptual entity — no Go type)

The cross product of two independent, default-off build constraints.

| Configuration | Tags passed | `ax.GRPCDial` | OTLP export | Forbidden trees linked |
| --- | --- | :---: | :---: | :---: |
| `default` | *(none)* | present | active | 68 grpc / 36 pb / 4 otlp / 3 gw |
| `no-grpc` | `ax_no_grpc` | **absent** | active | 66 / 36 / 4 / 3 |
| `no-otlp` | `ax_no_otlp` | present | **declined** | 66 / 33 / 0 / 0 |
| `minimal` | `ax_no_grpc,ax_no_otlp` | **absent** | **declined** | **0 / 0 / 0 / 0** |

**Invariants**

- Both constraints are *negative*: absence of the tag is the full-featured state.
- The four configurations are exhaustive; no third tag is introduced.
- Machine payloads (`__schema`, `ax.Error`) are byte-identical across all four.
- Trace-context extraction, root-span creation, and log trace/span correlation
  are behaviourally identical across all four.
- `ax.GRPCDial` is the **only** public identifier whose presence varies.

**Encoded as** hardcoded Go constants in `internal/cmd/surfacecheck`, so a change
to the matrix is a reviewable commit auditable via `git blame` — matching the
`covercheck` / `benchcheck` policy-as-constants convention.

---

## 2. Platform Profile (conceptual entity — no Go type)

The six `GOOS/GOARCH` pairs already covered by `crosscompile.yml:35-36`:

```
linux/amd64   linux/arm64
darwin/amd64  darwin/arm64
windows/amd64 windows/arm64
```

**Invariant**: `CGO_ENABLED=0` for every profile. The module is pure Go, so all
six type-check from a single linux/amd64 runner (verified — research.md D6).

**Combination count**: 4 configurations × 6 profiles = **24 surface loads**.

---

## 3. Surface Baseline (committed artifact)

`internal/cmd/surfacecheck/baseline.json` — the committed record of which
exported identifiers exist in which configuration and profile.

### Shape

```jsonc
{
  "version": 1,
  "packages": {
    "github.com/rshade/ax-go": {
      "symbols": {
        "func:GRPCDial(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)": {
          "configurations": ["default", "no-otlp"],
          "profiles": "all"
        },
        "func:HTTPClient() *http.Client": {
          "configurations": "all",
          "profiles": "all"
        }
      }
    }
  }
}
```

### Field contract

| Field | Meaning |
| --- | --- |
| `version` | Baseline schema version; bumped only on an incompatible format change |
| `packages` | Keyed by exact import path; the six-package allowlist, no prefixes |
| `symbols` | Keyed by a rendered, canonical symbol string (see below) |
| `configurations` | `"all"`, or an explicit sorted list of configuration names |
| `profiles` | `"all"`, or an explicit sorted list of `goos/goarch` strings |

### Symbol key derivation

Rendered via `types.ObjectString(obj, types.RelativeTo(pkg))` prefixed by kind
(`func:`, `type:`, `const:`, `var:`, `method:`, `field:`). Both
`types.Scope.Names()` and `types.ObjectString` are documented-stable and sorted,
which is what makes the baseline byte-deterministic under Constitution
Principle II.

The walk covers package-scope objects **plus** method sets
(`types.NewMethodSet(types.NewPointer(named))`) and struct fields — package-scope
alone would miss real API-shape drift.

### Invariants

- **Written sorted** at every level (packages, symbols, list values) — a
  regenerated baseline on an unchanged tree must be byte-identical.
- `"all"` is the canonical encoding when a symbol is universal; an exhaustive
  list that happens to cover everything is normalised to `"all"` on write.
- Today exactly **one** symbol is expected to carry a non-`"all"`
  `configurations` value: `ax.GRPCDial`. Everything else is universal (verified:
  173 exported objects, identical across all profiles tested).
- Architecture-dependent constants, if any appear, are recorded per-profile and
  **not** normalised away — they are legitimate drift signal.

### State transitions

```
                  regenerate (-update)
   baseline.json ──────────────────────> baseline.json'
         │                                    │
         │ compare (default mode)             │ reviewed in PR diff
         ▼                                    ▼
   PASS (exit 0)  /  FAIL (exit 1)        committed
                          │
                          └── names each drifted symbol + its configuration
```

The gate is **not** a one-way ratchet (unlike `doccover`'s `baseline.txt`):
symbols legitimately come and go, so both additions and removals surface as drift
and are resolved by a reviewed baseline regeneration.

---

## 4. Forbidden Import Rule Set (Go value)

Consumed by `internal/testutil`. A distinct `[]ForbiddenImport`, **not** a reuse
of `ForbiddenRuntimeImports()` — that one forbids the whole
`go.opentelemetry.io/otel/exporters/` prefix, which would wrongly catch
`stdouttrace`, legitimately present in the declined build (research.md D9).

| Pattern | Reason |
| --- | --- |
| `google.golang.org/grpc` | gRPC runtime |
| `google.golang.org/protobuf` | protobuf runtime and reflection tables |
| `go.opentelemetry.io/proto/otlp` | OTLP wire definitions |
| `github.com/grpc-ecosystem/grpc-gateway/v2` | generated gateway stubs |

`matchesForbiddenImport` already prefix-matches, so each pattern covers its whole
subtree (`grpc/status`, `grpc/codes`, …).

**Invariant**: asserted against the `minimal` configuration only. The
intermediate configurations legitimately retain 66 gRPC packages and are not
subject to this rule.

---

## 5. Telemetry Exporter Seam (Go function)

The single identifier that must exist in both variants:

```go
func newOTLPExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error)
```

| Variant | File | Behaviour |
| --- | --- | --- |
| `!ax_no_otlp` | `internal/telemetry/otlp.go` | Constructs the real HTTP exporter (today's `telemetry.go:139-162` body, unchanged) |
| `ax_no_otlp` | `internal/telemetry/otlp_disabled.go` | Returns `nil, <sentinel error>` |

**Contract preserved in both**: `Start` never returns an error (documented at
`telemetry.go:55-58`), always returns a usable recording `*sdktrace.TracerProvider`,
and routes any exporter failure through `writeDiagnostic(cfg.Stderr, "otel
exporter disabled", err)`. The disabled variant reuses that **exact existing
string**, which is what keeps `execute_test.go:126` green unchanged.

**Not duplicated** (stay unconditional): `normalizeOTLPEndpoint`,
`diagnosticExporter`, `writeDiagnostic`, `SanitizeDiagnostic`, `lockedWriter`,
`Config`, `Start`, `telemetryResource`, `DefaultShutdownBudget`, and the entire
`stdouttrace` branch.
