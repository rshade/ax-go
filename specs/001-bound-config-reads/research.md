# Phase 0 Research: Bound Config Reads at the Read Boundary

**Feature**: `001-bound-config-reads` | **Date**: 2026-06-02

This document resolves the open decisions for the feature and **absorbs governing
ADR-0010** so that ADR can be deleted as the feature's final task (Constitution
§Governance). No `NEEDS CLARIFICATION` markers remain.

## Decisions

### D1 — Bound at the read boundary with `LimitReader(maxBytes+1)` + length check

- **Decision**: Read at most `maxBytes + 1` bytes from the source. If the result
  exceeds `maxBytes`, classify as oversize (`TooLargeError`); otherwise hand the
  bytes to the Hujson parser. The `+1` makes "one byte over" detectable *without*
  reading the rest of the source, so memory stays ≈ `cap + 1` regardless of how
  large the source actually is (FR-006, SC-001).
- **Mechanism note**: The `maxBytes + 1` bound is realized by the D2 chunked
  read loop — which checks `ctx.Err()` between chunks while accumulating at most
  `maxBytes + 1` bytes — **not** by a bare `io.ReadAll(io.LimitReader(r,
  maxBytes+1))`. A plain `io.ReadAll` would bound the bytes correctly but would
  **not** honor the per-chunk cancelation D2 requires, so D1's byte bound and
  D2's cancelation granularity are two facets of the same loop, not competing
  strategies. T003 is the authoritative implementation instruction.
- **Rationale**: This is the minimal, allocation-bounded way to make oversize a
  validation failure rather than an OOM. It keeps the boundary check at the input
  edge, before Hujson parsing or `Standardize`, exactly as the constitution
  (Principle V) and ADR-0010 require.
- **Alternatives considered**: (a) Read everything then check `len` — rejected:
  defeats the purpose, allocates proportional to the source. (b) `bufio` peek of
  one byte past the cap — rejected: equivalent guarantee, more code than
  `LimitReader`. (c) Stat the file for the file-path entry — rejected: doesn't
  generalize to streams and races with concurrent writers; the boundary must be
  enforced on bytes actually read, uniformly for stream and file (Edge: file-path
  entry point).

### D2 — `context.Context` is the first parameter of both entry points

- **Decision**: `ParseConfig(ctx, r, dst, opts...)` and
  `ParseConfigFile(ctx, path, dst, opts...)`. `ReadBounded` gains a leading `ctx`
  and reads in bounded chunks, checking `ctx.Err()` before each chunk read so a
  canceled/expired context aborts the read; `normalizeConfigReadError` builds the
  `ax.Error` envelope from that `ctx` so `trace_id`/`span_id` are correlated.
- **Rationale**: Constitution Principle X mandates `context.Context` as the first
  parameter of any function doing I/O — both entry points read/open. The byte cap
  stops a *large* source; `ctx` cancelation stops a *slow* one (a hostile or
  hung stream), which is squarely in this feature's threat model (Principle IX).
  It also fixes the bootstrap's `context.Background()` envelope, restoring trace
  correlation (Principle VIII). The API is pre-release, so the signature change is
  cheap now and expensive later. *(User decision, captured during `/speckit-plan`.)*
- **Cancelation granularity**: The chunked loop checks `ctx.Err()` between reads,
  not mid-blocking-`Read`. This is goroutine-free (keeps the path race-clean with
  no extra synchronization) and sufficient: a cooperative source yields between
  chunks. A goroutine-plus-`select` wrapper was rejected as over-engineering for a
  bounded read and an added concurrency surface to test. Consequence (made
  explicit in FR-010): a source that blocks indefinitely *inside* a single `Read`
  call is **outside the cancelation guarantee** — FR-010 covers slow,
  cooperative, multi-chunk sources, not arbitrarily-hung readers.
- **Alternatives considered**: Keep the ctx-less signatures and justify the
  Principle X deviation in Complexity Tracking — rejected by the user in favor of
  compliance while pre-release.

### D3 — Zero cap is a valid, honored limit (not a sentinel)

- **Decision**: `maxBytes == 0` means "only empty input passes the size check."
  Empty input is **not** rejected as oversize, but "passes the size check" is not
  "parses successfully": empty bytes are not valid Hujson, so `hujson.Parse`
  returns its empty-input error — a non-size parse failure outside this feature's
  scope (spec Assumptions). Any non-empty input is `TooLargeError` → exit `2`.
  Zero is **not** a "use default" sentinel and **not** invalid like a negative cap.
