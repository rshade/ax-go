# Contract: Bounded Config Read API

**Feature**: `001-bound-config-reads` | **Date**: 2026-06-02

The public contract this feature exposes from package `ax`. This is the contract
agents and human consumers ground themselves on; the **frozen** parts are marked
and guarded by tests/goldens. Renaming a frozen identifier or `error_code` is a
breaking change (Constitution Principle VI).

## Exported surface (package `ax`, module root)

```go
// Default maximum config size: 1 MiB. Frozen identifier; value is contract.
const DefaultMaxConfigBytes int64 = 1 << 20

// MaxConfigBytesCeiling is the largest valid cap: 1 GiB. A cap above it is
// rejected as config_max_bytes_invalid (exit 2). Frozen identifier; value is
// contract. There is no effectively-unbounded read path.
const MaxConfigBytesCeiling int64 = 1 << 30

// ParseConfigOption configures ParseConfig and ParseConfigFile.
type ParseConfigOption func(*parseConfigOptions)

// WithMaxConfigBytes sets the maximum config bytes for one parse invocation.
//
// The value is not global and does not affect later calls. Zero is a valid,
// honored limit: empty input passes the size check and then follows normal parse
// semantics, while any non-empty input is rejected as config_too_large. Values
// below zero or above MaxConfigBytesCeiling return config_max_bytes_invalid,
// mapped to exit code 2; there is no unbounded read path. Passing a nil
// ParseConfigOption is rejected as config_option_invalid, also mapped to exit
// code 2.
func WithMaxConfigBytes(maxBytes int64) ParseConfigOption

// ParseConfig parses Hujson from r under a bounded read cap and unmarshals into dst.
//
// Reads default to DefaultMaxConfigBytes and consume at most cap+1 bytes.
// Oversize input returns an errors.As-discoverable *Error with error_code
// config_too_large and exit code 2. A cap below zero or above
// MaxConfigBytesCeiling returns config_max_bytes_invalid and exit code 2. A nil
// ParseConfigOption returns config_option_invalid and exit code 2. Hujson parse
// and schema/type decode failures return config_invalid and exit code 2, with
// the underlying decode error preserved in the chain (reachable via errors.Is
// and errors.As through Unwrap). Invalid decode destinations, such as nil or
// non-pointer dst values, surface the underlying *json.InvalidUnmarshalError as
// caller misuse and are not classified as config_invalid. Every valid cap is at
// most MaxConfigBytesCeiling, so there is no unbounded read path. ctx
// cancellation is honored between chunk reads, not inside a single blocking
// Read. Wrapped context.DeadlineExceeded maps to exit code 3 via ErrorExitCode,
// and wrapped context.Canceled maps to exit code 1. A non-EOF source error
// before cap+1 bytes is returned with its chain preserved and is not classified
// as oversize; if the same read crosses the cap and returns a source error, the
// oversize validation error wins.
func ParseConfig(ctx context.Context, r io.Reader, dst any, opts ...ParseConfigOption) error

// ParseConfigFile opens path and applies ParseConfig's contract to its contents.
//
// The file is closed before return. Open failures are returned as-is; read,
// cap, context-cancellation, and Hujson decode behavior match ParseConfig.
func ParseConfigFile(ctx context.Context, path string, dst any, opts ...ParseConfigOption) error
```

**Signature change from bootstrap**: both entry points gain a leading
`ctx context.Context` (Principle X). Pre-release, no compatibility shim.

**Envelope additions (review remediation, research D10)**: `(*Error).Unwrap()
error` exposes the envelope's cause for `errors.Is` / `errors.As` traversal, and
`WithErrorCause(err error) ErrorOption` attaches it. The cause is never
serialized into the JSON envelope. `ParseConfig` uses this to preserve the
schema/type decode error behind `config_invalid`. Frozen identifiers.

## Behavioral contract (maps to FR / SC)

