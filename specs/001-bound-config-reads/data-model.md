# Phase 1 Data Model: Bound Config Reads

**Feature**: `001-bound-config-reads` | **Date**: 2026-06-02

This feature has no persisted data (Principle VI — no state). The "entities" are
the in-memory values and the public contract types that flow through a single
parse invocation. They map directly to the spec's Key Entities.

## Entities

### Configuration input

- **What**: The raw bytes of configuration to parse, arriving as an `io.Reader`
  (`ParseConfig`) or a file opened from a path (`ParseConfigFile`). Treated as
  **untrusted and potentially unbounded**.
- **Fields**: opaque byte stream; no schema imposed by ax-go (the consumer's
  `dst` defines the shape).
- **Rules**:
  - Read is capped at the active limit (`maxBytes + 1` bytes max actually read).
  - Hujson is accepted on read (comments, trailing commas); output to `dst` is
    strict JSON semantics via `Standardize` + `encoding/json`.
  - Precedence at the boundary: a source read error observed before
    `maxBytes + 1` bytes are read is surfaced as the source error (FR-009); once
    `maxBytes + 1` bytes have been read, the input is oversize (`TooLargeError`)
    even if the same read also returns a non-EOF error.
  - Lifecycle: read → bound-check → `hujson.Parse` → `Standardize` →
    `json.Unmarshal`. A failure at the bound-check stage short-circuits before
    parsing. Empty input passes the bound-check (it is not oversize) and still
    flows to `hujson.Parse`, which fails as `config_invalid` (exit `2`) with
    the parser's error preserved as the envelope's cause (research D10).

### Read limit (cap)

- **What**: The maximum number of bytes the parser will accept from one input.
- **Terminology**: *cap*, *read limit*, and *maxBytes* all name this single
  value across the spec, plan, and tasks. Canonical usage: *cap* in prose,
  `maxBytes` as the identifier (the `WithMaxConfigBytes` argument and
  `ReadBounded` parameter).
- **Type**: `int64` (matches `io.LimitReader` and the option signature).
- **Default**: `DefaultMaxConfigBytes = 1 << 20` (1,048,576 bytes / 1 MiB).
- **Override**: `WithMaxConfigBytes(n int64)` — per-invocation, no global or
  residual state (SC-004).
- **Safe ceiling**: `MaxConfigBytesCeiling = 1 << 30` (1 GiB) is the maximum
  valid cap. A cap above it — including `math.MaxInt64` — is rejected as
  `config_max_bytes_invalid` (exit `2`); there is no unbounded read path. The
  canonical value lives in `internal/config` (so `ReadBounded` can enforce it
  without an import cycle) and is re-exported as `ax.MaxConfigBytesCeiling`
  (frozen identifier). Because every valid cap is `≤ 1 GiB`, `maxBytes + 1`
  cannot overflow (spec Edge: Cap above the safe ceiling).
- **Validation / value classes**:

  | Value | Classification | Outcome |
  |-------|----------------|---------|
  | `n < 0` | invalid configuration | `config_max_bytes_invalid`, exit `2` |
  | `n == 0` | valid, honored | empty input passes the size check (still parsed → empty fails as `config_invalid`, exit `2`, cause preserved); any non-empty input → `config_too_large`, exit `2` |
  | `0 < n < len(input)` | valid | `config_too_large`, exit `2` |
  | `n == len(input)` (`≤ ceiling`) | valid (inclusive boundary) | accepted, parsed (FR-004, SC-003) |
  | `len(input) < n ≤ MaxConfigBytesCeiling` | valid | accepted, parsed |
  | `n > MaxConfigBytesCeiling` (incl. `math.MaxInt64`) | invalid configuration | `config_max_bytes_invalid`, exit `2` — no unbounded path; overflow impossible (D4) |

### Validation error envelope

- **What**: The standard `ax.Error` returned on rejection (`error.go`,
  unchanged shape). Discoverable via `errors.As(err, &axErr)` where
  `var axErr *ax.Error`.
- **Relevant fields for this feature**:

  | Field | Value for this feature | Frozen? |
  |-------|------------------------|---------|
  | `error_code` | `config_too_large`, `config_max_bytes_invalid`, `config_invalid`, or `config_option_invalid` | **YES** — frozen public contract, golden-locked (FR-007, research D10) |
  | exit code (`ExitCode()`) | `2` (`ExitValidation`) | **YES** — deterministic (SC-002) |
  | `schema_version` | `ErrorSchemaVersion` (`1.0.0`) | governed by the envelope contract |
  | `actionable_fix` | human remediation (shrink config / raise limit; set non-negative cap) | message text not frozen; presence required (FR-007) |
  | `context.max_bytes` | the active cap (`int64`) | **NO** — informational only, not frozen (clarification) |
  | `trace_id` / `span_id` | from caller `ctx`; zero when no span active | documented non-deterministic (Principle II) |
  | `tool` / `version` | **empty** at the library level — `ParseConfig` does not set them; the CLI emission boundary injects them | not part of this feature's contract; goldens pin them as `""` (F5) |
  | cause (unexported, via `Unwrap`) | the underlying parser or schema/type decode error for `config_invalid` (`WithErrorCause`) | never serialized; in-process `errors.Is`/`errors.As` only (research D10) |

- **Distinct from**: a mid-read source I/O error (FR-009), which is the original
  error wrapped with `%w` — **not** an `ax.Error` and **not** classified as
  oversize.

## Internal types (package `internal/config`, not part of the public contract)

- `TooLargeError{ MaxBytes int64 }` — sentinel for oversize; mapped to
  `config_too_large` by `normalizeConfigReadError`.
- `InvalidMaxBytesError{ MaxBytes int64 }` — sentinel for an out-of-range cap
  (negative, or above `MaxConfigBytesCeiling`); mapped to
  `config_max_bytes_invalid`.

These remain `internal/` so consumers classify only via the public `ax.Error`
envelope and its frozen `error_code` (SC-005), never by importing the sentinels.

## State transitions

```text
input + cap
   │
   ├─ cap < 0 OR cap > ceiling ► InvalidMaxBytesError ─► ax.Error{config_max_bytes_invalid, exit 2}
   │
   ├─ ctx canceled / expired ► ctx.Err() (wrapped %w)  [not oversize, not ax.Error]
   │                            └─ at CLI: emitted as ax.Error{error_code: internal_error}; ErrorExitCode maps
   │                               DeadlineExceeded→3, Canceled→1 via errors.Is — classify by exit code, not error_code (FR-011)
   │
   │   ── precedence at the boundary (FR-009) ──
   ├─ bytesRead > cap ──────► TooLargeError ─────────► ax.Error{config_too_large, exit 2}
   │                            (oversize wins once cap+1 bytes are read, even if that read also errored)
   ├─ read error (non-EOF), bytesRead ≤ cap ─► source error (wrapped %w) [not oversize] (FR-009)
   │
   └─ bytesRead ≤ cap, no error ─► hujson.Parse → Standardize → json.Unmarshal → dst
                                    ├─ parser/schema-type failure (incl. empty input)
                                    │  └─► ax.Error{config_invalid, exit 2}
                                    │      (underlying error preserved as cause via Unwrap — D10)
                                    └─ invalid dst pointer ─► *json.InvalidUnmarshalError
                                       (caller misuse, not config_invalid)
```
