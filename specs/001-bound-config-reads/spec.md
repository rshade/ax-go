# Feature Specification: Bound Config Reads at the Read Boundary (1 MiB cap)

**Feature Branch**: `001-bound-config-reads`

**Created**: 2026-06-01

**Status**: Implemented

**Input**: User description: "Bound config reads at the read boundary (1 MiB cap)" (GitHub issue #1)

## Clarifications

### Session 2026-06-01

- Q: What does a configured cap of `0` bytes mean? → A: A cap of `0` is a valid, honored limit — empty input passes the size check (it is not rejected as oversize), any non-empty input is rejected as a validation error (exit code `2`). "Passes the size check" is not the same as "parses successfully": empty input is still handed to the parser, and because empty bytes are not valid Hujson, the parse fails as `config_invalid` (exit `2`) with the parser's error preserved as the envelope's cause (research D10). A cap of `0` is NOT a sentinel for "use default" and NOT treated as invalid like a negative cap.
- Q: How binding are the config `error_code` strings? → A: `config_too_large` (oversize) and `config_max_bytes_invalid` (invalid cap — negative or above the safe ceiling) are frozen, stable public `error_code` values, golden-locked in the error-envelope fixture; renaming either is a breaking change. They were joined during review remediation by `config_invalid` (Hujson parse / schema-type decode failure, cause preserved) and `config_option_invalid` (nil parse option), frozen and golden-locked on the same terms (research D10, FR-007). The error `context` payload (e.g., `max_bytes`) stays informational and is NOT part of the frozen contract.
- Q: How is SC-001's bounded-memory guarantee verified? → A: Both — (1) a deterministic tripwire/counting reader that fails the test if the parser reads past `cap + 1` bytes, and (2) a `testing.B` allocation benchmark (`-benchmem`) recording that bytes read and allocations stay bounded as input size grows.

## User Scenarios & Testing *(mandatory)*

The consumers of this feature are (a) Go developers building CLIs on top of
`ax-go` who load configuration through the library, and (b) the LLM agents that
drive those CLIs and may supply attacker-influenced or accidentally enormous
configuration. The feature makes oversized configuration a *bounded, predictable
validation failure* instead of an unbounded memory load.

### User Story 1 - Oversized config is rejected without exhausting memory (Priority: P1)

A CLI built on `ax-go` is handed a configuration source (a file path or a
stream) that is far larger than any legitimate configuration — whether through
an accident, a runaway generator, or a hostile agent trying to crash the
process. The library refuses to read the whole thing into memory; it stops at a
fixed boundary and reports a validation failure.

**Why this priority**: This is the entire reason the feature exists. It closes a
documented guardrail violation ("Never read unbounded user input"). Without it,
a single oversized input can OOM-kill any CLI in the portfolio. Every other
story is a refinement of this one.

**Independent Test**: Feed the configuration entry points an input larger than
the default limit and confirm the call returns an error rather than consuming
memory proportional to the input size. This alone delivers the core protective
value.

**Acceptance Scenarios**:

1. **Given** a configuration source larger than the default cap, **When** the
   consumer parses it, **Then** the parse returns a validation error and process
   memory does not grow proportionally to the input size.
2. **Given** a configuration source larger than the default cap, **When** the
   consumer parses it, **Then** no parsed result is produced and no side effects
   occur.

---

### User Story 2 - The size limit is configurable per consumer (Priority: P2)

A consumer with an unusual but legitimate need — a larger generated config, or a
deliberately tighter limit for an untrusted context — adjusts the maximum
allowed configuration size when invoking the parse, without forking or
re-implementing the loader.

**Why this priority**: The default protects everyone, but a single hardcoded
limit would force consumers with legitimate larger configs to abandon the safe
path. Making the cap a per-call setting keeps the safe path usable for all
consumers. It depends on Story 1's machinery existing.

**Independent Test**: Parse the same input twice — once under a cap that rejects
it and once under a cap that admits it — and confirm the outcome flips with the
configured limit.

**Acceptance Scenarios**:

1. **Given** an input that exceeds the default cap, **When** the consumer raises
   the cap above the input size and parses, **Then** the parse succeeds.
2. **Given** an input that fits the default cap, **When** the consumer lowers the
   cap below the input size and parses, **Then** the parse returns a validation
   error.
3. **Given** a negative cap, **When** the consumer parses, **Then** the parse
   returns a validation error (a misconfigured limit is itself bad input, not a
   crash).

---

### User Story 3 - The rejection is a machine-actionable error envelope (Priority: P3)

An agent driving the CLI receives the oversize rejection and must react
programmatically: detect that it was a *validation* failure (not a network or
auth failure), read a stable error code, and learn how to recover (shrink the
config or raise the limit).

**Why this priority**: A rejection that agents cannot classify is only marginally
better than a crash. The standard `ax.Error` envelope, the deterministic exit
code, and an actionable fix turn a failure into a recoverable step. It builds on
Stories 1 and 2 having produced a rejection in the first place.

**Independent Test**: Trigger an oversize rejection and confirm the returned
error is the standard envelope, carries the validation exit code, and includes
an actionable remediation and the relevant limit in its context.

**Acceptance Scenarios**:

1. **Given** an oversize input, **When** the parse fails, **Then** the error is
   the standard `ax.Error` envelope and is discoverable via `errors.As`.
2. **Given** an oversize input, **When** the parse fails, **Then** the error maps
   to the validation exit code (`2`).
3. **Given** an oversize input, **When** the parse fails, **Then** the error
   carries an actionable fix and the configured byte limit in its context.

---

### Edge Cases

- **Exactly at the limit**: An input whose size equals the cap exactly MUST be
  accepted — the boundary is inclusive, so legitimate configs sized right at the
  limit are not spuriously rejected.
- **One byte over the limit**: An input one byte larger than the cap MUST be
  rejected. The boundary must be detectable without reading the entire
  oversized source.
- **Zero cap**: A cap of zero is a valid, honored limit — not a sentinel for
  "use default" and not treated as invalid like a negative cap. Empty input
  passes the size check (it is not rejected as oversize); any non-empty input is
  rejected as a validation error mapped to exit code `2`. "Passes the size check"
  is not "parses successfully": empty input is still handed to the parser, and
  because empty bytes are not valid Hujson, the parse fails as `config_invalid`
  (exit `2`) with the parser's error preserved as the envelope's cause
  (research D10).
- **Cap above the safe ceiling**: There is a finite safe maximum cap,
  `MaxConfigBytesCeiling` (1 GiB). A cap greater than the ceiling — including
  `math.MaxInt64` — MUST be rejected as invalid configuration
  (`config_max_bytes_invalid`, exit `2`), never honored as an unbounded read.
  Because no effectively-unbounded read path exists, the boundary check cannot
  overflow: any cap large enough to wrap `cap + 1` is already rejected as
  above-ceiling.
- **Mid-read source failure**: If the underlying source errors partway through
  (e.g., a broken stream), that read error MUST be surfaced (wrapped), not
  masked as an oversize error. Precedence when both conditions arise at the
  boundary: a source error returned **before** `cap + 1` bytes have been read
  wins (surfaced as the source error, FR-009); once `cap + 1` bytes have been
  read, the input is classified oversize (`config_too_large`) even if the same
  read also returns a non-EOF error.
- **Canceled or expired context**: If the caller's context is already canceled,
  or its deadline expires mid-read, the parse MUST abort the bounded read and
  surface the context error (`context.Canceled` / `context.DeadlineExceeded`)
  with its chain preserved — never masked as an oversize error and never a
  panic. Cancelation is observed *between* chunk reads (a cooperative,
  multi-chunk source yields between reads); a source that blocks indefinitely
  *inside* a single `Read` call is outside this guarantee. This stops a *slow*
  (cooperative) source, complementing the byte cap that stops a *large* one.
- **File-path entry point**: The same cap behavior applies whether the
  configuration arrives as a stream or is opened from a file path.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Both the stream-based and file-path configuration parse entry
  points MUST enforce a maximum read size, defaulting to 1 MiB
  (1,048,576 bytes).
- **FR-002**: The maximum read size MUST be configurable per parse invocation
  through a functional option, without requiring the consumer to replace the
  parse entry points.
- **FR-003**: An input that exceeds the active cap MUST result in a validation
  error mapped to exit code `2` — never an out-of-memory condition and never a
  generic, unclassified error.
- **FR-004**: An input whose size is exactly equal to the active cap MUST be
  accepted and parsed normally.
- **FR-005**: A cap outside the valid range — negative, or greater than the safe
  ceiling `MaxConfigBytesCeiling` (1 GiB) — MUST be treated as invalid
  configuration and reported as a validation error (`config_max_bytes_invalid`)
  mapped to exit code `2`. There is no effectively-unbounded read path.
- **FR-006**: The bound MUST be enforced at the read boundary — the amount of
  memory consumed while detecting an oversize input MUST be bounded to
  approximately the cap (at most ~`cap + 1` bytes read, the threshold SC-001's
  tripwire enforces), independent of how large the underlying source is.
- **FR-007**: Config failures MUST be emitted as the standard `ax.Error`
  envelope, discoverable via `errors.As`, carrying an actionable remediation
  message. Four `error_code` values are frozen public contract, golden-locked
  in `testdata/` fixtures; renaming any of them is a breaking change:
  `config_too_large` (oversize), `config_max_bytes_invalid` (invalid cap —
  negative or above the ceiling), `config_invalid` (Hujson parse / schema-type
  decode failure, with the underlying error preserved as the envelope's cause
  via `Unwrap` — research D10), and `config_option_invalid` (nil parse option).
  The byte limit is additionally provided in the error `context` (`max_bytes`)
  as informational data for the size/cap codes and is NOT part of the frozen
  contract.
- **FR-008**: Given identical input and an identical cap, repeated parses MUST
  produce the same outcome (the same success/failure classification and the same
  error code), consistent with the determinism mandate.
- **FR-009**: A read error originating from the underlying source MUST be
  surfaced with its error chain preserved, distinct from an oversize
  classification. Precedence is defined at the boundary: a source error observed
  **before** `cap + 1` bytes have been read is surfaced as the source error;
  once `cap + 1` bytes have been read, the input is classified oversize
  (`config_too_large`, exit `2`) even if the same `Read` also returns a non-EOF
  error.
- **FR-010**: Both parse entry points MUST take a `context.Context` as their
  first parameter and MUST honor its cancelation: a context that is already
  canceled, or that expires mid-read, MUST abort the bounded read and surface
  the context error (`context.Canceled` / `context.DeadlineExceeded`) with its
  chain preserved (discoverable via `errors.Is`), distinct from an oversize
  classification and never a panic. Cancelation is observed *between* chunk
  reads, so the guarantee covers cooperative, multi-chunk sources that yield
  between reads; a source that blocks indefinitely *inside* a single `Read` call
  is explicitly outside this guarantee (closing it would require a
  goroutine-plus-`select` wrapper that research D2 rejected as over-engineering
  for a bounded read). The same context correlates the error envelope's
  `trace_id` / `span_id` when a span is active. (This is a pre-release signature
  change with no compatibility shim; it satisfies the constitution's
  context-first mandate for I/O and its "cancel a slow source" resource-safety
  posture.)
- **FR-011**: At the CLI boundary, the exit-code mapping (`ErrorExitCode`) MUST
  classify a parse that failed with `context.DeadlineExceeded` as the
  network/timeout exit code (`3`) and one that failed with `context.Canceled` as
  the unknown/internal exit code (`1`), recognized via `errors.Is` so the
  original error chain is preserved (the context error is NOT wrapped in an
  `ax.Error`, which would break `errors.Is` against the context sentinel). This
  keeps the cancelation path FR-010 introduces deterministically classifiable,
  consistent with the constitution's deterministic exit-code mandate. The
  emitted `ax.Error` envelope for such a failure carries `error_code`
  `internal_error` (the library returns the raw wrapped context error, which
  `Execute` normalizes for emission via its generic non-`ax.Error` path):
  cancelation/timeout failures are therefore classified by **exit code**
  (`3` / `1`), NOT by a frozen `error_code`. (SC-005's `error_code`
  classification governs the oversize / invalid-cap rejections; SC-006 governs
  the cancelation paths.)

### Key Entities *(include if feature involves data)*

- **Configuration input**: The raw byte stream of configuration to be parsed,
  arriving either as a stream or as a file opened from a path. Treated as
  untrusted and potentially unbounded in size.
- **Read limit (cap)**: The maximum number of bytes the parser will accept from
  a configuration input. Has a safe default (1 MiB), a per-invocation override,
  and a finite safe ceiling (`MaxConfigBytesCeiling`, 1 GiB) above which a cap is
  rejected as invalid configuration.
- **Validation error envelope**: The standard `ax.Error` returned on rejection,
  carrying a stable `error_code`, the validation exit code (`2`), an actionable
  fix, and — informationally — the configured byte limit in its `context`
  (`max_bytes`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a configuration input arbitrarily larger than the cap (e.g.,
  100× the cap), the parse rejects it without reading past the boundary, and the
  memory attributable to the read stays bounded near the cap rather than scaling
  with the input size. Verified two ways: (1) a deterministic tripwire/counting
  reader that fails if the parser requests more than `cap + 1` bytes, and (2) a
  `testing.B` allocation benchmark (`-benchmem`) recording that bytes read and
  allocations stay bounded as input size grows.
- **SC-002**: 100% of oversize-input and invalid-cap rejections surface the
  validation exit code (`2`).
- **SC-003**: A configuration input sized exactly at the cap is accepted in 100%
  of cases (no off-by-one false rejection at the boundary).
- **SC-004**: A consumer can change the cap for a parse invocation and the new
  limit takes effect immediately for that invocation, with no global or residual
  state from prior invocations.
- **SC-005**: Every rejection is programmatically classifiable from the returned
  error alone — a consumer can read the stable `error_code` (`config_too_large`,
  `config_max_bytes_invalid`, `config_invalid`, `config_option_invalid`) and the
  validation exit code (`2`) with no string parsing of human-facing text
  required. The exceeded limit is additionally available in the error `context`
  (`max_bytes`) as informational data for the size/cap codes.
- **SC-006**: 100% of parse failures caused by an expired deadline surface the
  network/timeout exit code (`3`), and 100% caused by a canceled context surface
  the unknown/internal exit code (`1`), classifiable from the returned error
  alone via `errors.Is` with no parsing of human-facing text.

## Assumptions

- Source inputs: GitHub issue #1 and governing ADR ADR-0010 (Hujson input). The
  governing ADR's decisions are absorbed into `research.md` during planning and
  the ADR is retired as the feature's final task (keep ADR detail out of this
  spec body; it is user-facing).
- The cap is measured against raw bytes read from the source, before parsing or
  standardization — the protection is at the input boundary, not after the data
  is already in memory.
- The default cap of 1 MiB is assumed sufficient for all legitimate
  configuration files in the portfolio; consumers with larger legitimate needs
  use the override (Story 2), up to the `MaxConfigBytesCeiling` (1 GiB) safe
  maximum, above which a cap is rejected as invalid.
- Failures unrelated to size or syntax — a missing file or a permission error
  on open — are outside this feature's scope and surface as their underlying
  errors. Syntactically invalid configuration and config schema/type mismatches
  are classified as `config_invalid` (exit `2`) with the parser or decode error
  preserved as the envelope's cause (research D10). Invalid decode destinations,
  such as nil or non-pointer `dst` values, are caller misuse and surface as the
  underlying `*json.InvalidUnmarshalError`.
- Fuzz coverage of the bounded reader is tracked as a separate follow-up per the
  source issue and is not a deliverable of this feature.
- A bootstrap implementation already realizes much of this contract in the
  current codebase; the planning phase should reconcile this specification
  against the existing code and scope the remaining work (verification,
  documentation, and the separately-tracked fuzz follow-up) accordingly.
- The bootstrap's parse entry points are `context`-less; this feature adds a
  `context.Context` first parameter (FR-010). Because the API is pre-release,
  the signature change ships without a compatibility shim — it is a deliberate
  constitution-alignment decision, not an incidental refactor, and is captured
  as a first-class requirement so it traces from spec through tasks.