| ID | Guarantee | Verified by |
|----|-----------|-------------|
| FR-001 | Both entry points enforce a max read size, default 1 MiB. | entry-point tests; default constant |
| FR-002 | Cap is per-invocation via `WithMaxConfigBytes`, no entry-point replacement. | option tests; SC-004 |
| FR-003 | Oversize → validation error, exit `2`, never OOM, never unclassified. | oversize test + tripwire (SC-001) |
| FR-004 | Size **exactly** at cap is accepted (inclusive boundary). | boundary table (SC-003) |
| FR-005 | Out-of-range cap (negative or > `MaxConfigBytesCeiling`) → validation error, exit `2`. | negative-cap + above-ceiling tests |
| FR-006 | Bound enforced at the read boundary; memory ≈ cap, source-size-independent. | tripwire reader + `-benchmem` benchmark (SC-001) |
| FR-007 | Failures are `ax.Error` (errors.As), with actionable fix; `error_code`s frozen + golden-locked; `max_bytes` in context is informational. | golden fixtures + field assertions |
| FR-008 | Identical input + cap → identical classification + `error_code`. | repeated-parse determinism test |
| FR-009 | Mid-read source error surfaced (chain preserved), distinct from oversize; precedence: a source error before `cap+1` bytes wins, oversize wins once `cap+1` bytes are read. | failing-reader test (`errors.Is`) + boundary test (a read returning both `n>0` and a non-EOF error) |
| FR-010 | Both entry points take `ctx` first and honor cancelation; a canceled/expired `ctx` aborts the read and surfaces `context.Canceled` / `context.DeadlineExceeded` (chain preserved), distinct from oversize. Cancelation is observed *between* chunk reads (cooperative, multi-chunk sources); a reader blocking inside a single `Read` is out of scope. | cancelation tests (`errors.Is`): **already-canceled → `Canceled`** at both public entry points (`ParseConfig`/`ParseConfigFile`) and internally; **deadline-mid-read → `DeadlineExceeded`** at the internal `ReadBounded` level (the shared loop both entry points delegate to) and at the public level via `ParseConfig` over a slow multi-chunk reader. `ParseConfigFile`'s mid-read deadline rides the same `ReadBounded` loop; a deterministic cross-platform slow *file-path* source is impractical, so its mid-read path is verified internally and its public surface via the already-canceled case. |
| FR-011 | At the CLI boundary, `ErrorExitCode` maps `context.DeadlineExceeded`→`3` and `context.Canceled`→`1` via `errors.Is` (chain preserved; the context error is NOT wrapped in `ax.Error`). The emitted envelope's `error_code` is `internal_error` — classify by exit code, not `error_code`. | exit-code mapping test (SC-006) |

## Frozen error-code contract (FR-007 / SC-005)

These `error_code` strings are **frozen public contract**, golden-locked in
`testdata/`. Agents branch on them without parsing human-facing text.

| `error_code` | Trigger | `ExitCode()` | `actionable_fix` (text not frozen) | `context.max_bytes` (informational) |
|--------------|---------|--------------|-------------------------------------|--------------------------------------|
| `config_too_large` | input exceeds the active cap | `2` | "reduce the config size or raise the limit with WithMaxConfigBytes" | active cap |
| `config_max_bytes_invalid` | cap `< 0` or `> MaxConfigBytesCeiling` | `2` | "set a config byte limit between 0 and MaxConfigBytesCeiling" | the invalid cap |
| `config_invalid` | Hujson parse / standardize / schema-type JSON decode failure | `2` | "fix the config syntax or field types and retry" | (none; the underlying error is the envelope's cause via `Unwrap`) |
| `config_option_invalid` | nil `ParseConfigOption` | `2` | "remove nil ParseConfigOption values before parsing config" | (none) |

**Golden fixtures**: `testdata/config_too_large.golden.json`,
`testdata/config_max_bytes_invalid.golden.json`,
`testdata/config_invalid.golden.json`,
`testdata/config_option_invalid.golden.json` — the raw library `*ax.Error`.
Generated with no active span (zero `trace_id`) for determinism; `tool` and
`version` are empty (`""`) because `ParseConfig` is a library function and does
not populate them — they are injected at the CLI emission boundary, not here. A
change to any fixture is a reviewed, deliberate act; a change to an
`error_code` value itself is a breaking change.

## Out of scope (delegated / deferred)

- **Write path** (Hujson AST `Patch` / strict-JSON emit) — future ROADMAP #9;
  decision preserved in `research.md` (absorbed ADR-0010).
- **Fuzz harness** for the parser surface — tracked follow-up (Principle VII
  deferral recorded in `plan.md` Complexity Tracking).
- **Non-size, non-config-validation failures** (missing file, permission error
  on open, mid-read source errors, and invalid `dst` values that trigger
  `*json.InvalidUnmarshalError`) — surfaced as their underlying errors with the
  chain preserved; not governed by this contract. Syntactically invalid Hujson is
  IN scope: it classifies as `config_invalid` with the parser's error attached as
  the envelope's cause (research D10).
