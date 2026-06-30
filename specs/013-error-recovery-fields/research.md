# Research: Error-envelope recovery & remediation fields

**Feature**: `013-error-recovery-fields` | **Date**: 2026-06-29

This document resolves the design unknowns for adding `retryable` and
`retry_after_seconds` to the `ax.Error` envelope, and absorbs the decisions of
the already-retired ADR-0002 for provenance.

## Decisions

### D1 — Retry-safety is a three-state `*bool`, not a plain `bool`

- **Decision**: Represent `retryable` as a Go `*bool` with JSON
  `json:"retryable,omitempty"`. Three states: `&true` → `"retryable":true`,
  `&false` → `"retryable":false`, `nil` → key absent.
- **Rationale**: FR-002 requires "explicitly unsafe" to be distinguishable from
  "unspecified". A plain `bool` with `omitempty` collapses `false` and unset to
  the same absent key, so an agent could never receive an explicit
  `"retryable":false` ("do NOT retry") — only "true or silence". An explicit
  negative is itself actionable recovery information, which is the whole point of
  the Context/Comprehensibility pillar. `*bool` is the idiomatic Go way to model
  an optional tri-state scalar that JSON-marshals cleanly.
- **Alternatives considered**:
  - *Plain `bool`*: simplest, but only two observable states (true / absent);
    loses the explicit-deny signal. Rejected against FR-002.
  - *`string` enum (`"retryable"|"not_retryable"|""`)*: more states possible but
    invites typos, needs validation, and is heavier than a boolean concept.
    Rejected — over-modeled for a yes/no/unknown.
  - *Separate `non_retryable bool`*: two booleans to express one tri-state is
    error-prone (both set?). Rejected.

### D2 — Backoff is relative `int64` seconds, never an absolute timestamp

- **Decision**: `RetryAfterSeconds int64` with JSON
  `json:"retry_after_seconds,omitempty"`. The value is a relative delta — "wait N
  seconds from now" — and the consumer computes its own wake instant.
- **Rationale**: Constitution II mandates byte-identical output across runs. An
  absolute `retry_after` instant is wall-clock-derived and would differ every
  run, forcing it onto the documented non-deterministic-exception list alongside
  `trace_id`/`idempotency_key`. Relative seconds are fully deterministic and
  mirror the HTTP `Retry-After` delta-seconds form agents already understand.
  `int64` (not `float64`) honors the "no bare float64 for numeric contract
  values" rule and avoids precision drift.