- **Rationale**: Directly from the spec clarification (2026-06-01) and Edge:
  Zero cap. The `LimitReader(0+1)` + `len > 0` check already yields this behavior;
  the decision is to lock it with an explicit test rather than leave it implicit.
- **Alternatives considered**: Treat `0` as default (rejected — ambiguous,
  contradicts the clarification) or as invalid (rejected — only negative is
  invalid).

### D4 — Finite safe ceiling; no unbounded read path

- **Decision**: There is a finite safe maximum cap,
  `MaxConfigBytesCeiling = 1 << 30` (1 GiB), re-exported from `ax` (canonical
  value in `internal/config` so `ReadBounded` enforces it without an import
  cycle). `ReadBounded` rejects any cap `< 0` or `> MaxConfigBytesCeiling` as
  `InvalidMaxBytesError` → `config_max_bytes_invalid` (exit `2`) before reading a
  byte. There is **no** effectively-unbounded read path: `math.MaxInt64` is just
  one above-ceiling value, rejected like any other. Because every valid cap is
  `≤ 1 GiB`, `maxBytes + 1` can never overflow `int64`, so the read loop needs no
  special overflow guard.
- **Rationale**: Closes the Principle IX conflict at its source — "NEVER read
  unbounded user input" is honored literally because no cap admits an unbounded
  read. The ceiling is generous (1024× the 1 MiB default) so legitimate
  large-config consumers are unaffected, while a pathological or hostile cap can
  never disable the bound. The overflow hazard the bootstrap special-cased
  disappears by construction.
- **Alternatives considered**: (a) Treat `math.MaxInt64` as a documented unsafe
  opt-out that reads as-is (rejected — leaves a real unbounded read path that
  conflicts with Principle IX's absolute "NEVER read unbounded user input," which
  `/speckit-analyze` flagged CRITICAL; a doc note does not resolve a constitution
  MUST). (b) `uint64`/`big.Int` boundary math (rejected — `int64` matches `io`
  and the option type, and with a finite ceiling no boundary value can overflow).
  The ceiling value (1 GiB) is tunable; a tighter ceiling (e.g., 256 MiB) is
  equally valid and a one-constant change.

### D5 — A mid-read source error is surfaced, never masked as oversize (FR-009)

- **Decision**: If the underlying reader returns a non-EOF error partway through,
  return that error with its chain preserved (wrapped with `%w` for context),
  distinct from `TooLargeError`. `errors.Is`/`errors.As` against the original
  source error must succeed. **Precedence** when a read both crosses the bound and
  errors: a source error observed before `maxBytes + 1` bytes are read wins; once
  `maxBytes + 1` bytes have been read the input is oversize (`TooLargeError`) even
  if that same read returned a non-EOF error. The chunked loop (T003) enforces
  this by checking the accumulated length before propagating a coincident read
  error.
- **Rationale**: FR-009 + Principle IX (`%w` everywhere). A broken stream is an
  I/O failure, not a validation failure; conflating them would mislead an agent's
  recovery logic.
- **Alternatives considered**: Swallow the read error and report oversize
  (rejected — loses the chain, misclassifies the failure).

### D6 — Frozen `error_code`s, golden-locked; `max_bytes` stays informational

- **Decision**: `config_too_large` (oversize) and `config_max_bytes_invalid`
  (negative cap) are **frozen public contract**. Lock them with golden-file
  fixtures of the full envelope (deterministic: zero `trace_id` when no span is
  active) **and** assert the frozen fields (`error_code`, exit code `2`,
  `schema_version`) directly in the test. The `context.max_bytes` field is
  asserted as *present and informational* — it is explicitly **not** part of the
  frozen contract, so a future change to it is a golden update, not a breaking
  change.
- **Rationale**: FR-007 / SC-005 and the spec clarification. The direct field
  assertions express the frozen guarantee; the golden file is the loud-failing
  tripwire that surfaces any envelope drift for review.
