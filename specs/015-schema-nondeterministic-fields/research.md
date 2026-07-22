# Phase 0 Research: __schema Non-Deterministic Field Enumeration

**Feature**: `015-schema-nondeterministic-fields` | **Spec**: [spec.md](./spec.md)

## Governing ADR Status

No governing ADR applies to this feature. The GitHub issue (#16) that seeded
`spec.md` references "ADR-0003" (the `__schema` decision) and "ADR-0002"
(schema versioning), but neither corresponds to a file in `docs/adr/` —
that directory currently holds only `0004-trace-id-format.md` and
`0008-cli-framework-cobra.md`. The relevant standing decisions already live
as constitution principles (II, III, XI), so there is nothing to absorb or
retire here. Per the constitution's frozen-ADR governance, no new ADR is
created; this document is the decision record for the feature instead.

## Existing Architecture (as of this feature's starting point)

- `schema/schema.go` (import-isolated public package, NOT `internal/schema`)
  holds the real `__schema` implementation: `Schema`, `CommandSchema`,
  `FlagSchema`, `ErrorSchemaInfo`, `MCPSchema`, `MCPTool`, `BuildSchema`,
  `NewSchemaCommand`, `BuildMCPSchema`. Root package `ax` (`schema.go`) is a
  pure facade of type aliases and pass-through functions.
- `internal/schema/schema.go` is Cobra-tree reflection only (`BuildCommand`,
  `CollectFlags`, `WalkCommands`) — unexported-package `Command`/`Flag`
  structs, no JSON tags, no dependency on `contract`.
- `internal/mcp/mcp.go` independently walks the Cobra tree (via
  `internalschema.WalkCommands`) to build the MCP tool list; it does not go
  through `schema.BuildSchema`/`convertCommandSchema` at all.
- Every command's success payload is `contract.Envelope[T]{Data T, Meta
  Metadata}`, built once per invocation via `ax.NewEnvelope(ctx, payload)`
  inside `RunE`. `contract.Metadata{TraceID, SpanID, IdempotencyKey, DryRun}`
  is shared/generic across every command; `T` is command-author-defined and
  invisible to `BuildSchema` (which only ever sees the `*cobra.Command` tree,
  built once at `Execute`-mount time, before any request happens).
- `contract.Error{ErrorCode, Message, TraceID, Tool, Version, SchemaVersion,
  ActionableFix, Context, Suggestions, Retryable, RetryAfterSeconds}` is one
  flat, global shape shared by the whole CLI (not per-command).
- No struct-tag-driven reflection over arbitrary Go output structs exists
  anywhere in the codebase today — the only reflection is over Cobra
  command/flag metadata. Adding "reflect an output struct via a tag" is new
  machinery, not an extension of an existing pattern.
- `--idempotency-key` auto-generates a UUID v4 in `execute.go`'s
  `wrapPersistentPreRun` when absent, for every command in the tree
  regardless of whether that command declares the flag itself.
- `BenchmarkBuildCommand` is a CI-tracked hot path (`AGENTS.md` → Performance
  Regression Budget table: "`__schema` reflection path"), budget 5% `ns/op`
  / +1 `allocs/op`. Any change to `BuildCommand`'s per-invocation work is
  budget-constrained.
- Golden-file coverage of `__schema` output exists at two layers: root
  `schema_test.go` + `testdata/schema_{ax,mcp}.golden.json` (hand-edited, no
  `-update` flag), and `examples/integration/golden_test.go` +
  `examples/integration/testdata/schema_{ax,mcp}.golden.json` (has
  `-update`, per AGENTS.md the canonical location for `__schema` golden
  tests). A third test, `TestRootSchemaOutputMatchesIsolatedPackage`, pins
  the root facade and the `schema` package to byte-identical output.
- Feature `013-error-recovery-fields` (`research.md` decision D5) already
  established the precedent that adding new **optional** fields to a
  machine-payload shape does not bump `schema_version`/`ErrorSchemaVersion`,
  reasoning: (a) only required-field changes were meant to trigger a bump
  under the retired ADR-0002 rule, transcribed there since the ADR file no
  longer exists, and (b) bumping would change the bytes of every envelope,
  breaking the byte-identical-by-default guarantee for consumers who use no
  new field.

## Decisions

### D1: Marking mechanism — struct tag + envelope-output registration, not free-floating string lists

**Decision**: Introduce a namespaced struct tag, `ax:"nondeterministic"`, that
a command author places directly on an output payload field whose value can vary
between otherwise-identical runs. The tag travels with the field through renames
— satisfying FR-003 exactly — with zero field-name bookkeeping.

Because `BuildSchema` only ever sees the `*cobra.Command` tree and never the
payload type `T` a command's `RunE` closure wraps in `contract.Envelope[T]`, the
tag alone is not enough — something has to tell schema-generation which payload
type to reflect over, and which commands actually emit the standard success
envelope. A new generic function does both pieces of registration explicitly,
once, at command-construction time (the same place `cmd.Flags().StringVar(...)`
already runs):

```go
func WithNonDeterministicFields[T any](cmd *cobra.Command)
```

Calling `ax.WithNonDeterministicFields[reportPayload](cmd)` declares that
`cmd`'s success output is `contract.Envelope[reportPayload]`. For a non-nil
command, the helper reflects `reflect.TypeFor[reportPayload]()` **once**, at
registration time (not per `__schema` invocation, not per command run), computes
the sorted, deduplicated list of `data.`-prefixed JSON-path locators for every
`ax:"nondeterministic"` field, and stores two private entries on Cobra's native
`cmd.Annotations map[string]string` extension point: an envelope-output marker
and the encoded data-field locator list. A nil `cmd` is a no-op, preserving the
"never panic" library contract.

At schema-build time, commands with the private envelope marker receive the
union of the built-in envelope metadata locators (D2) plus their registered
`data.` locators. Commands without the marker receive an explicit empty
command-scoped list. No new mutable package-level state is introduced (each
command's Annotations map is per-instance, owned by that `*cobra.Command`), and
no `any`/`interface{}` leaks into the exported signature — the type parameter
does the work.

**Rationale**: This design satisfies both FR-004 ("MUST NOT require a
hand-maintained list kept separately from the field definitions") and FR-002
(locators map to the command's actual output shape). The author never writes a
field-name string for their own payload; only the tag (co-located with the
field) and a type-parameterized registration call are needed. The registration
call is also the only reliable runtime signal that a command emits
`Envelope[T]`; without it, `BuildSchema` cannot distinguish a normal success
payload command from raw-output commands such as `__schema`, shell completion,
or long-running server commands. Reflection runs against `reflect.Type` only
(static type information), never a live instance, so it works uniformly for
struct, slice-element, and map-value field types without ever constructing a
zero value.

**Alternatives considered**:

- *Pure struct-tag reflection with an implicit global type registry* (no
  explicit `WithNonDeterministicFields` call — a `sync.Map` keyed by command
  path populated via `init()`-style side effects). Rejected: Constitution
  Principle X forbids mutable package-level state and `init()` side effects;
  it also breaks the "no exceptions" transparency the constitution wants —
  an explicit call at the construction site is a comprehensible, greppable
  registration.
- *`go/ast`-based static source scan*, mirroring
  `internal/cmd/doccover`'s approach to `ExampleXxx` coverage. Rejected: that
  tool runs at CI/build time against source files, appropriate for a
  documentation-coverage *check*; `__schema` is a runtime command any
  ax-go-based CLI's binary must answer standalone, with no guarantee the Go
  source tree or a build step is available at run time (an installed binary,
  or one invoked from a different working directory, cannot source-scan
  itself). Wrong layer for a runtime primitive.
- *Flat hand-authored `[]string` per command*, mirroring
  `ErrorSchemaInfo.Required`/`Optional`. Rejected for **author-defined**
  fields specifically — this is the literal anti-pattern FR-004 exists to
  avoid (a second list an author must remember to update on rename). Retained
  deliberately for the **single, global** `ax.Error` shape (D4 below), where
  there is exactly one flat struct for the whole CLI and the existing
  `Required`/`Optional` pattern is already the established, working
  convention — introducing a second mechanism there would be pure churn for
  no benefit.
- *Unconditional built-in metadata locators on every command node*. Rejected:
  it would be cheap, but it would knowingly list `meta.*` paths for commands
  whose actual output is not an `Envelope[T]`, violating FR-002's direct mapping
  requirement and making FR-001's explicit-empty case effectively unreachable.

### D2: Built-in envelope metadata fields — hardcoded literal for registered envelope commands

**Decision**: `meta.trace_id`, `meta.span_id`, and `meta.idempotency_key` are
included only for commands registered as standard success-envelope emitters via
`WithNonDeterministicFields[T]`. They come from a fixed, hardcoded string-slice
literal (`{"meta.trace_id", "meta.span_id", "meta.idempotency_key"}`) evaluated
during schema conversion — not by reflecting `contract.Metadata` on every
`__schema` call. `contract.Metadata`'s `TraceID`, `SpanID`, and
`IdempotencyKey` fields additionally carry the `ax:"nondeterministic"` tag for
self-documentation (so a reader of `contract/json.go` sees the marking inline,
not as tribal knowledge), but production code never reflects that struct at
request time. A single unit test reflects `contract.Metadata` once and asserts
the result equals the hardcoded literal — if a future change adds, removes, or
retags a `Metadata` field without updating the literal, the test catches the
drift immediately (FR-008's spirit, applied to the built-ins specifically).

There is no built-in success-envelope timestamp field in ax-go today:
`contract.Metadata` contains `TraceID`, `SpanID`, `IdempotencyKey`, and
`DryRun`. Timestamp-shaped payload fields (for example `generated_at`) are
therefore treated as payload/domain fields and are enumerated when the author
tags them with `ax:"nondeterministic"`. If ax-go later adds a standard
timestamp field to `Metadata` or `Error`, the drift test forces the built-in
literal or error list to be updated in the same change.

`DryRun` is deliberately **not** tagged: its value is a direct, deterministic
reflection of the `--dry-run` flag for a given invocation, not something that
varies between two runs with identical input.

**Rationale**: `BuildCommand` runs on every `__schema` invocation (it is not
cached) and is one of the four benchmarks tracked under the CI performance
budget (`BenchmarkBuildCommand`, 5% `ns/op` / +1 `allocs/op`). Reflecting a
fixed, four-field struct on every call is unlikely to blow that budget, but a
hardcoded three-string literal costs nothing at all — zero reflection, zero
allocation beyond copying the final locator list — so there is no reason to
spend any of the budget on information that never changes at runtime. The unit
test gets the safety of reflection (drift-detection) without paying its cost in
the hot path. Applying the literal only to registered envelope commands keeps
the list precise for raw-output and error-only commands.

### D3: Locator format

**Decision**: A locator is a dot-separated path of `json` tag names (the tag
name only, `,omitempty`/`,string` etc. stripped) from the envelope root:
`meta.trace_id`, `data.report_id`. For a field nested inside a slice, array,
or map value type, the path descends through the element type **without**
an index or key segment (`data.items.id`, never `data.items[0].id`), because
list positions and map keys are not stable across runs and reflection over
`reflect.Type` never needs a live instance to know an element type's fields.
Embedded (anonymous) struct fields are inlined at the parent's path.
Unexported fields are always skipped (they are never marshaled by
`encoding/json` either, so they can never appear in real output).

**Rationale**: This mirrors how `encoding/json` computes a field's wire name
in the common case, so the locator an author writes matches, verbatim, the
path they would see if they printed their own JSON output — no separate
mental model to learn. This does not extend to `encoding/json`'s
field-shadowing precedence (a shallower field wins over a same-named field
promoted from a deeper embedding, and the deeper field never appears in real
output at all): `DataLocators` still reports a locator for the shadowed
field. This is a known, documented limitation (see `DataLocators`'s doc
comment) rather than a design goal — reusing a JSON field name across
embedding depths is unusual enough in practice that resolving full
`encoding/json` precedence was judged not worth the added walker complexity
for this feature.

### D4: Error envelope — one flat, hardcoded list

**Decision**: `ErrorSchemaInfo` gains a fourth field,
`NonDeterministicFields []string` (JSON key `non_deterministic_fields`),
populated identically wherever `ErrorSchemaInfo` is built, with the fixed
value `["trace_id"]` — matching the flat, unprefixed naming already used by
`Required`/`Optional` (`ax.Error` is never wrapped in an `Envelope[T]`, so
there is no `meta.`/`data.` prefix to apply). No reflection is introduced for
this list: `contract.Error` is one global shape shared by the whole CLI
(unlike `CommandSchema`, which is genuinely per-command), so a fixed literal
alongside the existing hand-authored `Required`/`Optional` lists is
consistent, not a regression — see D1's "Alternatives considered" for why
this is treated differently from the author-defined-field case.

### D5: MCP adapter (`--as=mcp`)

**Decision**: `MCPTool` gains `NonDeterministicFields []string` (JSON key
`nonDeterministicFields`) — camelCase to match the MCP tool convention already
used by `inputSchema` (rather than the ax-native format's snake_case), sourced
from the exact same per-command locator list as
`CommandSchema.NonDeterministicFields`. It does **not** use `omitempty`: now
that raw-output and error-only commands legitimately have an empty
command-scoped list, MCP consumers need the same explicit empty-array signal as
direct `__schema` consumers (FR-001/FR-007).

Because `internal/mcp.Build` walks the Cobra tree independently of
`schema.BuildSchema`/`convertCommandSchema` (it does not currently share code
with the ax-native path), the envelope-registration + locator-union logic is
implemented once, in `internal/schema` (already imported by both `schema` and
`internal/mcp`), as a small exported helper both call — avoiding duplicating
the merge logic in two packages.

### D6: `non_deterministic_fields` presence — never omitted on the ax-native shape

**Decision**: `CommandSchema.NonDeterministicFields []string` uses
`json:"non_deterministic_fields"` with **no** `omitempty`, and the slice is
always initialized non-nil (`[]string{}` at minimum) so `encoding/json`
marshals `[]`, never `null`, for commands with no registered success-envelope
output or marked non-deterministic fields. This directly satisfies FR-001,
which requires the array to be explicit rather than omitted.

### D7: `schema_version` — no bump

**Decision**: Adding `non_deterministic_fields` to `CommandSchema` and
`ErrorSchemaInfo`, and `nonDeterministicFields` to `MCPTool`, does **not**
bump `SchemaVersion`/`contract.ErrorSchemaVersion` (stays `"1.0.0"`).

**Rationale**: Directly follows the D5 precedent set in
`specs/013-error-recovery-fields/research.md` and Constitution Principle XI's
additive-tolerant policy for machine-payload shapes: adding a field is
non-breaking; only removing, renaming, or re-typing an existing field is
breaking (and would ride the minor digit, never patch/major, pre-v1.0). FR-010
in `spec.md` encodes the same rule specifically for entries *within* the
`non_deterministic_fields` list itself (adding an entry later is additive;
removing one is breaking) — that is a separate, finer-grained guarantee about
list *contents*, layered on top of this coarser field-presence guarantee.

### D8: Determinism of the enumeration itself

**Decision**: Every `non_deterministic_fields` list (ax-native, MCP, and the
error envelope's) is sorted and deduplicated before being written into a
schema. `__schema` output for an unchanged command tree must itself stay
byte-identical run over run (Constitution Principle II applies to `__schema`
output exactly as it does to any other command's output) — an unsorted list
built from Go map iteration order, or from Cobra's Annotations map, would
silently violate that.

### D9: Fail-closed reflection, never panic

**Decision**: The registration/reflection helper behind
`WithNonDeterministicFields[T]` must never panic on any `cmd` or `T`. A nil
`cmd` is a no-op. Non-struct types, deeply nested or self-referential structs,
and fields whose types cannot be usefully JSON-tagged (funcs, channels) are
handled fail-closed: the walker bounds recursion depth and skips
non-struct/non-slice/non-map/non-pointer field kinds rather than erroring or
panicking, per Constitution Principle IX ("NEVER `panic` in library code"). A
misuse (e.g., registering a type with no `ax:"nondeterministic"` tags at all)
is not an error condition — it simply contributes only the built-in metadata
set for that registered envelope command, which is valid input.

### D10: `internal/testutil` mask-list overlap — explicitly out of scope for this feature

`internal/testutil/determinism.go`'s `MaskNonDeterministic` currently
hardcodes exactly three maskable field names via regex
(`trace_id|span_id|idempotency_key`) — the same "separately maintained list"
shape FR-004 objects to, one layer down in the test harness rather than in
`__schema` itself. Redriving that regex from a command's own
`non_deterministic_fields` output (instead of a hardcoded pattern) is a
natural, appealing follow-up, but `spec.md`'s Out of Scope section already
excludes "building an agent-side diff tool that consumes this metadata" —
`testutil`'s masking is exactly such a consumer, internal to this repo's own
test suite. This feature avoids introducing a new changing field that would
force mask-list expansion: the integration example uses the existing
`data.entity_id` field (D11), which is already pinned through the
`runWithEntityID` test seam. Redriving `MaskNonDeterministic` from schema
metadata remains a future feature rather than being silently absorbed into this
one's scope.

### D11: Demonstration coverage in `examples/integration`

**Decision**: In addition to extending the golden fixtures with the registered
built-in metadata fields, tag the existing `examples/integration`
`helloPayload.EntityID` field with `ax:"nondeterministic"` and register
`helloPayload` via `ax.WithNonDeterministicFields[helloPayload]` on the root
command. Register other commands that emit standard success envelopes
(`streamPayload`, `patchConfigPayload`) so their `meta.*` fields appear, but do
not add domain tags to those payloads unless their data fields actually vary
for identical inputs.

**Rationale**: User Story 2 ("command author marks a field once") and User
Story 3 ("regressions are caught before release") are only genuinely
independently testable if there is at least one real, author-defined
non-deterministic field exercised end-to-end through `__schema` and the
golden-file suite — otherwise the feature's tests would only ever exercise the
three built-in metadata fields, leaving the author-facing half of the feature
(FR-003/FR-004, the actual point of the struct tag + registration mechanism)
unverified by anything beyond a narrow unit test on the reflection helper.
`EntityID` is already a real generated value in production and already has a
deterministic test seam, so it proves the schema contract without destabilizing
the existing golden-output tests or expanding `internal/testutil`'s mask list.
This is scoped as one tag on one existing field plus registration calls, not a
new command or new payload field.

## Summary of File-Level Impact (informs Phase 1 / tasks.md, not a task list itself)

- `contract/json.go`: add `ax:"nondeterministic"` tags to `Metadata.TraceID`,
  `Metadata.SpanID`, `Metadata.IdempotencyKey` (documentation only — no
  behavior change, tag is not `json:`).
- `internal/schema/schema.go`: add `Annotations map[string]string` passthrough
  to `Command`; add the shared envelope-registration + locator-union helper
  used by both `schema` and `internal/mcp`.
- `schema/schema.go`: add `NonDeterministicFields` to `CommandSchema` and
  `ErrorSchemaInfo`; add `WithNonDeterministicFields[T any]`; wire
  `convertCommandSchema` to populate the new field via the `internal/schema`
  helper, with explicit empty lists for unregistered commands.
- `internal/mcp/mcp.go`: add `NonDeterministicFields` sourcing to `Build`,
  via the same shared `internal/schema` helper; `schema/schema.go`'s
  `MCPTool`/`BuildMCPSchema` passes it through.
- `schema.go` (root `ax` facade): forward `WithNonDeterministicFields[T]`
  (generic pass-through cannot be a type alias; a thin forwarding function is
  required, matching how `BuildSchema`/`NewSchemaCommand` are already
  forwarded).
- `testdata/schema_{ax,mcp}.golden.json` (root) and
  `examples/integration/testdata/schema_{ax,mcp}.golden.json`: regenerated to
  include the new fields.
- `examples/integration/main.go`: `helloPayload.EntityID` gains the
  `ax:"nondeterministic"` tag; success-envelope commands register their payload
  types with `WithNonDeterministicFields` (D11).
- `AGENTS.md`: Core AX Mandates gains a short paragraph pointing at
  `non_deterministic_fields` as the authoritative enumeration (FR-009).