- **Alternatives considered**:
  - *RFC 3339 absolute instant*: human-friendly, but non-deterministic; rejected
    against Constitution II / FR-004 (user's explicit choice: relative seconds).
  - *`time.Duration` (int64 nanoseconds)*: marshals as a raw nanosecond integer
    in JSON — surprising and unit-ambiguous on the wire. Seconds are the agent's
    natural unit. Rejected.
  - *Float seconds*: precision loss + violates the no-`float64` rule. Rejected.

### D3 — A negative backoff value normalizes to "unset" (omitted)

- **Decision**: `WithRetryAfterSeconds(n)` treats `n < 0` as no hint: the field
  is left at its zero value and omitted. `n == 0` is a meaningful "retry
  immediately" value but, under `omitempty`, also serializes as absent — see D4.
- **Rationale**: Option builders return `func(*Error)`, not `error`, so they
  cannot fail loudly; clamping negatives to "unset" is the deterministic,
  fail-safe normalization (FR-009). Emitting a negative wait is nonsensical and
  could push a naive consumer into an immediate hot-loop or a panic on a
  `time.Duration` cast.
- **Alternatives considered**:
  - *Clamp negative → 0*: would emit `retry_after_seconds:0` only if 0 were
    serialized, but `omitempty` hides 0 anyway, so "normalize to unset" and
    "clamp to 0" are observably identical here. We document "unset" as the
    intent. Acceptable either way.
  - *Panic / return error*: violates the no-panic rule and the option-builder
    signature. Rejected.

### D4 — `omitempty` semantics and the `retry_after_seconds == 0` corner

- **Decision**: Both new fields use `omitempty`. For `int64`, `omitempty` omits
  `0`. We accept that an explicit "retry immediately" (`0`) is therefore
  indistinguishable from "unset" on the wire.
- **Rationale**: Preserving FR-005/FR-006 (byte-identical default envelope) is
  worth more than distinguishing `0` from absent for a backoff hint — a consumer
  that sees no `retry_after_seconds` and `retryable:true` already retries as soon
  as it likes, which is exactly what `0` would mean. Making `0` observable would
  require `*int64`, adding pointer ceremony for negligible value. (Contrast D1,
  where the false/unset distinction is genuinely actionable and justifies the
  pointer.)
- **Alternatives considered**: `*int64` for true tri-state seconds — rejected as
  unjustified complexity given `0`≈absent semantically for a wait hint.

### D5 — `schema_version` stays `1.0.0` (no bump)

- **Decision**: `ErrorSchemaVersion` remains `"1.0.0"`. Adding optional fields
  does **not** bump it.
- **Rationale**: Two independent constraints converge here. (1) ADR-0002's own
  rule: only changes to **required** fields require a `schema_version` bump and a
  superseding decision; optional additions are explicitly within the Option-C
  extensibility envelope. (2) Binding determinism: `schema_version` is emitted on
  every error, so bumping it to `1.1.0` would change the bytes of envelopes that
  set **no** recovery fields, violating FR-005 (default envelope byte-identical)
  and breaking the existing `error_envelope.golden.json`. Therefore the additive
  fields share schema version `1.0.0`.
- **Alternatives considered**:
  - *Bump to `1.1.0`*: signals the additive change in-band, but breaks
    byte-identical default output and the existing golden. Rejected against
    FR-005.

### D6 — Struct field order (JSON key order)

- **Decision**: Declare `Retryable` then `RetryAfterSeconds` **after**
  `Suggestions` in `contract.Error`, before the unexported `exitCode`/`cause`.
- **Rationale**: `encoding/json` emits fields in declaration order. Appending at
  the end keeps the existing key sequence (`…,suggestions,…`) intact, so the
  default golden is untouched and the new golden's key order is the natural
  superset. Determinism by construction.

### D7 — Public option-builder names

- **Decision**: `WithRetryable(retryable bool) ErrorOption` and
  `WithRetryAfterSeconds(seconds int64) ErrorOption` on `contract`, re-exported
  verbatim from root `ax`. `WithRetryable` takes a plain `bool` and stores its
  address internally (so callers never juggle `*bool`).
- **Rationale**: Matches the existing `WithXxx` functional-option vocabulary
  (`WithActionableFix`, `WithSuggestions`). A `bool` parameter keeps the
  ergonomic surface simple while the struct holds the tri-state `*bool`; "option
  not called" is the third (nil) state, which is exactly how producers express
  "unspecified".
- **Alternatives considered**: `WithRetryable()` (no arg, implies true) — loses
  the ability to assert explicit `false`. Rejected against FR-002.

## Open implementation choice deferred to coding

Per the learning-mode collaboration, the `bool`-vs-`*bool` representation for
retry-safety (D1) is presented to the author at implementation time as the
shaping decision, with this research recording the recommended path (`*bool`)
and its rationale. The decision does not block planning or task generation.

## Decision Records Absorbed

> Constitution §Governance requires every governing ADR's decision, considered
> alternatives, and consequences to be transcribed here. ADR-0002 was **already
> deleted** from `docs/adr/` in PR #79 (commit `05f0536`, the import-isolated
> contracts feature, spec 010) — its envelope shape now lives in
> `contract/error.go` and is governed by Constitution Principles II & III. This
> transcription is retroactive provenance; there is **no ADR file left to
> delete**, so `tasks.md` carries no retirement task.

### ADR-0002 — JSON Error Envelope Schema (ACCEPTED 2026-05-28, retired in #79)

**Context**: When a command fails, the agent needs a predictable shape to parse
and act on. Every Go CLI otherwise invents its own error format; ax-go's value is
making this uniform. The envelope is emitted to `stderr` (Stream Separation),
never `stdout`.

**Decision drivers**: agents must distinguish error categories without parsing
prose; errors must correlate to trace context; the envelope must be extensible
so tools add domain context without forking the base; schema versioning lets
consumers evolve safely.

**Considered options**:

- **A. Minimal** `{error_code, message}` — smallest surface; no trace
  correlation, no recovery hints, ad-hoc extensions. Rejected.
- **B. Standard** `{error_code, message, actionable_fix, trace_id, tool,
  version}` — covers categorization/recovery/correlation/provenance; `actionable_fix`
  is best-effort. Rejected as not extensible enough.
- **C. Rich** = Standard + `{schema_version, context:{…}, suggestions:[…]}` —
  maximally useful; `schema_version` supports evolution; `context` holds
  tool-specific fields. Adopted.

**Decision**: Adopt **Option C**. Required fields: `error_code`, `message`,
`trace_id`, `tool`, `version`, `schema_version`. Optional fields:
`actionable_fix`, `context`, `suggestions`. `schema_version` follows SemVer;
**major** bumps signal breaking shape changes.

**Consequences**:

- All adopting tools emit a consistent error shape to `stderr`; agents build one
  error-handling path across every ax-go CLI.
- `trace_id` is wired even in OTel no-op mode (zero-value valid hex string) so
  consumer parsers do not break.
- The base package owns the canonical shape; consumers reference it.
- Changes to **required** fields require a `schema_version` major bump (and,
  historically, a superseding ADR — now a Spec Kit feature + constitution
  amendment, per ADR-retirement governance).

**How this feature relates**: `retryable` and `retry_after_seconds` are added as
**optional** fields. This is squarely within Option C's extensibility intent,
touches no required field, and therefore needs no `schema_version` bump (D5) and
no superseding governance beyond this feature. The decision authority that was
ADR-0002 is now Constitution Principles II (Determinism) and III (the `ax.Error`
envelope is golden-guarded public API).