- **Alternatives considered**: Golden-only (rejected — would freeze `max_bytes`
  too, contradicting "informational"); assertion-only (rejected — loses the
  golden's drift tripwire on the rest of the envelope shape).

### D7 — SC-001 verified two ways: tripwire reader + `-benchmem` benchmark

- **Decision**: (1) A deterministic counting/tripwire `io.Reader` wrapper that
  `t.Fatal`s if the parser requests more than `cap + 1` bytes, proving the
  boundary is enforced without reading the whole source. (2) A `testing.B`
  benchmark (`-benchmem`, via `make bench`) over inputs at 1×, 10×, and 100× the
  cap, recording that bytes read and `B/op`/`allocs/op` stay bounded as input
  grows.
- **Rationale**: Mandated verbatim by the spec clarification (2026-06-01) and
  SC-001. The tripwire gives a fast, deterministic CI signal; the benchmark
  substantiates the allocation claim (Principle VII: no numeric performance claim
  without a `testing.B`).
- **Alternatives considered**: Measuring process RSS (rejected — noisy,
  non-deterministic, platform-dependent); benchmark only (rejected — the spec
  requires the deterministic tripwire too).

### D8 — Fuzz of the parser surface is deferred (tracked follow-up)

- **Decision**: Do **not** add a `FuzzXxx` harness in this feature. File/keep a
  tracked follow-up (per source issue #1) for fuzzing the Hujson parser surface
  alongside the other parser surfaces (idempotency-key validation, envelope
  round-trip, `TRACEPARENT`).
- **Rationale**: Constitution Principle VII calls for fuzz on every parser
  surface, but the spec's Assumptions and issue #1 deliberately scope fuzzing as a
  separate deliverable. This is recorded as a justified deferral in the plan's
  Complexity Tracking, not a silent omission.
- **Alternatives considered**: Fuzz now (rejected — expands scope past the byte
  boundary and duplicates the follow-up's broader parser-fuzz mandate).

### D9 — Context-error exit-code mapping at the CLI boundary

- **Decision**: `ErrorExitCode` (and thus `Execute`) maps
  `context.DeadlineExceeded` → exit `3` (network/timeout) and `context.Canceled`
  → exit `1` (unknown/internal), recognized via `errors.Is`. The context error is
  **not** wrapped in an `ax.Error` envelope — doing so would break `errors.Is`
  against the context sentinel and contradict FR-010's "chain preserved"
  guarantee — so the mapping is by inspection, not by re-typing the error. At the
  CLI boundary `Execute` still emits an `ax.Error` envelope for the failure with
  `error_code` `internal_error` (its generic non-`ax.Error` normalization path);
  the failure is classified by the mapped exit code, NOT by `error_code` — D9
  deliberately mints no frozen context `error_code`. *Refinement (review
  remediation)*: `ErrorExitCode` consults an explicit `*ax.Error` envelope
  first — an envelope's exit code wins over any sentinel buried in its cause
  chain (reachable via `Unwrap` since D10); the context-sentinel mapping
  applies to non-envelope errors.
- **Rationale**: FR-010 makes a parse cancelable, but a wrapped `context` error
  is not an `*ax.Error`, so the existing `ErrorExitCode` fell through to
  `ExitInternal` (`1`) for *both* cases — misclassifying a deadline as an internal
  error and violating the constitution's deterministic timeout→`3` mapping.
  `DeadlineExceeded` is squarely a timeout (`3`). `Canceled` is a
  caller-initiated abort with no dedicated code in the `0`–`4` set; `1`
  (unknown/internal) is the closest honest fit. (FR-011, SC-006.)
- **Alternatives considered**: Wrap context errors in `ax.Error` with a frozen
  `error_code` (rejected — breaks `errors.Is` to the context sentinel, the whole
  point of FR-010's chain preservation). Leave the mapping to each consuming CLI
  (rejected — would let the determinism gap the feature introduces leak to every
  consumer; the spec owns the mapping it creates).

### D10 — Config decode and option failures classify as frozen validation codes

- **Decision**: A Hujson parse / standardize / schema-type JSON decode failure
  returns the standard `ax.Error` envelope with frozen `error_code`
  `config_invalid` (exit `2`), and a nil `ParseConfigOption` returns
  `config_option_invalid` (exit `2`). Both are frozen public contract,
  golden-locked in `testdata/config_invalid.golden.json` and
  `testdata/config_option_invalid.golden.json`. The underlying decode error is
  attached as the envelope's cause via `WithErrorCause` and exposed through
  `(*ax.Error).Unwrap`, so `errors.Is` / `errors.As` against the source error
  (e.g. `*json.UnmarshalTypeError`) keep working; the cause is never serialized
  into the JSON envelope. Open failures (missing file, permission), mid-read
  source errors (FR-009), and caller misuse such as nil or non-pointer `dst`
  values that produce `*json.InvalidUnmarshalError` remain raw,
  chain-preserved errors.
- **Rationale**: Supersedes the earlier "syntactically invalid configuration is
  out of scope / surfaced as the underlying error" stance. Agents driving a CLI
  need to classify bad config syntax and field types without string-parsing
  library internals, and an unclassified raw user-input decode error fell
  through to exit `1` (internal) at the CLI boundary — misclassifying user error
  as library failure. Invalid destination pointers are not user input and must
  remain programmer errors. Cause-wrapping keeps the full chain for in-process
  callers, so classification is gained without losing diagnosis (Principle IX:
  `errors.Is`/`errors.As` MUST work against every returned error).
- **Alternatives considered**: Return the raw decode error (rejected — agents
  cannot classify it and `Execute` maps it to exit `1`); classify without
  attaching the cause (rejected — destroys the chain, the exact defect the code
  review flagged); non-frozen codes (rejected — agents will branch on them, so
  stability must be guaranteed).

## Decision Records Absorbed

> Constitution §Governance requires every governing ADR's decision, considered
> alternatives, and consequences to be transcribed here **before** the ADR file is
> deleted. The retirement task in `tasks.md` deletes `docs/adr/0010-input-config-hujson.md`
> and updates all references to it.

### ADR-0010 — Input Config Format: Hujson (ACCEPTED 2026-05-28)

**Context**: ax-go CLIs accept human-written configuration files. Strict JSON is
hostile to humans (no comments, no trailing commas, opaque errors). YAML and TOML
introduce separate mental models. Hujson ("Human JSON", Tailscale) is JSON plus
comments and trailing commas — minimal overhead from JSON — and LLM agents that
emit trailing commas/comments when generating config are tolerated without parse
errors.

**Decision drivers**: humans tolerate comments/trailing commas; LLM-generated
config slips in trailing commas; stay close to JSON so `jq`/JSON Schema/IDE
tooling works; a single well-maintained Go parser (`tailscale/hujson`).

**Considered options**:

- **A. `tailscale/hujson`** — JSON + comments + trailing commas; parses to an AST,
  then `Standardize()` strips extensions to strict JSON for `encoding/json`.
  Pros: tiny mental diff from JSON; well-maintained; AST `Patch` for in-place
  edits that preserve comments. Cons: cannot Marshal Go structs back to Hujson
  with comments — **reads are Hujson, writes are strict JSON**.
- **B. Strict `encoding/json`** — stdlib, zero deps; but human-hostile.
- **C. YAML (`gopkg.in/yaml.v3`)** — comment-friendly but a separate mental model
  from the JSON output, whitespace-sensitive, richer than needed.
- **D. TOML (`BurntSushi/toml`)** — readable but flat-friendly; nested config
  awkward; another mental model.
- **E. Starlark / Pkl / Jsonnet** — expressive but overkill; agents handle them
  poorly versus JSON-shaped formats.

**Decision**: Adopt **Option A**, `github.com/tailscale/hujson`, for human-facing
input configuration.

- **Read path** (this feature): bounded read (default 1 MiB, configurable via
  `WithMaxConfigBytes`) → `hujson.Parse` → `Standardize` to strict JSON →
  `encoding/json.Unmarshal` into Go structs. Oversized configs fail as validation
  errors (exit `2`). `ax.ParseConfig` / `ax.ParseConfigFile` are the canonical
  helpers.
- **Write path** (future — ROADMAP #9, **not** implemented by this feature): if
  the file exists, use Hujson's AST `Patch` to preserve user comments/formatting;
  if creating from scratch, emit strict JSON (Hujson can't Marshal with comments).
  This decision is preserved here so it survives the ADR's deletion; the future
  write-path feature MUST reference this section as its absorbed source.

**Consequences**:

- Direct dependency on `github.com/tailscale/hujson` (already in `go.mod`).
- `ax.ParseConfig(...)` is the canonical read helper in the base library.
- Config reads are bounded at the read boundary so user-provided config cannot
  trigger unbounded memory growth — the substance of this feature.
- The read-Hujson / write-JSON asymmetry must be documented in the user-facing
  config guide so consumers aren't surprised when a CLI rewrites their config and
  strips comments.
- For tools that frequently mutate user config, prefer AST `Patch` over
  Marshal+Write to preserve formatting.

**Retirement note**: Once this `research.md` is committed, `tasks.md`'s final task
deletes `docs/adr/0010-input-config-hujson.md` and updates the references found in
the codebase: `README.md` (ADR index row 0010), `ROADMAP.md` (#9 write path +
read-path "done" line), `AGENTS.md` (the `(ADR-0010)` mention), and
`docs/adr/0011-output-payload-json.md` (its cross-reference to ADR-0010). No Go
source file references ADR-0010 by number, so no doc-comment edits are required
for retirement beyond the contract tightening this feature already does.
